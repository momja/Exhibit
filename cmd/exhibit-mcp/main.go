// Command exhibit-mcp is a stdio Model Context Protocol server for one Exhibit
// artifact (Exh-q6xo). An agent such as Claude Code launches it with two
// environment variables:
//
//	EXHIBIT_URL    the instance's app origin, e.g. https://exhibit.example.com
//	EXHIBIT_TOKEN  an agent token the artifact's owner issued from its page
//
// It is its own small binary rather than a subcommand of the server, because
// it runs on the agent's machine, not the instance's: it imports none of the
// server, carries none of its assets, and installs with
//
//	go install github.com/momja/Exhibit/cmd/exhibit-mcp@latest
//
// It holds no authority of its own. Every tool is an HTTP request made with
// the token, which the instance resolves to one artifact and refuses outside.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/momja/Exhibit/internal/mcpserver"
)

func main() {
	// stdout is the protocol; anything human goes to stderr.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	base, token := os.Getenv("EXHIBIT_URL"), os.Getenv("EXHIBIT_TOKEN")
	if base == "" || token == "" {
		fmt.Fprintln(os.Stderr, "exhibit-mcp: set EXHIBIT_URL to the Exhibit instance and EXHIBIT_TOKEN to an agent token "+
			"issued from the artifact's page (More, Connect your own agent)")
		os.Exit(2)
	}
	api := &mcpserver.HTTPAPI{BaseURL: base, Token: token, Client: &http.Client{Timeout: 2 * time.Minute}}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := mcpserver.New(api, version()).Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		slog.Error("exhibit-mcp stopped", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}
