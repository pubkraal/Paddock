// Package mailer is the platform email primitive (ADR-0007): a narrow Mailer
// interface plus interchangeable adapters — LogMailer for dev/tests and
// SMTPMailer for the docker-compose Mailpit sink and any SMTP relay. Template
// rendering is the caller's job; domains compose a Message and call Send. The
// production EU provider (open decision O2) is a future adapter behind the same
// interface, with no call-site changes.
package mailer

import (
	"context"
	"errors"
)

// errMissingRecipient is returned by Send when a Message carries no recipient.
var errMissingRecipient = errors.New("mailer: message has no recipient")

// Message is one transactional email. Both bodies are optional individually but
// at least Text should be set; when HTML is present the message is sent as
// multipart/alternative so clients pick the richer part.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Mailer sends a single transactional Message. It is the only surface call
// sites depend on; adapters are swapped without touching them.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}
