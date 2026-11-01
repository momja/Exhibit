package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/momja/Exhibit/internal/mcpserver"
)

func serve() error {
	maxBody := mcpserver.DefaultMaxRequestBodyBytes
	if raw := os.Getenv("MAX_REQUEST_BODY_BYTES"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return fmt.Errorf("MAX_REQUEST_BODY_BYTES must be a positive integer")
		}
		maxBody = value
	}
	handler, err := mcpserver.NewHTTPHandler(mcpserver.HTTPConfig{
		BaseURL: os.Getenv("EXHIBIT_URL"), PublicURL: os.Getenv("MCP_PUBLIC_URL"),
		ExhibitPublicURL: os.Getenv("EXHIBIT_PUBLIC_URL"),
		Version:          version(), MaxRequestBodyBytes: maxBody,
	})
	if err != nil {
		return err
	}
	addr := os.Getenv("MCP_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	server := &http.Server{
		Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 5 * time.Minute, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 64 << 10,
		// No write timeout: MCP uses long-lived SSE responses.
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			// Close also interrupts idle SSE streams; sessions are ephemeral and the
			// client reconnects to initialize after the service restarts.
			_ = server.Close()
		case <-done:
		}
	}()
	slog.Info("MCP service listening", "addr", addr)
	err = server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
