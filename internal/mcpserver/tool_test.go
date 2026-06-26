package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kingoftowns/mcp-notify/internal/notify"
)

// fakeChannel implements notify.Channel for testing.
type fakeChannel struct {
	sendFunc func(context.Context, notify.Rendered) (string, error)
}

func (ch *fakeChannel) Send(ctx context.Context, r notify.Rendered) (string, error) {
	return ch.sendFunc(ctx, r)
}

func TestHandler_SuccessResult(t *testing.T) {
	knownID := "test-message-id-12345"
	ch := &fakeChannel{
		sendFunc: func(_ context.Context, r notify.Rendered) (string, error) {
			if r.Subject == "" {
				t.Error("expected non-empty subject")
			}
			return knownID, nil
		},
	}

	svc := &notify.NotificationService{
		R:         notify.NewRenderer(),
		C:         ch,
		Recipient: "recipient@example.com",
	}

	ctx := context.Background()
	input := SendInput{
		Subject: "test notification",
		Body:    "Hello **world**",
		Status:  "completed",
	}

	output, err := sendHandler(ctx, svc, input)
	if err != nil {
		t.Fatalf("sendHandler() error = %v", err)
	}

	if !strings.Contains(output.Message, "recipient@example.com") {
		t.Errorf("output.Message should contain recipient, got: %q", output.Message)
	}
	if !strings.Contains(output.Message, knownID) {
		t.Errorf("output.Message should contain message ID %q, got: %q", knownID, output.Message)
	}
	// Verify the RFC3339 timestamp actually parses
	parts := strings.Split(output.Message, ",")
	if len(parts) >= 2 {
		tsStr := strings.TrimSpace(parts[len(parts)-1])
		if _, err := time.Parse(time.RFC3339, tsStr); err != nil {
			t.Errorf("expected RFC3339 timestamp, got %q: %v", tsStr, err)
		}
	} else {
		t.Errorf("expected comma-separated timestamp in message, got: %q", output.Message)
	}
}

func TestHandler_ErrorWraps(t *testing.T) {
	ch := &fakeChannel{
		sendFunc: func(_ context.Context, r notify.Rendered) (string, error) {
			return "", errors.New("smtp connection refused") // some sentinel error
		},
	}

	svc := &notify.NotificationService{
		R:         notify.NewRenderer(),
		C:         ch,
		Recipient: "recipient@example.com",
	}

	ctx := context.Background()
	input := SendInput{
		Subject: "test",
		Body:    "body",
	}

	_, err := sendHandler(ctx, svc, input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "email send failed") {
		t.Errorf("expected error to contain 'email send failed', got: %v", err)
	}
}
