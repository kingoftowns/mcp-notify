package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kingoftowns/mcp-notify/internal/config"
	"github.com/kingoftowns/mcp-notify/internal/mcpserver"
	"github.com/kingoftowns/mcp-notify/internal/notify"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	renderer := notify.NewRenderer()
	emailChannel := notify.NewEmailChannel(cfg)

	svc := &notify.NotificationService{
		R:         renderer,
		C:         emailChannel,
		Recipient: cfg.NotifyRecipient,
	}

	srv := mcpserver.NewServer(svc)
	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return srv },
		nil, // default options
	)

	mux := http.NewServeMux()
	mux.Handle("GET /mcp", handler)
	mux.Handle("POST /mcp", handler)

	addr := ":" + cfg.Port
	log.Printf("mcp-notify listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}

	fmt.Println("mcp-notify server stopped")
}
