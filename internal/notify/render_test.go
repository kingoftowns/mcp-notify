package notify

import (
	"strings"
	"testing"
)

func TestRender_SanitizesScript(t *testing.T) {
	r := newRenderer()

	// Use markdown-native input — goldmark suppresses raw HTML.
	// The body has safe markdown content + an explicit <script> attempt.
	// Goldmark omits the raw HTML; bluemonday should also not inject it.
	result, err := r.Render("test", "hello **world**\n\n<script>alert(\"xss\")</script>", "")
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if strings.Contains(result.HTMLBody, "<script>") {
		t.Error("expected <script> tag to be sanitized, but found it in output")
	}
	// Goldmark renders **world** as <strong>, so bluemonday preserves it passed through template
	if !strings.Contains(result.HTMLBody, "hello") {
		t.Error("expected markdown text content to be preserved")
	}
}

func TestRender_StripsCRLF(t *testing.T) {
	r := newRenderer()
	maliciousSubject := "hi\r\nBcc: evil@x.com"

	result, err := r.Render(maliciousSubject, "body text", "")
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if strings.Contains(result.Subject, "\r") {
		t.Error("subject contains carriage return after sanitization")
	}
	if strings.Contains(result.Subject, "\n") {
		t.Error("subject contains newline after sanitization")
	}
	if !strings.HasPrefix(result.Subject, "[Info] hi") {
		t.Errorf("expected subject to start with '[Info] hi', got %q", result.Subject)
	}
}

func TestRender_StatusTag(t *testing.T) {
	r := newRenderer()

	tests := []struct {
		name       string
		status     string
		wantPrefix string
	}{
		{"completed status", "completed", "[Completed]"},
		{"waiting status", "waiting", "[Waiting]"},
		{"error status", "error", "[Error]"},
		{"info status", "info", "[Info]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.Render("test subject", "body", tt.status)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if !strings.HasPrefix(result.Subject, tt.wantPrefix) {
				t.Errorf("expected subject prefix %q, got %q", tt.wantPrefix, result.Subject)
			}
			// Also verify the status tag appears in the HTML banner
			if !strings.Contains(result.HTMLBody, tt.wantPrefix) &&
				!strings.Contains(result.HTMLBody, strings.Trim(tt.wantPrefix, "[]")) {
				t.Errorf("expected status label %q to appear in HTML body", tt.wantPrefix)
			}
		})
	}
}

func TestRender_DefaultInfo(t *testing.T) {
	r := newRenderer()

	// Empty status should default to info
	result, err := r.Render("test", "body", "")
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.HasPrefix(result.Subject, "[Info]") {
		t.Errorf("expected empty status to default to [Info], got %q", result.Subject)
	}

	// Unknown status should also default to info
	result2, err := r.Render("test", "body", "unknown_status_xyz")
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.HasPrefix(result2.Subject, "[Info]") {
		t.Errorf("expected unknown status to default to [Info], got %q", result2.Subject)
	}
}

func TestRender_InlineStyles(t *testing.T) {
	r := newRenderer()

	result, err := r.Render("test", "hello world", "completed")
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Banner should have inline style with the completed color (#16a34a = green)
	if !strings.Contains(result.HTMLBody, "#16a34a") &&
		!containsStyleWithColor(result.HTMLBody, "green") {
		t.Error("expected completed status to produce green banner color in inline styles")
	}

	// Should have some inline style attribute
	if !strings.Contains(result.HTMLBody, "style=") {
		t.Error("expected inline style attributes in HTML output")
	}

	// Footer should contain "via mcp-notify"
	if !strings.Contains(result.HTMLBody, "via mcp-notify") {
		t.Error("expected footer to contain 'via mcp-notify'")
	}
}

// containsStyleWithColor checks if the HTML has a style attribute containing a color-related value.
// Helper to keep tests readable while being robust to specific color hex values.
func containsStyleWithColor(html, colorHint string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, colorHint)
}
