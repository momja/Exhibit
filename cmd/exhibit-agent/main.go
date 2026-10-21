// exhibit-agent is a single-assignment, storage-free Pi supervisor. It receives
// execution config over authenticated HTTP, never an Exhibit operator token.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/momja/Exhibit/internal/sprites"
)

var releaseVersion = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	address := flag.String("addr", "127.0.0.1:8090", "HTTP listen address (terminate TLS at the trusted gateway)")
	root := flag.String("work-root", "/tmp/exhibit-agent", "exclusive scratch directory, never a core data volume")
	pi := flag.String("pi", "pi", "pinned Pi executable")
	execution := flag.String("execution-id", "", "assigned execution attempt identity")
	session := flag.String("session-id", "", "assigned conversation identity")
	tokenFile := flag.String("token-file", "", "0600 file containing this assignment's worker bearer credential")
	localAssignments := flag.Bool("local-assignments", false, "authenticated static local service, accepting up to 32 isolated assignments; production Sprites use fixed identities")
	holdMode := flag.String("sprite-holds", "auto", "in-Sprite Tasks API holds: auto, required, or off")
	holdTTL := flag.Duration("hold-ttl", time.Minute, "active task hold expiry (whole seconds, 3s to 5m)")
	version := flag.Bool("version", false, "print the immutable release version")
	runtimeInfo := flag.Bool("runtime-info", false, "print protocol and pinned runtime versions")
	flag.Parse()
	if *version {
		fmt.Println(releaseVersion)
		return nil
	}
	if *runtimeInfo {
		fmt.Printf("exhibit-agent protocol=%s pi=%s node=%s\n", agentexec.ProtocolVersion, agentexec.PiVersion, agentexec.NodeVersion)
		return nil
	}
	if *tokenFile == "" {
		return errors.New("--token-file is required; supply an assigned worker credential, never an operator token")
	}
	info, err := os.Lstat(*tokenFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return errors.New("worker token must be a private regular file of at most 4096 bytes")
	}
	token, err := os.ReadFile(*tokenFile)
	if err != nil {
		return errors.New("unable to read worker authentication file")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = agentexec.CheckPiVersion(ctx, *pi)
	cancel()
	if err != nil {
		return err
	}
	lock, err := agentexec.LockScratchRoot(*root)
	if err != nil {
		return err
	}
	defer lock.Close()
	var holds agentexec.Holds
	switch *holdMode {
	case "required":
		info, err := os.Stat(sprites.TasksSocket)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			return errors.New("required in-Sprite Tasks API socket is unavailable")
		}
		holds = sprites.NewHolds()
	case "auto":
		if info, err := os.Stat(sprites.TasksSocket); err == nil && info.Mode()&os.ModeSocket != 0 {
			holds = sprites.NewHolds()
		}
	case "off":
	default:
		return errors.New("--sprite-holds must be auto, required, or off")
	}
	service, err := agentexec.NewService(agentexec.ServiceConfig{ExecutionID: *execution, SessionID: *session, Token: strings.TrimSpace(string(token)), Runner: agentexec.Local{PiBin: *pi, WorkRoot: *root}, LocalAssignments: *localAssignments, Holds: holds, HoldTTL: *holdTTL})
	if err != nil {
		return err
	}
	server := &http.Server{Addr: *address, Handler: service, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 40 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 16 << 10}
	server.SetKeepAlivesEnabled(false)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = service.Shutdown(ctx)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		workerErr := service.Shutdown(ctx)
		serverErr := server.Shutdown(ctx)
		return errors.Join(workerErr, serverErr)
	}
}
