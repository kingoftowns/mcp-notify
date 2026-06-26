//go:build integration

package notify

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kingoftowns/mcp-notify/internal/config"
)

// TestRealSend sends a real email via Gmail SMTP.
// It is guarded by the "integration" build tag and skips gracefully when
// GMAIL_APP_PASSWORD is not set in the environment.
func TestRealSend(t *testing.T) {
	if os.Getenv("GMAIL_APP_PASSWORD") == "" {
		t.Skip("GMAIL_APP_PASSWORD not set — skipping live email test")
	}

	// Load real config from environment.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	ch := NewEmailChannel(cfg)

	rendered := Rendered{
		Subject:  "[Info] Integration test",
		TextBody: "This is a test email from mcp-notify.\n\nIf you see this, the email channel works.",
		HTMLBody: "<p>This is a <strong>test</strong> email from mcp-notify.</p><p>If you see this, the email channel works.</p>",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	messageID, err := ch.Send(ctx, rendered)
	if err != nil {
		t.Fatalf("send real email: %v", err)
	}

	t.Logf("Email sent! Message-ID: %s", messageID)
}
