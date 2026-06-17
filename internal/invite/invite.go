// Package invite is the bulk-provisioning bridge between accreditation import
// and the worker (ADR-0016): a transactionally-enqueued River job that, when
// worked, issues a consumer-grant magic link and emails it. The job is inserted
// inside the same transaction as the accreditation row write, so an invite never
// outlives — or precedes — its row.
package invite

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// AccreditationInviteArgs is the job payload: which provisioned consumer to
// invite. The email is what the magic-link request resolves.
type AccreditationInviteArgs struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Email  string `json:"email"`
}

// Kind identifies the job type in the River tables.
func (AccreditationInviteArgs) Kind() string { return "accreditation_invite" }

// magicLinkRequester is the slice of the identity service the worker needs:
// resolve the email and email its magic link. Defined at the consumer so the
// worker is tested with a fake.
type magicLinkRequester interface {
	RequestMagicLink(ctx context.Context, email string) error
}

// Worker handles AccreditationInviteArgs by issuing and sending the invitee's
// magic link. It reuses the identity service's RequestMagicLink, so token
// issuance, the consumer-grant purpose, and the email send are single-sourced.
type Worker struct {
	river.WorkerDefaults[AccreditationInviteArgs]

	requester magicLinkRequester
}

// NewWorker builds the invite Worker over the magic-link requester.
func NewWorker(requester magicLinkRequester) *Worker {
	return &Worker{requester: requester}
}

// Work sends the invitee's magic link. A send/issue failure is returned so River
// retries with backoff.
func (w *Worker) Work(ctx context.Context, job *river.Job[AccreditationInviteArgs]) error {
	return w.requester.RequestMagicLink(ctx, job.Args.Email)
}

// jobInserter is the slice of River's insert client the Enqueuer needs. The
// concrete *river.Client[*sql.Tx] satisfies it; tests use a fake.
type jobInserter interface {
	InsertTx(
		ctx context.Context, tx *sql.Tx, args river.JobArgs, opts *river.InsertOpts,
	) (*rivertype.JobInsertResult, error)
}

// Enqueuer enqueues invite jobs inside a caller's transaction so the job commits
// atomically with the accreditation write (ADR-0016). It satisfies the
// catalog service's invite-enqueuer dependency.
type Enqueuer struct {
	inserter jobInserter
}

// NewEnqueuer wraps a River insert client (or any tx-scoped job inserter).
func NewEnqueuer(inserter jobInserter) *Enqueuer {
	return &Enqueuer{inserter: inserter}
}

// EnqueueInviteTx enqueues one accreditation invite within tx.
func (e *Enqueuer) EnqueueInviteTx(ctx context.Context, tx *sql.Tx, userID, orgID, email string) error {
	_, err := e.inserter.InsertTx(ctx, tx, AccreditationInviteArgs{
		UserID: userID,
		OrgID:  orgID,
		Email:  email,
	}, nil)

	return err
}
