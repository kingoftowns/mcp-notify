package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/kingoftowns/mcp-notify/internal/notify"
)

// SendInput contains the notification fields for the send_notification tool.
// Recipient is deliberately absent — it is configured server-side (SEC-02).
type SendInput struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Status  string `json:"status,omitempty"`
}

// SendOutput is returned after a successful notification.
type SendOutput struct {
	Message string `json:"message"`
}

// sendHandler performs the notification by calling the NotificationService.
// It is used by the MCP tool handler closure registered in NewServer.
func sendHandler(ctx context.Context, svc *notify.NotificationService, input SendInput) (*SendOutput, error) {
	messageID, ts, err := svc.Notify(ctx, input.Subject, input.Body, input.Status)
	if err != nil {
		return nil, fmt.Errorf("email send failed: %w", err)
	}

	msg := fmt.Sprintf("Email delivered to %s — message-id %s, %s",
		svc.Recipient, messageID, ts.Format(time.RFC3339))
	return &SendOutput{Message: msg}, nil
}
