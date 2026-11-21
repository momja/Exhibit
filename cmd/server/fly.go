package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// serveFly keeps both processes on the one volume-backed Fly machine. A Fly
// process group per binary would create separate machines, not a local satellite.
// Either child exiting stops its sibling and fails the supervisor for restart.
func serveFly() error {
	if os.Getenv("SINGLE_LISTENER") != "1" || os.Getenv("ADDR") != ":8080" || os.Getenv("MCP_PROXY_URL") != "http://127.0.0.1:8090" {
		return fmt.Errorf("fly requires SINGLE_LISTENER=1, ADDR=:8080 and MCP_PROXY_URL=http://127.0.0.1:8090")
	}
	origin := os.Getenv("APP_ORIGIN")
	if origin == "" || os.Getenv("EXHIBIT_PUBLIC_URL") != origin || os.Getenv("MCP_URL") != origin+"/mcp" || os.Getenv("MCP_PUBLIC_URL") != os.Getenv("MCP_URL") {
		return fmt.Errorf("fly requires EXHIBIT_PUBLIC_URL=APP_ORIGIN and MCP_URL=MCP_PUBLIC_URL=APP_ORIGIN/mcp")
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	core := exec.Command(binary)
	core.Env = os.Environ()
	satellite := exec.Command("/exhibit-mcp")
	// The satellite gets no operator secrets, database config or provider keys.
	// Both processes still share a machine/OS identity; this is environment hygiene.
	satellite.Env = []string{
		"EXHIBIT_URL=http://127.0.0.1:8080",
		"EXHIBIT_PUBLIC_URL=" + origin,
		"MCP_PUBLIC_URL=" + os.Getenv("MCP_PUBLIC_URL"),
		"MCP_ADDR=127.0.0.1:8090",
		"MAX_REQUEST_BODY_BYTES=" + os.Getenv("MAX_REQUEST_BODY_BYTES"),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	return superviseFly(ctx, core, satellite)
}

func superviseFly(ctx context.Context, children ...*exec.Cmd) error {
	exited := make(chan error, len(children))
	started := []*exec.Cmd{}
	defer func() {
		for _, child := range started {
			_ = child.Process.Signal(syscall.SIGTERM)
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for range started {
			select {
			case <-exited:
			case <-timer.C:
				for _, child := range started {
					_ = child.Process.Kill()
				}
				// Wait goroutines reap all children; killed children cannot hold shutdown.
				return
			}
		}
	}()
	for _, child := range children {
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			return err
		}
		started = append(started, child)
		go func() { exited <- child.Wait() }()
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-exited:
		// Preserve the completion for the shutdown drain above.
		exited <- err
		return fmt.Errorf("Fly child exited: %v", err)
	}
}
