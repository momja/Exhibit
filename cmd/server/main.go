package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/artifact-viewer/artifact-viewer/internal/api"
	"github.com/artifact-viewer/artifact-viewer/internal/blob"
	"github.com/artifact-viewer/artifact-viewer/internal/logging"
	"github.com/artifact-viewer/artifact-viewer/internal/store"
)

func main() {
	dataDir := getenv("DATA_DIR", "./data")
	dbPath := dataDir + "/app.db"
	blobDir := dataDir + "/blobs"
	appOrigin := getenv("APP_ORIGIN", "http://localhost:8080")
	renderOrigin := getenv("RENDER_ORIGIN", "http://localhost:8081")
	authToken := getenv("AUTH_TOKEN", "dev-token")
	addr := getenv("ADDR", ":8080")
	renderAddr := getenv("RENDER_ADDR", ":8081")

	// Debug mode: verbose, leveled logging for test environments. Either
	// DEBUG=1 (any non-empty value) or LOG_LEVEL=debug turns it on; any
	// other LOG_LEVEL name (info/warn/error) is honored as-is. Unknown
	// levels default to info so a typo never silences the service.
	level := logging.ParseLevel(getenv("LOG_LEVEL", "info"))
	if os.Getenv("DEBUG") != "" {
		level = slog.LevelDebug
	}
	logging.Configure(level)
	slog.Info("exhibit starting",
		slog.String("app_origin", appOrigin),
		slog.String("render_origin", renderOrigin),
		slog.String("addr", addr),
		slog.String("render_addr", renderAddr),
		slog.String("log_level", levelName(level)),
		slog.Bool("debug", level <= slog.LevelDebug),
	)

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fatal("create data dir", err)
	}

	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		fatal("open store", err)
	}
	defer func() { _ = st.Close() }() // best-effort cleanup at shutdown

	bl, err := blob.NewFSStore(blobDir)
	if err != nil {
		fatal("open blob store", err)
	}

	router := api.NewRouter(api.Config{
		Store:        st,
		Blob:         bl,
		AppOrigin:    appOrigin,
		RenderOrigin: renderOrigin,
		AuthToken:    authToken,
	})

	go func() {
		slog.Info("render server listening", slog.String("addr", renderAddr))
		if err := http.ListenAndServe(renderAddr, router.RenderHandler()); err != nil {
			fatal("render server", err)
		}
	}()
	slog.Info("app server listening", slog.String("addr", addr))
	if err := http.ListenAndServe(addr, router); err != nil {
		fatal("app server", err)
	}
}

// fatal logs the error at error level and exits, mirroring log.Fatalf without
// pulling the stdlib log package into the startup path.
func fatal(msg string, err error) {
	slog.Error(msg, slog.String("err", err.Error()))
	os.Exit(1)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// levelName returns a human-readable name for a slog.Level for startup logs.
func levelName(l slog.Level) string {
	switch {
	case l <= slog.LevelDebug:
		return "debug"
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	default:
		return "info"
	}
}
