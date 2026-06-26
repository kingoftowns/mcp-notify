package notify

import (
	"context"
	"time"
)

// Rendered holds the HTML and plaintext versions of a notification.
type Rendered struct {
	Subject   string
	HTMLBody  string
	TextBody  string
	StatusTag string // e.g. "[Completed]", "[Info]" — the prefix applied to subject
}

// Renderer converts markdown body + status into a formatted notification.
type Renderer interface {
	Render(subject, body, status string) (Rendered, error)
}

// Channel delivers a rendered notification (e.g. via email).
type Channel interface {
	Send(ctx context.Context, r Rendered) (messageID string, err error)
}

// NotificationService orchestrates rendering and delivery of notifications.
type NotificationService struct {
	R         Renderer
	C         Channel
	Recipient string
}

// Notify renders the notification and sends it through the channel.
func (s *NotificationService) Notify(ctx context.Context, subject, body, status string) (messageID string, ts time.Time, err error) {
	rendered, err := s.R.Render(subject, body, status)
	if err != nil {
		return "", time.Time{}, err
	}

	messageID, err = s.C.Send(ctx, rendered)
	if err != nil {
		return "", time.Time{}, err
	}

	return messageID, time.Now(), nil
}
