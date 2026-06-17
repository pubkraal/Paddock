package mailer

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// mimeBoundary separates the alternative parts of a multipart message. A fixed
// value is fine: it only needs to not occur in the body, and our bodies are
// short transactional text we control.
const mimeBoundary = "paddock-boundary-2f8a1c"

// sendFunc matches net/smtp.SendMail so the real sender is injected in
// production and a fake is injected in tests (see export_test.go), keeping Send
// coverable without a socket.
type sendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error

// SMTPMailer sends over SMTP. In dev it targets the Mailpit container on
// localhost:1025 (no auth, no TLS); the same adapter works against any relay.
type SMTPMailer struct {
	addr string
	from string
	send sendFunc
}

// NewSMTPMailer returns an SMTPMailer that delivers via the given SMTP address,
// using the given envelope/From address.
func NewSMTPMailer(addr, from string) *SMTPMailer {
	return &SMTPMailer{addr: addr, from: from, send: smtp.SendMail}
}

// Send builds an RFC 822 message from msg and transmits it. No SMTP auth is
// used (Mailpit and internal relays accept unauthenticated submission); add an
// smtp.Auth here when a relay requires it.
func (m *SMTPMailer) Send(_ context.Context, msg Message) error {
	if msg.To == "" {
		return errMissingRecipient
	}

	body := buildMessage(m.from, msg)

	if err := m.send(m.addr, nil, m.from, []string{msg.To}, body); err != nil {
		return fmt.Errorf("mailer: smtp send: %w", err)
	}

	return nil
}

func buildMessage(from string, msg Message) []byte {
	var b strings.Builder

	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", msg.Subject)
	b.WriteString("MIME-Version: 1.0\r\n")

	if msg.HTML == "" {
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n")
		b.WriteString(msg.Text)
		b.WriteString("\r\n")

		return []byte(b.String())
	}

	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", mimeBoundary)
	writePart(&b, "text/plain", msg.Text)
	writePart(&b, "text/html", msg.HTML)
	fmt.Fprintf(&b, "--%s--\r\n", mimeBoundary)

	return []byte(b.String())
}

func writePart(b *strings.Builder, contentType, content string) {
	fmt.Fprintf(b, "--%s\r\n", mimeBoundary)
	fmt.Fprintf(b, "Content-Type: %s; charset=\"utf-8\"\r\n\r\n", contentType)
	b.WriteString(content)
	b.WriteString("\r\n")
}
