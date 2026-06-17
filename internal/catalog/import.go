package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/platform/tabular"
)

// maxImportRows bounds how many data rows one import processes, so a huge roster
// cannot drive an unbounded provisioning loop and a giant long-running
// transaction (CWE-770). Excess rows are dropped and reported, never silently
// truncated.
const maxImportRows = 10000

// capRows returns at most maxImportRows rows and a RowError describing the drop
// when the file is over the cap.
func capRows(rows [][]string) ([][]string, []RowError) {
	if len(rows) <= maxImportRows {
		return rows, nil
	}

	msg := fmt.Sprintf("file has %d rows; only the first %d were imported — split the file", len(rows), maxImportRows)

	return rows[:maxImportRows], []RowError{{Line: maxImportRows + 1, Message: msg}}
}

// RowError records one rejected import row: its 1-based line in the source file
// (the header is line 1) and why it was skipped. Malformed rows are reported,
// not fatal — the rest of the file still imports (PLAN §6).
type RowError struct {
	Line    int
	Message string
}

// entryColumns map an entry-list header to logical fields. Only the car number
// is required; team/class/drivers/livery are best-effort. Multiple drivers in
// one cell are split on common separators.
var entryColumns = []tabular.Column{
	{Key: "car_no", Synonyms: []string{"car", "car №", "car no", "no", "number", "#", "№"}, Required: true},
	{Key: "team", Synonyms: []string{"team", "entrant", "team name"}},
	{Key: "class", Synonyms: []string{"class", "category", "cat"}},
	{Key: "drivers", Synonyms: []string{"drivers", "driver", "driver lineup", "lineup", "crew"}},
	{Key: "livery", Synonyms: []string{"livery", "livery ref", "livery refs", "liveries"}},
}

// accreditationColumns map an accreditation header. Name, email and tier are
// required; the validity window and credential reference are optional.
var accreditationColumns = []tabular.Column{
	{Key: "name", Synonyms: []string{"name", "person", "full name", "person name"}, Required: true},
	{Key: "email", Synonyms: []string{"email", "e-mail", "mail", "email address"}, Required: true},
	{Key: "tier", Synonyms: []string{"tier", "band", "access", "accreditation"}, Required: true},
	{Key: "valid_from", Synonyms: []string{"valid from", "from", "start"}},
	{Key: "valid_to", Synonyms: []string{"valid to", "to", "end", "expires"}},
	{Key: "credential", Synonyms: []string{"credential", "credential ref", "pass", "badge"}},
}

// EntryPreview is the parsed-and-validated result of an entry-list file before
// or after persistence: the valid entries and the rejected rows.
type EntryPreview struct {
	Entries []Entry
	Errors  []RowError
}

// PreviewEntryList parses and validates an entry-list sheet without persisting.
func (s *Service) PreviewEntryList(sheet tabular.Sheet) (EntryPreview, error) {
	return parseEntries(sheet)
}

// ImportEntryList parses, validates, and persists an entry-list sheet under an
// event, returning the same preview. Valid rows are inserted in one transaction;
// rejected rows are reported, not fatal.
func (s *Service) ImportEntryList(
	ctx context.Context, orgID, eventID, filename string, sheet tabular.Sheet,
) (EntryPreview, error) {
	preview, err := parseEntries(sheet)
	if err != nil {
		return EntryPreview{}, err
	}

	if len(preview.Entries) == 0 {
		return preview, nil
	}

	err = s.store.WithOrg(ctx, orgID, func(ctx context.Context, tx *sql.Tx) error {
		// Verify the event belongs to the caller's org before writing. The RLS
		// WITH CHECK on entry_lists only guards org_id; the event_id FK check
		// bypasses RLS, so without this scoped read an admin could attach rows to
		// another org's event (ADR-0008 / the IDOR guard). GetEventTx is
		// RLS-SELECT-filtered, so a foreign event id returns ErrEventNotFound.
		if _, err := s.store.GetEventTx(ctx, tx, eventID); err != nil {
			return err
		}

		listID, err := s.store.InsertEntryListTx(ctx, tx, orgID, eventID, filename)
		if err != nil {
			return err
		}

		return s.store.InsertEntriesTx(ctx, tx, orgID, listID, preview.Entries)
	})
	if err != nil {
		return EntryPreview{}, err
	}

	return preview, nil
}

func parseEntries(sheet tabular.Sheet) (EntryPreview, error) {
	m, err := tabular.Map(sheet.Header, entryColumns)
	if err != nil {
		return EntryPreview{}, err
	}

	var preview EntryPreview

	rows, capErrs := capRows(sheet.Rows)
	preview.Errors = append(preview.Errors, capErrs...)

	seen := make(map[string]bool)

	for i, row := range rows {
		line := i + 2 // header is line 1

		carNo := m.Value(row, "car_no")
		if carNo == "" {
			preview.Errors = append(preview.Errors, RowError{Line: line, Message: "missing car number"})

			continue
		}

		if seen[carNo] {
			preview.Errors = append(preview.Errors, RowError{Line: line, Message: "duplicate car number " + carNo})

			continue
		}

		seen[carNo] = true

		preview.Entries = append(preview.Entries, Entry{
			CarNo:      carNo,
			Team:       m.Value(row, "team"),
			Class:      m.Value(row, "class"),
			Drivers:    splitMulti(m.Value(row, "drivers")),
			LiveryRefs: splitMulti(m.Value(row, "livery")),
		})
	}

	return preview, nil
}

// AccreditationRow is one parsed, valid roster line ready to provision.
type AccreditationRow struct {
	Line  int
	Name  string
	Email string
	Tier  Tier
}

// AccreditationPreview is the parsed result of an accreditation file: the valid
// rows, the per-tier tally, and the rejected rows.
type AccreditationPreview struct {
	Rows       []AccreditationRow
	TierCounts map[Tier]int
	Errors     []RowError
}

// AccreditationResult is the outcome of importing an accreditation roster: the
// preview plus how many fresh invites were enqueued.
type AccreditationResult struct {
	AccreditationPreview
	Invited int
}

// PreviewAccreditation parses and validates an accreditation sheet without
// persisting or provisioning.
func (s *Service) PreviewAccreditation(sheet tabular.Sheet) (AccreditationPreview, error) {
	return parseAccreditations(sheet)
}

// ImportAccreditation parses a roster, then for each valid row provisions a
// consumer account, inserts the accreditation, and enqueues a magic-link invite
// — all in one transaction so an invite never outlives its row (ADR-0016).
// Re-import is idempotent: an already-accredited person yields no new invite.
func (s *Service) ImportAccreditation(
	ctx context.Context, orgID, eventID string, sheet tabular.Sheet,
) (AccreditationResult, error) {
	preview, err := parseAccreditations(sheet)
	if err != nil {
		return AccreditationResult{}, err
	}

	result := AccreditationResult{AccreditationPreview: preview}

	err = s.store.WithOrg(ctx, orgID, func(ctx context.Context, tx *sql.Tx) error {
		// Scoped ownership check before any write — see ImportEntryList: the
		// accreditations event_id FK bypasses RLS, so confirm the event is the
		// caller's via an RLS-filtered read first (the IDOR guard).
		if _, err := s.store.GetEventTx(ctx, tx, eventID); err != nil {
			return err
		}

		for i := range preview.Rows {
			row := preview.Rows[i]

			user, _, err := s.provisioner.ProvisionConsumerTx(ctx, tx, orgID, row.Email)
			if errors.Is(err, identity.ErrEmailTaken) {
				result.Errors = append(result.Errors, RowError{
					Line:    row.Line,
					Message: "email registered to another organization: " + row.Email,
				})

				continue
			}

			if err != nil {
				return err
			}

			_, created, err := s.store.InsertAccreditationTx(ctx, tx, Accreditation{
				OrgID:      orgID,
				EventID:    eventID,
				UserID:     user.ID,
				PersonName: row.Name,
				Email:      row.Email,
				Tier:       row.Tier,
			})
			if err != nil {
				return err
			}

			if !created {
				continue
			}

			if err := s.enqueuer.EnqueueInviteTx(ctx, tx, user.ID, orgID, row.Email); err != nil {
				return err
			}

			result.Invited++
		}

		return nil
	})
	if err != nil {
		return AccreditationResult{}, err
	}

	return result, nil
}

func parseAccreditations(sheet tabular.Sheet) (AccreditationPreview, error) {
	m, err := tabular.Map(sheet.Header, accreditationColumns)
	if err != nil {
		return AccreditationPreview{}, err
	}

	preview := AccreditationPreview{TierCounts: make(map[Tier]int)}

	rows, capErrs := capRows(sheet.Rows)
	preview.Errors = append(preview.Errors, capErrs...)

	for i, row := range rows {
		line := i + 2

		name := m.Value(row, "name")
		email := m.Value(row, "email")
		tierRaw := m.Value(row, "tier")

		if name == "" || email == "" {
			preview.Errors = append(preview.Errors, RowError{Line: line, Message: "missing name or email"})

			continue
		}

		if !strings.Contains(email, "@") {
			preview.Errors = append(preview.Errors, RowError{Line: line, Message: "invalid email: " + email})

			continue
		}

		tier, err := ParseTier(tierRaw)
		if err != nil {
			preview.Errors = append(preview.Errors, RowError{Line: line, Message: "invalid tier: " + tierRaw})

			continue
		}

		preview.Rows = append(preview.Rows, AccreditationRow{Line: line, Name: name, Email: email, Tier: tier})
		preview.TierCounts[tier]++
	}

	return preview, nil
}

// splitMulti splits a multi-value cell ("A; B / C") into trimmed, non-empty
// parts. An empty cell yields nil so the column default applies.
func splitMulti(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}

	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ';' || r == '/' || r == '|'
	})

	var out []string

	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}

	return out
}
