package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pubkraal/paddock/internal/identity"
	"github.com/pubkraal/paddock/internal/platform/mailer"
)

type mockMailer struct {
	msg    mailer.Message
	err    error
	called bool
}

func (m *mockMailer) Send(_ context.Context, msg mailer.Message) error {
	m.called = true
	m.msg = msg

	return m.err
}

// blockingMailer ignores its context and blocks until released, modelling a slow
// SMTP hop (net/smtp.SendMail is not context-aware).
type blockingMailer struct {
	block chan struct{}
}

func (b blockingMailer) Send(context.Context, mailer.Message) error {
	<-b.block

	return nil
}

func TestLinkMailer_SendMagicLink(t *testing.T) {
	t.Parallel()

	mm := &mockMailer{}
	lm := identity.NewLinkMailer(mm, time.Second)

	link := "https://paddock.test/auth/redeem?token=abc123"

	if err := lm.SendMagicLink(context.Background(), "press@example.test", link); err != nil {
		t.Fatalf("SendMagicLink: %v", err)
	}

	if mm.msg.To != "press@example.test" {
		t.Errorf("To = %q, want press@example.test", mm.msg.To)
	}

	if mm.msg.Subject == "" {
		t.Error("Subject is empty")
	}

	if !strings.Contains(mm.msg.Text, link) {
		t.Errorf("text body missing the link:\n%s", mm.msg.Text)
	}

	if !strings.Contains(mm.msg.HTML, link) {
		t.Errorf("html body missing the link:\n%s", mm.msg.HTML)
	}
}

func TestLinkMailer_SendMagicLinkError(t *testing.T) {
	t.Parallel()

	mm := &mockMailer{err: errors.New("smtp down")}
	lm := identity.NewLinkMailer(mm, time.Second)

	err := lm.SendMagicLink(context.Background(), "press@example.test", "https://paddock.test/x")
	if err == nil {
		t.Fatal("expected error when transport fails, got nil")
	}
}

func TestLinkMailer_SendMagicLinkTimesOut(t *testing.T) {
	t.Parallel()

	bm := blockingMailer{block: make(chan struct{})}
	t.Cleanup(func() { close(bm.block) }) // let the background goroutine finish

	lm := identity.NewLinkMailer(bm, 20*time.Millisecond)

	err := lm.SendMagicLink(context.Background(), "press@example.test", "https://paddock.test/x")
	if err == nil {
		t.Fatal("expected a timeout error when the send stalls, got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}
