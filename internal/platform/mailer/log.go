package mailer

import (
	"context"
	"log/slog"
)

// LogMailer records sends to the application log without transmitting anything.
// It is the development and unit-test default (ADR-0007).
type LogMailer struct {
	logger *slog.Logger
}

// NewLogMailer returns a LogMailer writing to the given logger.
func NewLogMailer(logger *slog.Logger) *LogMailer {
	return &LogMailer{logger: logger}
}

// Send logs the recipient and subject and returns nil.
func (m *LogMailer) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return errMissingRecipient
	}

	m.logger.InfoContext(ctx, "mail send (log only)", "to", msg.To, "subject", msg.Subject)

	return nil
}
