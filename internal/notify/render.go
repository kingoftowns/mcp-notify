package notify

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

type goldmarkRenderer struct {
	md    goldmark.Markdown
	sanit *bluemonday.Policy
	tmpl  *template.Template
}

func newRenderer() *goldmarkRenderer {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
	)
	sanit := bluemonday.UGCPolicy()

	// UGCPolicy by default strips style and class attributes, which is correct
	// for the fragment. The wrapper template owns all styling.
	sanit.RequireParseableURLs(false)

	tmpl := template.Must(template.New("email").Parse(htmlWrapper))

	return &goldmarkRenderer{
		md:    md,
		sanit: sanit,
		tmpl:  tmpl,
	}
}

// statusColor maps a status string to hex color and label prefix.
// Defaults to "info" (blue) when the status is unknown or empty.
func statusColor(status string) (color, label string) {
	switch strings.ToLower(status) {
	case "completed":
		return "#16a34a", "[Completed]"
	case "waiting":
		return "#d97706", "[Waiting]"
	case "error":
		return "#dc2626", "[Error]"
	default:
		return "#2563eb", "[Info]"
	}
}

func (r *goldmarkRenderer) Render(subject, body, status string) (Rendered, error) {
	// Determine status color and label (D-05 / D-06)
	bannerColor, statusLabel := statusColor(status)

	// Strip CRLF from subject before use (SEC-03 / D-07)
	crlfStripper := strings.NewReplacer("\r", "", "\n", "")
	safeSubject := crlfStripper.Replace(subject)
	prefixedSubject := statusLabel + " " + safeSubject

	// Render markdown body to HTML fragment
	var htmlBuf strings.Builder
	if err := r.md.Convert([]byte(body), &htmlBuf); err != nil {
		return Rendered{}, fmt.Errorf("markdown render: %w", err)
	}
	rawHTML := htmlBuf.String()

	// Sanitize the goldmark FRAGMENT only — never the assembled document
	// (UGCPolicy strips inline style, which would erase the trusted wrapper)
	sanitizedFragment := r.sanit.Sanitize(rawHTML)

	// Build the inline-styled HTML wrapper template data
	now := time.Now()
	timestamp := now.Format("Mon Jan 2 2006 15:04:05 MST")

	tmplData := struct {
		BannerColor string
		StatusLabel string
		Body        template.HTML // sanitized — safe to inject unescaped
		Timestamp   string
	}{
		BannerColor: bannerColor,
		StatusLabel: statusLabel,
		Body:        template.HTML(sanitizedFragment),
		Timestamp:   fmt.Sprintf("via mcp-notify — %s", timestamp),
	}

	var outBuf strings.Builder
	if err := r.tmpl.Execute(&outBuf, tmplData); err != nil {
		return Rendered{}, fmt.Errorf("template execute: %w", err)
	}

	return Rendered{
		Subject:   prefixedSubject,
		HTMLBody:  outBuf.String(),
		TextBody:  body, // plaintext fallback = raw markdown
		StatusTag: statusLabel,
	}, nil
}

// htmlWrapper is the inline-styled email template.
// The body fragment has already been sanitized by bluemonday UGCPolicy.
// All styling is done via inline style attributes for email-client compatibility.
const htmlWrapper = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="margin:0;padding:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:16px;line-height:1.5;color:#1f2937;background-color:#f9fafb">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#f9fafb;padding:24px 0">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1)">
<tr><td style="padding:0">
<div style="background-color:{{.BannerColor}};padding:16px 24px;text-align:center">
<span style="color:#ffffff;font-size:18px;font-weight:600">{{.StatusLabel}}</span>
</div>
</td></tr>
<tr><td style="padding:24px">
{{.Body}}
</td></tr>
<tr><td style="padding:0 24px">
<hr style="border:none;border-top:1px solid #e5e7eb;margin:0 0 16px 0">
</td></tr>
<tr><td style="padding:0 24px 24px 24px">
<p style="margin:0;font-size:13px;color:#9ca3af;text-align:center">{{.Timestamp}}</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`
