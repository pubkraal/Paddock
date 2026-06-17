package identity

import (
	"context"
	"fmt"

	"github.com/pubkraal/paddock/internal/platform/mailer"
)

// LinkMailer composes and sends the magic-link email. It is the domain mailer
// ADR-0007 calls for: it owns the message body and delegates transport to the
// platform Mailer.
type LinkMailer struct {
	mailer mailer.Mailer
}

// NewLinkMailer builds a LinkMailer over the given transport.
func NewLinkMailer(m mailer.Mailer) *LinkMailer {
	return &LinkMailer{mailer: m}
}

// SendMagicLink emails a sign-in link to the recipient.
func (l *LinkMailer) SendMagicLink(ctx context.Context, to, link string) error {
	if err := l.mailer.Send(ctx, mailer.Message{
		To:      to,
		Subject: "Your Paddock sign-in link",
		Text:    magicLinkText(link),
		HTML:    magicLinkHTML(link),
	}); err != nil {
		return fmt.Errorf("identity: send magic link: %w", err)
	}

	return nil
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
