package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kingoftowns/mcp-notify/internal/notify"
)

// NewServer creates an MCP server with a single send_notification tool.
// The server exposes no resources — only the notification tool.
func NewServer(svc *notify.NotificationService) *mcp.Server {
	impl := &mcp.Implementation{
		Name:    "mcp-notify",
		Version: "0.1.0",
	}

	server := mcp.NewServer(impl, &mcp.ServerOptions{
		// No resource, prompt, or logging capabilities — this server only
		// exposes tools. The tools capability is auto-inferred when AddTool
		// is called below.
		Capabilities: &mcp.ServerCapabilities{},
	})

	tool := &mcp.Tool{
		Name: "send_notification",
		Description: "Send an email notification. " +
			"The recipient is configured by the server administrator " +
			"and cannot be changed via this tool.",
	}

	mcp.AddTool(
		server, tool,
		func(ctx context.Context, _ *mcp.CallToolRequest, input SendInput) (*mcp.CallToolResult, SendOutput, error) {
			output, err := sendHandler(ctx, svc, input)
			if err != nil {
				return nil, SendOutput{}, err
			}
			return nil, *output, nil
		},
	)

	return server
}
