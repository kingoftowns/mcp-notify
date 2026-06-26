package config

import (
	"os"
	"testing"
)

func TestConfig_Load_Success(t *testing.T) {
	os.Setenv("GMAIL_USERNAME", "test@example.com")
	os.Setenv("GMAIL_APP_PASSWORD", "app-password-123")
	os.Setenv("NOTIFY_RECIPIENT", "recipient@example.com")
	defer os.Unsetenv("GMAIL_USERNAME")
	defer os.Unsetenv("GMAIL_APP_PASSWORD")
	defer os.Unsetenv("NOTIFY_RECIPIENT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected PORT 8080, got %q", cfg.Port)
	}
	if cfg.GmailUsername != "test@example.com" {
		t.Errorf("expected test@example.com, got %q", cfg.GmailUsername)
	}
	if cfg.GmailAppPassword != "app-password-123" {
		t.Errorf("expected app-password-123, got %q", cfg.GmailAppPassword)
	}
	if cfg.NotifyRecipient != "recipient@example.com" {
		t.Errorf("expected recipient@example.com, got %q", cfg.NotifyRecipient)
	}
}

func TestConfig_Load_MissingRequired(t *testing.T) {
	// Only set GMAIL_USERNAME, leave GMAIL_APP_PASSWORD and NOTIFY_RECIPIENT unset
	os.Setenv("GMAIL_USERNAME", "test@example.com")
	defer os.Unsetenv("GMAIL_USERNAME")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error due to missing required fields, got nil")
	}
}

func TestConfig_Load_DefaultPort(t *testing.T) {
	os.Setenv("GMAIL_USERNAME", "test@example.com")
	os.Setenv("GMAIL_APP_PASSWORD", "app-password-123")
	os.Setenv("NOTIFY_RECIPIENT", "recipient@example.com")
	defer os.Unsetenv("GMAIL_USERNAME")
	defer os.Unsetenv("GMAIL_APP_PASSWORD")
	defer os.Unsetenv("NOTIFY_RECIPIENT")
	// Ensure PORT is unset
	os.Unsetenv("PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default PORT 8080, got %q", cfg.Port)
	}
}
