package invite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/pubkraal/paddock/internal/invite"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

var errBoom = errors.New("boom")

func TestArgs_Kind(t *testing.T) {
	t.Parallel()

	if got := (invite.AccreditationInviteArgs{}).Kind(); got != "accreditation_invite" {
		t.Errorf("Kind() = %q, want accreditation_invite", got)
	}
}

type fakeRequester struct {
	gotEmail string
	err      error
}

func (f *fakeRequester) RequestMagicLink(_ context.Context, email string) error {
	f.gotEmail = email

	return f.err
}

func TestWorker_WorkSendsMagicLink(t *testing.T) {
	t.Parallel()

	req := &fakeRequester{}
	w := invite.NewWorker(req)

	job := &river.Job[invite.AccreditationInviteArgs]{
		Args: invite.AccreditationInviteArgs{UserID: "u1", OrgID: "o1", Email: "consumer@nls.test"},
	}

	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if req.gotEmail != "consumer@nls.test" {
		t.Errorf("requested email = %q, want consumer@nls.test", req.gotEmail)
	}
}

func TestWorker_WorkPropagatesError(t *testing.T) {
	t.Parallel()

	w := invite.NewWorker(&fakeRequester{err: errBoom})

	job := &river.Job[invite.AccreditationInviteArgs]{
		Args: invite.AccreditationInviteArgs{Email: "x@nls.test"},
	}

	if err := w.Work(context.Background(), job); !errors.Is(err, errBoom) {
		t.Fatalf("Work err = %v, want errBoom (retryable)", err)
	}
}

type fakeInserter struct {
	gotArgs river.JobArgs
	err     error
}

func (f *fakeInserter) InsertTx(
	_ context.Context, _ *sql.Tx, args river.JobArgs, _ *river.InsertOpts,
) (*rivertype.JobInsertResult, error) {
	f.gotArgs = args

	return &rivertype.JobInsertResult{}, f.err
}

func TestEnqueuer_EnqueueInviteTx(t *testing.T) {
	t.Parallel()

	ins := &fakeInserter{}
	enq := invite.NewEnqueuer(ins)

	if err := enq.EnqueueInviteTx(context.Background(), nil, "u1", "o1", "consumer@nls.test"); err != nil {
		t.Fatalf("EnqueueInviteTx: %v", err)
	}

	args, ok := ins.gotArgs.(invite.AccreditationInviteArgs)
	if !ok {
		t.Fatalf("enqueued args type = %T, want AccreditationInviteArgs", ins.gotArgs)
	}

	if args.UserID != "u1" || args.OrgID != "o1" || args.Email != "consumer@nls.test" {
		t.Errorf("enqueued args = %+v", args)
	}
}

func TestEnqueuer_EnqueueInviteTxError(t *testing.T) {
	t.Parallel()

	enq := invite.NewEnqueuer(&fakeInserter{err: errBoom})

	if err := enq.EnqueueInviteTx(context.Background(), nil, "u1", "o1", "x@nls.test"); !errors.Is(err, errBoom) {
		t.Fatalf("EnqueueInviteTx err = %v, want errBoom", err)
	}
}
