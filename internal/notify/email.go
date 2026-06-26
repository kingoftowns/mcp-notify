package notify

import (
	"context"
	"fmt"

	"github.com/wneessen/go-mail"

	"github.com/kingoftowns/mcp-notify/internal/config"
)

type emailChannel struct {
	username  string
	password  string
	recipient string
}

// newEmailChannel creates a Channel backed by Gmail SMTP (smtp.gmail.com:587
// with STARTTLS). The caller provides Gmail credentials and the fixed recipient.
// NewEmailChannel creates a Channel backed by Gmail SMTP (smtp.gmail.com:587
// with STARTTLS). The caller provides Gmail credentials and the fixed recipient.
func NewEmailChannel(cfg *config.Config) Channel { return newEmailChannel(cfg) }

func newEmailChannel(cfg *config.Config) *emailChannel {
	return &emailChannel{
		username:  cfg.GmailUsername,
		password:  cfg.GmailAppPassword,
		recipient: cfg.NotifyRecipient,
	}
}

// Send delivers a rendered notification via Gmail SMTP.
// Returns the message ID assigned by go-mail's SetMessageID before send.
func (ch *emailChannel) Send(ctx context.Context, r Rendered) (string, error) {
	msg := mail.NewMsg()
	if err := msg.From(ch.username); err != nil {
		return "", fmt.Errorf("set from: %w", err)
	}
	if err := msg.To(ch.recipient); err != nil {
		return "", fmt.Errorf("set to: %w", err)
	}
	msg.Subject(r.Subject)

	// Generate and set a message ID before sending so GetMessageID returns it.
	msg.SetMessageID()

	// Multipart: plaintext body + HTML alternative
	msg.SetBodyString(mail.TypeTextPlain, r.TextBody)
	msg.AddAlternativeString(mail.TypeTextHTML, r.HTMLBody)

	client, err := mail.NewClient(
		"smtp.gmail.com",
		mail.WithPort(587),
		mail.WithSMTPAuth(mail.SMTPAuthLogin),
		mail.WithUsername(ch.username),
		mail.WithPassword(ch.password),
	)
	if err != nil {
		return "", fmt.Errorf("new smtp client: %w", err)
	}
	defer client.Close()

	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return "", fmt.Errorf("send: %w", err)
	}

	return msg.GetMessageID(), nil
}
