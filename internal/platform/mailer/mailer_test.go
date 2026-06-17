package mailer_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/smtp"
	"strings"
	"testing"

	"github.com/pubkraal/paddock/internal/platform/mailer"
)

func testMessage(opts ...func(*mailer.Message)) mailer.Message {
	m := mailer.Message{
		To:      "press@example.test",
		Subject: "Your magic link",
		Text:    "Open this link to sign in.",
		HTML:    "<p>Open this link to sign in.</p>",
	}

	for _, opt := range opts {
		opt(&m)
	}

	return m
}

func TestLogMailer_Send(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	m := mailer.NewLogMailer(logger)

	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"press@example.test", "Your magic link"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output %q missing %q", out, want)
		}
	}
}

func TestLogMailer_SendNoRecipient(t *testing.T) {
	t.Parallel()

	m := mailer.NewLogMailer(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))

	err := m.Send(context.Background(), testMessage(func(msg *mailer.Message) { msg.To = "" }))
	if err == nil {
		t.Fatal("expected error for missing recipient, got nil")
	}
}

func TestNewSMTPMailer_ValidatesBeforeNetwork(t *testing.T) {
	t.Parallel()

	// The production constructor wires the real sender; an empty recipient is
	// rejected before any network call, so this exercises it without a socket.
	m := mailer.NewSMTPMailer("localhost:1025", "no-reply@paddock.test")

	if err := m.Send(context.Background(), mailer.Message{}); err == nil {
		t.Fatal("expected validation error for empty recipient")
	}
}

func TestSMTPMailer_SendComposesAndTransmits(t *testing.T) {
	t.Parallel()

	var (
		gotAddr string
		gotFrom string
		gotTo   []string
		gotMsg  []byte
	)

	send := func(addr string, _ smtp.Auth, from string, to []string, body []byte) error {
		gotAddr, gotFrom, gotTo, gotMsg = addr, from, to, body

		return nil
	}

	m := mailer.NewTestSMTPMailer("localhost:1025", "no-reply@paddock.test", send)

	if err := m.Send(context.Background(), testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotAddr != "localhost:1025" {
		t.Errorf("addr = %q, want localhost:1025", gotAddr)
	}

	if gotFrom != "no-reply@paddock.test" {
		t.Errorf("from = %q, want no-reply@paddock.test", gotFrom)
	}

	if len(gotTo) != 1 || gotTo[0] != "press@example.test" {
		t.Errorf("to = %v, want [press@example.test]", gotTo)
	}

	msg := string(gotMsg)
	for _, want := range []string{
		"From: no-reply@paddock.test\r\n",
		"To: press@example.test\r\n",
		"Subject: Your magic link\r\n",
		"multipart/alternative",
		"text/plain",
		"text/html",
		"Open this link to sign in.",
		"<p>Open this link to sign in.</p>",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n--- message ---\n%s", want, msg)
		}
	}
}

func TestSMTPMailer_SendTextOnly(t *testing.T) {
	t.Parallel()

	var gotMsg []byte

	send := func(_ string, _ smtp.Auth, _ string, _ []string, body []byte) error {
		gotMsg = body

		return nil
	}

	m := mailer.NewTestSMTPMailer("localhost:1025", "no-reply@paddock.test", send)

	err := m.Send(context.Background(), testMessage(func(msg *mailer.Message) { msg.HTML = "" }))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg := string(gotMsg)
	if strings.Contains(msg, "multipart/alternative") {
		t.Errorf("text-only message should not be multipart:\n%s", msg)
	}

	if !strings.Contains(msg, "Content-Type: text/plain") {
		t.Errorf("text-only message missing text/plain part:\n%s", msg)
	}
}

func TestSMTPMailer_SendNoRecipient(t *testing.T) {
	t.Parallel()

	called := false
	send := func(_ string, _ smtp.Auth, _ string, _ []string, _ []byte) error {
		called = true

		return nil
	}

	m := mailer.NewTestSMTPMailer("localhost:1025", "no-reply@paddock.test", send)

	if err := m.Send(context.Background(), testMessage(func(msg *mailer.Message) { msg.To = "" })); err == nil {
		t.Fatal("expected error for missing recipient, got nil")
	}

	if called {
		t.Error("send must not be called when the recipient is missing")
	}
}

func TestSMTPMailer_SendPropagatesError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("dial tcp: connection refused")
	send := func(_ string, _ smtp.Auth, _ string, _ []string, _ []byte) error {
		return sentinel
	}

	m := mailer.NewTestSMTPMailer("localhost:1025", "no-reply@paddock.test", send)

	err := m.Send(context.Background(), testMessage())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap the underlying send error", err)
	}
}
