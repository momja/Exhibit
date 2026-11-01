// Command exhibit-mcp runs the optional Streamable HTTP MCP service.
// Configure it with EXHIBIT_URL, MCP_PUBLIC_URL, MCP_ADDR and
// MAX_REQUEST_BODY_BYTES. Each request supplies its own library grant;
// the service holds no authority, database or secrets of its own.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: exhibit-mcp")
		os.Exit(2)
	}
	if err := serve(); err != nil {
		slog.Error("exhibit-mcp stopped", "err", err)
		os.Exit(1)
	}
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}
