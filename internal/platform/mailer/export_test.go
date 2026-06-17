package mailer

import "net/smtp"

// NewTestSMTPMailer builds an SMTPMailer with an injected sender so Send is
// exercised without a real socket. The signature uses only exported types so
// the black-box _test package can supply the fake.
func NewTestSMTPMailer(addr, from string, send func(string, smtp.Auth, string, []string, []byte) error) *SMTPMailer {
	return &SMTPMailer{addr: addr, from: from, send: send}
}
