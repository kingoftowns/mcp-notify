package notify

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wneessen/go-mail"
)

// TestSend_Multipart verifies that constructing a go-mail Msg with
// From/To/Subject, SetMessageID, and the two multipart bodies (plaintext + HTML
// alternative) produces the expected headers and MIME parts — without dialing
// any network connection.
func TestSend_Multipart(t *testing.T) {
	msg := mail.NewMsg()

	if err := msg.From("sender@gmail.com"); err != nil {
		t.Fatalf("set from: %v", err)
	}
	if err := msg.To("recipient@example.com"); err != nil {
		t.Fatalf("set to: %v", err)
	}
	msg.Subject("[Test] Hello")
	msg.SetMessageID()
	msg.SetBodyString(mail.TypeTextPlain, "Hello plain")
	msg.AddAlternativeString(mail.TypeTextHTML, "<p>Hello HTML</p>")

	// Write the formatted message to a buffer (no network I/O).
	var buf bytes.Buffer
	if _, err := msg.WriteTo(&buf); err != nil {
		t.Fatalf("write msg to buffer: %v", err)
	}

	output := buf.String()

	// --- Assert key headers ---
	if !strings.Contains(output, "From: <sender@gmail.com>") {
		t.Errorf("expected From header with sender@gmail.com, got:\n%s", output)
	}
	if !strings.Contains(output, "To: <recipient@example.com>") {
		t.Errorf("expected To header with recipient@example.com, got:\n%s", output)
	}
	if !strings.Contains(output, "Subject: [Test] Hello") {
		t.Errorf("expected Subject header, got:\n%s", output)
	}
	if !strings.Contains(output, "Message-ID:") {
		t.Errorf("expected Message-ID header, got:\n%s", output)
	}
	if !strings.Contains(output, "MIME-Version: 1.0") {
		t.Errorf("expected MIME-Version header, got:\n%s", output)
	}

	// --- Assert multipart structure ---
	if !strings.Contains(output, "Content-Type: text/plain") {
		t.Errorf("expected text/plain content-type, got:\n%s", output)
	}
	if !strings.Contains(output, "Content-Type: text/html") {
		t.Errorf("expected text/html content-type, got:\n%s", output)
	}

	// Assert the plaintext body content is present
	if !strings.Contains(output, "Hello plain") {
		t.Errorf("expected plaintext body 'Hello plain', got:\n%s", output)
	}

	// Assert the HTML body content is present
	if !strings.Contains(output, "Hello HTML") {
		t.Errorf("expected HTML body 'Hello HTML', got:\n%s", output)
	}
}
