package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/api"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/logging"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
)

func main() {
	// One subcommand, and only because the credential it produces must be
	// computed somewhere the password itself never becomes configuration
	// (av-q30x). Everything else this binary does is driven by the environment.
	if len(os.Args) > 1 {
		if os.Args[1] == "hash-password" {
			hashPassword()
			return
		}
		fatal("unknown argument", fmt.Errorf("%q — the only subcommand is `hash-password`", os.Args[1]))
	}

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

	// Public instance mode (av-4ac9): configuration only — it changes no
	// authentication behaviour on its own. Read after logging is configured,
	// because a malformed knob is reported rather than silently fixed up.
	publicMode := api.PublicModeFromEnv()

	slog.Info("exhibit starting",
		slog.String("app_origin", appOrigin),
		slog.String("render_origin", renderOrigin),
		slog.String("addr", addr),
		slog.String("render_addr", renderAddr),
		slog.String("log_level", levelName(level)),
		slog.Bool("debug", level <= slog.LevelDebug),
		slog.Bool("public_mode", publicMode.Enabled),
	)
	if publicMode.Enabled {
		slog.Info("public instance mode enabled",
			slog.String("instance_name", publicMode.Name),
			slog.Int64("public_owner_id", publicMode.OwnerID),
		)
	}

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
	// Agent sessions authenticate with a per-session credential scoped to one
	// artifact, never the service token (av-e0yj). One registry, shared: the
	// manager issues from it, the API resolves and enforces against it.
	agentCreds := agentscope.NewRegistry()
	var agentMgr *agent.Manager
	piBin := getenv("PI_BIN", "pi")
	if path, err := exec.LookPath(piBin); err != nil {
		slog.Warn("pi binary not found; agent support disabled", slog.String("pi_bin", piBin))
	} else {
		agentMgr, err = agent.New(agent.Config{
			PiBin:       path,
			WorkRoot:    dataDir + "/agent",
			APIBaseURL:  appOrigin,
			Credentials: agentCreds,
			MockLLMURL:  mockLLMURL,
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
	// The second login path (av-q30x), and the one that needs no identity
	// server at all. Either, both, or neither may be configured; the session
	// gate arms on either.
	localCredential := newLocalCredential()
	if identity != nil || localCredential != nil {
		if n, err := st.DeleteExpiredSessions(context.Background()); err != nil {
			slog.Warn("prune expired sessions", slog.String("err", err.Error()))
		} else if n > 0 {
			slog.Info("pruned expired sessions", slog.Int64("count", n))
		}
	}

	router := api.NewRouter(api.Config{
		Store:            st,
		Blob:             bl,
		AppOrigin:        appOrigin,
		RenderOrigin:     renderOrigin,
		AuthToken:        authToken,
		Agent:            agentMgr,
		AgentCredentials: agentCreds,
		Secrets:          box,
		MockEnabled:      mockLLMURL != "",
		Identity:         identity,
		LocalCredential:  localCredential,
		Public:           publicMode,
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

// newLocalCredential builds the instance's own username/password login from
// the environment, or returns nil when neither variable is set — the
// single-user default, unchanged.
//
// LOGIN_PASSWORD_HASH takes a **bcrypt hash, not a password**, and that is the
// one piece of friction this feature deliberately keeps. Accepting a plaintext
// password and hashing it at startup would be theatre: the plaintext would be
// sitting in the process environment, in whatever file set it, and in
// `docker inspect` — so the hash would protect nothing that was not already
// exposed beside it. Requiring the hash means the password exists only in the
// operator's head and in one bcrypt string that is useless anywhere else, which
// matters because a password (unlike AUTH_TOKEN) is the kind of secret people
// reuse across services. `server hash-password` produces it.
//
// Setting one variable without the other is fatal rather than ignored. The
// failure mode it prevents is the expensive one: a typo'd variable name
// silently leaving the instance with no login at all, which looks exactly like
// success until someone else finds the library.
func newLocalCredential() *auth.Credential {
	username, hash := os.Getenv("LOGIN_USERNAME"), os.Getenv("LOGIN_PASSWORD_HASH")
	if username == "" && hash == "" {
		return nil
	}
	cred, err := auth.NewCredential(username, hash)
	if err != nil {
		fatal("configure local login", err)
	}
	slog.Info("local login configured", slog.String("username", cred.Username()))
	return cred
}

// hashPassword prints the LOGIN_PASSWORD_HASH value for a password read from
// stdin.
//
// Stdin rather than an argument, so the password never lands in a shell
// history, a process list, or this container's own argv:
//
//	docker compose run --rm app hash-password
//
// then type it and press ctrl-D. Everything printed to stderr is guidance; the
// single line on stdout is the hash, so the command pipes cleanly.
func hashPassword() {
	fmt.Fprintln(os.Stderr, "Enter the password, then press Enter and ctrl-D:")
	// Read every line and take the first: a terminal sends the trailing
	// newline, and a here-doc or an echo pipe may send more than one.
	scanner := bufio.NewScanner(os.Stdin)
	var password string
	if scanner.Scan() {
		password = strings.TrimRight(scanner.Text(), "\r")
	}
	if err := scanner.Err(); err != nil {
		fatal("read password", err)
	}
	if password == "" {
		fatal("read password", fmt.Errorf("empty"))
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fatal("hash password", err)
	}
	fmt.Fprintln(os.Stderr, "\nSet this as LOGIN_PASSWORD_HASH (with LOGIN_USERNAME):")
	fmt.Println(hash)
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
