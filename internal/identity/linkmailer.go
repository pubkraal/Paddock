package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/pubkraal/paddock/internal/platform/mailer"
)

// LinkMailer composes and sends the magic-link email. It is the domain mailer
// ADR-0007 calls for: it owns the message body and delegates transport to the
// platform Mailer.
type LinkMailer struct {
	mailer  mailer.Mailer
	timeout time.Duration
}

// NewLinkMailer builds a LinkMailer over the given transport. timeout bounds how
// long a send may stretch the caller's response: a slow SMTP hop must not widen
// the anti-enumeration timing window (ADR-0013).
func NewLinkMailer(m mailer.Mailer, timeout time.Duration) *LinkMailer {
	return &LinkMailer{mailer: m, timeout: timeout}
}

// SendMagicLink emails a sign-in link to the recipient, returning once the send
// completes or the timeout elapses — whichever is first. On timeout the send may
// finish in the background; the caller's response is not held open for it.
func (l *LinkMailer) SendMagicLink(ctx context.Context, to, link string) error {
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	msg := mailer.Message{
		To:      to,
		Subject: "Your Paddock sign-in link",
		Text:    magicLinkText(link),
		HTML:    magicLinkHTML(link),
	}

	done := make(chan error, 1)
	go func() { done <- l.mailer.Send(ctx, msg) }()

	select {
	case <-ctx.Done():
		return fmt.Errorf("identity: send magic link: %w", ctx.Err())
	case err := <-done:
		if err != nil {
			return fmt.Errorf("identity: send magic link: %w", err)
		}

		return nil
	}
}

func magicLinkText(link string) string {
	return "Sign in to Paddock:\n\n" + link +
		"\n\nThis link is single-use and expires shortly. " +
		"If you didn't request it, you can ignore this email."
}

func magicLinkHTML(link string) string {
	return fmt.Sprintf(
		`<p>Sign in to Paddock:</p>`+
			`<p><a href=%q>Sign in</a></p>`+
			`<p>This link is single-use and expires shortly. `+
			`If you didn't request it, you can ignore this email.</p>`,
		link,
	)
}
