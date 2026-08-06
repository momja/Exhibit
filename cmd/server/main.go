package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/exec"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/api"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/logging"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
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

	// One-time catch-up for artifacts stored before migration 010 added
	// source_text: their bodies live in the blob store, unreachable from the
	// SQL migration itself. Non-fatal — search still works over title/tags
	// if this fails; it's an enhancement pass, not load-bearing for startup.
	if err := st.BackfillSourceText(context.Background(), bl); err != nil {
		slog.Warn("backfill artifact source text", slog.String("err", err.Error()))
	}

	// Agent support (Exh-yvhp): BYO keys are sealed with the server secret;
	// each chat session runs a `pi --mode rpc` sidecar. Degrades gracefully —
	// no pi binary just disables the agent surface.
	box, err := secrets.Load(os.Getenv("EXHIBIT_SECRET"), dataDir+"/secret.key")
	if err != nil {
		fatal("load server secret", err)
	}
	mockLLMURL := os.Getenv("MOCK_LLM_URL")
	var agentMgr *agent.Manager
	piBin := getenv("PI_BIN", "pi")
	if path, err := exec.LookPath(piBin); err != nil {
		slog.Warn("pi binary not found; agent support disabled", slog.String("pi_bin", piBin))
	} else {
		agentMgr, err = agent.New(agent.Config{
			PiBin:      path,
			WorkRoot:   dataDir + "/agent",
			APIBaseURL: appOrigin,
			AuthToken:  authToken,
			MockLLMURL: mockLLMURL,
		}, st)
		if err != nil {
			fatal("init agent manager", err)
		}
		slog.Info("agent support enabled", slog.String("pi_bin", path), slog.Bool("mock_llm", mockLLMURL != ""))
	}

	// Identity (av-30rj). Unset OIDC_ISSUER is the default and the whole of
	// the single-user path: no provider, no /auth routes, static token and
	// owner 1 as before. Set, it is the one constructor a provider swap
	// touches — everything downstream is provider-agnostic.
	identity := newIdentityProvider(context.Background(), appOrigin)
	if identity != nil {
		if n, err := st.DeleteExpiredSessions(context.Background()); err != nil {
			slog.Warn("prune expired sessions", slog.String("err", err.Error()))
		} else if n > 0 {
			slog.Info("pruned expired sessions", slog.Int64("count", n))
		}
	}

	router := api.NewRouter(api.Config{
		Store:        st,
		Blob:         bl,
		AppOrigin:    appOrigin,
		RenderOrigin: renderOrigin,
		AuthToken:    authToken,
		Agent:        agentMgr,
		Secrets:      box,
		MockEnabled:  mockLLMURL != "",
		Identity:     identity,
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

// newIdentityProvider builds the OIDC provider from the environment, or
// returns nil when OIDC_ISSUER is unset — the single-user default.
//
// Discovery runs here, at startup, off the issuer's
// /.well-known/openid-configuration. That is what makes "any OIDC provider" a
// matter of configuration rather than of code, and doing it eagerly means a
// misconfigured or unreachable issuer is a startup failure the operator sees
// immediately instead of a mystery at the first login.
func newIdentityProvider(ctx context.Context, appOrigin string) auth.IdentityProvider {
	issuer := os.Getenv("OIDC_ISSUER")
	if issuer == "" {
		return nil
	}
	provider, err := auth.NewOIDCProvider(ctx, auth.OIDCConfig{
		Issuer:       issuer,
		ClientID:     os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		// The redirect lives on the app origin, never the render origin —
		// the session cookie must not be reachable from a document that
		// runs artifact code.
		RedirectURL: appOrigin + "/auth/callback",
	})
	if err != nil {
		fatal("configure identity provider", err)
	}
	slog.Info("identity provider configured", slog.String("issuer", issuer))
	return provider
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
