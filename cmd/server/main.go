package main

import (
	"bufio"
	"context"
	"errors"
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
	"github.com/momja/Exhibit/internal/humanize"
	"github.com/momja/Exhibit/internal/logging"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
)

func main() {
	// A small subcommand surface, and only for what must not be configuration:
	// a password, which never becomes an environment variable (av-q30x), and
	// the accounts an operator provisions before there is a UI to provision
	// them from (av-rzvf). Everything else here is driven by the environment.
	if len(os.Args) > 1 {
		runSubcommand(os.Args[1:])
		return
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

	// The same catch-up for storage accounting (av-fw1b): lengths are recorded
	// when bytes are written, so a library that predates migration 021 would
	// report 0 B until every artifact happened to be edited. Selects only
	// blobs with no recorded length, so this is free on every start after the
	// first. Non-fatal for the same reason as above — an unmeasured library
	// under-reports a number nothing refuses on.
	if err := st.BackfillBlobSizes(context.Background(), bl); err != nil {
		slog.Warn("backfill blob sizes", slog.String("err", err.Error()))
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
	// server at all. Since av-rzvf its accounts live in the users table, so
	// there are two independent reasons an instance has a local login: the
	// environment credential below, and any account already provisioned.
	localCredential := newLocalCredential()
	localUsers, err := st.CountLocalCredentials(context.Background())
	if err != nil {
		fatal("count local accounts", err)
	}
	// An instance with no way in at all gets one (av-jviu), so a fresh
	// deployment is usable without a CLI round-trip and a restart.
	if localUsers == 0 && identity == nil && localCredential == nil {
		if err := seedDefaultAdmin(context.Background(), st); err != nil {
			fatal("create the default admin account", err)
		}
		localUsers = 1
	}
	if localUsers > 0 {
		slog.Info("local accounts present", slog.Int64("count", localUsers))
	}
	warnIfDefaultPassword(context.Background(), st)
	if identity != nil || localCredential != nil || localUsers > 0 {
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
		LocalUsers:       localUsers > 0,
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

// DefaultAdminName and DefaultAdminPassword are the account a fresh instance
// gets so that it is usable the moment it boots, with no CLI round-trip and no
// restart (av-jviu).
//
// The password is a documented constant, chosen deliberately over a generated
// one for the sake of a shorter first five minutes. The cost is real and worth
// stating plainly: between first boot and the operator changing it, anyone who
// can reach the instance can sign in as its admin. That is why
// warnIfDefaultPassword below nags on every startup rather than only on the
// one that created the account — a warning scrolled past once is not a warning.
const (
	DefaultAdminName     = "admin"
	DefaultAdminPassword = "changeme"
)

// seedDefaultAdmin creates that account. It runs only when the instance has no
// local accounts *and* no other way in: an OIDC issuer or a LOGIN_USERNAME pair
// each mean the operator has already chosen how they sign in, and quietly
// adding a guessable second door to that would be a backdoor rather than a
// convenience.
//
// Worth being explicit that "no other way in" describes an *existing*
// single-user instance as exactly as it describes an empty one, so this fires
// on the first boot after an upgrade too. That is the intended behaviour and
// not a side effect: the seeded row is user 1, the id such a library is already
// filed under, so it adopts that library rather than starting an empty one
// beside it. What it also does is put a login in front of pages that were open
// the day before, which an operator upgrading deserves to be told rather than
// to discover — deployment.md §3.2 carries that warning.
func seedDefaultAdmin(ctx context.Context, st *store.SQLiteStore) error {
	hash, err := auth.HashPassword(DefaultAdminPassword)
	if err != nil {
		return err
	}
	if _, err := st.CreateLocalUser(ctx, store.NewLocalUser{
		ExternalID: auth.LocalExternalID(DefaultAdminName), Email: DefaultAdminName, PasswordHash: hash,
	}); err != nil {
		return err
	}
	slog.Info("created the default admin account",
		slog.String("username", DefaultAdminName),
		slog.String("password", DefaultAdminPassword),
	)
	return nil
}

// warnIfDefaultPassword complains, at every startup, for as long as the seeded
// account still has its documented password. Costs one bcrypt compare per boot.
func warnIfDefaultPassword(ctx context.Context, st *store.SQLiteStore) {
	_, hash, err := st.LookupLocalCredential(ctx, auth.LocalExternalID(DefaultAdminName))
	if err != nil || hash == "" {
		return // no such account, or it has no password — nothing to warn about
	}
	if !auth.VerifyStoredPassword(hash, DefaultAdminPassword) {
		return // changed, which is the whole point
	}
	slog.Warn("the admin account still has its default password — anyone who can reach this instance can sign in as its admin",
		slog.String("username", DefaultAdminName),
		slog.String("fix", "sign in and change it, or run: user passwd "+DefaultAdminName),
	)
}

// newLocalCredential builds the environment credential, or returns nil when
// neither variable is set — the single-user default, unchanged.
//
// Since av-rzvf accounts live in the users table, and this pair is retained as
// **bootstrap and break-glass** rather than replaced. It names an account
// (LOGIN_USERNAME) and supplies an always-accepted password for it
// (LOGIN_PASSWORD_HASH): on an empty instance that creates the account, which
// by the first-user rule is the admin; on a populated one it is the way back
// into an account whose password has been lost. Vaultwarden's admin token
// plays the same role for the same reason.
//
// DECISION — it stays live permanently while it is set, rather than expiring
// once `users` is non-empty. The trade is real in both directions and was
// taken deliberately:
//
//   - Permanent means a permanent bypass of the user table: anyone who can
//     read the process environment can log in as that account, and disabling
//     the account in the database does not stop them.
//   - Expiring at the first account would remove exactly the case break-glass
//     exists for. A credential that only works while there are no users is a
//     bootstrap credential and nothing more; an operator who has locked
//     themselves out sets it, restarts, and finds it inert — with no way in
//     short of editing the database by hand.
//
// Permanent wins because the bypass it grants is one the operator already
// has. AUTH_TOKEN sits in the same environment and is already full access to
// every API route, so this adds no new class of exposure — while the recovery
// it buys is otherwise unavailable. It is also opt-in and loud: unset is the
// default and means no environment credential at all, and startup logs that
// it is enabled so it cannot be left on unnoticed. An operator who wants the
// bypass gone removes the two variables and restarts; their provisioned
// accounts keep working, which is what makes that a safe thing to do.
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
	slog.Warn("environment login credential enabled — bootstrap and break-glass; "+
		"it always accepts this password for this account, whatever the users table says",
		slog.String("username", cred.Username()))
	return cred
}

const usage = `subcommands:
  hash-password          print a LOGIN_PASSWORD_HASH value for a password read from stdin
  user list              list the accounts on this instance
  user add <name>        provision an account, password read from stdin
  user passwd <name>     change an account's password, read from stdin
  user disable <name>    stop an account signing in, and sign it out everywhere
  user enable <name>     let a disabled account sign in again
  storage usage          print stored bytes per owner, heaviest first
  storage recompute      re-measure every stored blob and rewrite the recorded sizes
`

// runSubcommand dispatches the operator-facing commands. They are subcommands
// of the server binary rather than a second tool because they need the same
// migrations, the same store, and the same hashing parameters — a separate
// binary would have to be kept in agreement with all three.
func runSubcommand(args []string) {
	switch args[0] {
	case "hash-password":
		hashPassword()
	case "user":
		userCommand(args[1:])
	case "storage":
		storageCommand(args[1:])
	default:
		fmt.Fprint(os.Stderr, usage)
		fatal("unknown argument", fmt.Errorf("%q", args[0]))
	}
}

// userCommand provisions the accounts av-rzvf moved into the database. It is
// the only way to create the second account on an instance until the admin UI
// lands (av-utap), and it stays useful after: it is what an operator with shell
// access has when nobody can log in.
//
// It opens the same database the server does, which runs migrations — so
// running it against a fresh volume initializes the schema, and an account can
// be provisioned before the server's first start.
func userCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		fatal("user", fmt.Errorf("needs a subcommand"))
	}
	// Warn, not the server's info: the operator asked one question and the
	// answer is on stdout. A migration that has to run is still reported by
	// failing, which is the only part of it they can act on.
	logging.Configure(slog.LevelWarn)
	dataDir := getenv("DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fatal("create data dir", err)
	}
	st, err := store.OpenSQLite(dataDir + "/app.db")
	if err != nil {
		fatal("open store", err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()

	name := ""
	if len(args) > 1 {
		name = auth.NormalizeLoginName(args[1])
	}
	switch args[0] {
	case "list":
		users, err := st.ListUsers(ctx)
		if err != nil {
			fatal("list users", err)
		}
		if len(users) == 0 {
			fmt.Fprintln(os.Stderr, "no accounts yet — the first login on this instance becomes its admin")
			return
		}
		for _, u := range users {
			kind := "sso"
			if u.HasPassword {
				kind = "local"
			}
			role := "user"
			if u.IsAdmin {
				role = "admin"
			}
			// The status column is the reason a disabled account does not
			// simply vanish from this list: it is still an owner id holding a
			// library, and the operator has to be able to see it to bring it
			// back (av-utap).
			state := "active"
			if u.Disabled {
				state = "disabled"
			}
			fmt.Printf("%d\t%s\t%s\t%s\t%s\n", u.ID, u.Email, kind, role, state)
		}
	case "add":
		if name == "" {
			fatal("user add", fmt.Errorf("needs a login name"))
		}
		user, err := st.CreateLocalUser(ctx, store.NewLocalUser{
			ExternalID: auth.LocalExternalID(name), Email: name, PasswordHash: hashFromStdin(name),
		})
		if errors.Is(err, store.ErrDuplicateName) {
			fatal("create user", fmt.Errorf("%q already has an account — `user passwd` changes its password", name))
		} else if err != nil {
			fatal("create user", err)
		}
		// Said out loud because it is the one thing about a new account that
		// is not obvious from having typed the command, and because on an
		// empty instance the first `user add` is what makes the admin.
		if user.IsAdmin {
			fmt.Fprintf(os.Stderr, "created %s (owner id %d) — the first account on an instance is its admin\n",
				user.Email, user.ID)
			fmt.Fprintln(os.Stderr, "restart the server so it starts requiring a login")
		} else {
			fmt.Fprintf(os.Stderr, "created %s (owner id %d)\n", user.Email, user.ID)
		}
	case "passwd":
		if name == "" {
			fatal("user passwd", fmt.Errorf("needs a login name"))
		}
		user, _, err := st.LookupLocalCredential(ctx, auth.LocalExternalID(name))
		if err != nil {
			fatal("find user", fmt.Errorf("%q has no local account — `user add` creates one", name))
		}
		if err := st.SetLocalPassword(ctx, user.ID, hashFromStdin(name)); err != nil {
			fatal("set password", err)
		}
		fmt.Fprintf(os.Stderr, "password changed for %s\n", user.Email)
	case "disable", "enable":
		// The CLI half of av-utap's admin screen, and the half that still
		// works when nobody can log in. It reaches the same store call the
		// API handler does — so disabling from a shell revokes the account's
		// live sessions exactly as disabling from the page does, and the
		// last-admin refusal is the same refusal.
		setUserDisabled(ctx, st, name, args[0] == "disable")
	default:
		fmt.Fprint(os.Stderr, usage)
		fatal("user", fmt.Errorf("unknown subcommand %q", args[0]))
	}
}

// storageCommand is av-fw1b's operator surface: what is on this disk, and the
// repair when the recorded numbers stop matching it.
//
// It is a subcommand rather than an API route for the same reason `user` is —
// it is what somebody with shell access has — and because recompute reads
// every stored byte, which is a shape that belongs in a command an operator
// starts deliberately, not on a request path.
func storageCommand(args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		fatal("storage", fmt.Errorf("needs a subcommand"))
	}
	logging.Configure(slog.LevelWarn)
	dataDir := getenv("DATA_DIR", "./data")
	st, err := store.OpenSQLite(dataDir + "/app.db")
	if err != nil {
		fatal("open store", err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()

	switch args[0] {
	case "usage":
		owners, err := st.ListStorageUsage(ctx)
		if err != nil {
			fatal("storage usage", err)
		}
		if len(owners) == 0 {
			fmt.Fprintln(os.Stderr, "nothing stored")
			return
		}
		for _, o := range owners {
			fmt.Printf("owner %-6d %10s  %d blobs\n", o.OwnerID, humanize.Bytes(o.Bytes), o.Blobs)
		}
		// Counted over distinct blobs rather than by adding the lines above.
		// A blob two owners reference is charged in full to each of them —
		// which is right for "what is this owner holding" and wrong for "what
		// is on this disk", and this line is the second question.
		blobs, total, err := st.StoredBytes(ctx)
		if err != nil {
			fatal("storage usage", err)
		}
		fmt.Printf("%-12s %10s  %d blobs stored\n", "on disk", humanize.Bytes(total), blobs)
	case "recompute":
		// Every owner the schema can name, including owner 1 on a
		// single-user instance, which has no users row to enumerate — so
		// the owners come from what is stored, not from the directory.
		owners, err := st.ListStorageOwners(ctx)
		if err != nil {
			fatal("storage recompute", err)
		}
		bl, err := blob.NewFSStore(dataDir + "/blobs")
		if err != nil {
			fatal("open blob store", err)
		}
		for _, ownerID := range owners {
			res, err := st.RecomputeStorageUsage(ctx, ownerID, bl)
			if err != nil {
				fatal("storage recompute", err)
			}
			line := fmt.Sprintf("owner %-6d %10s  %d blobs measured", ownerID, humanize.Bytes(res.Bytes), res.Blobs)
			if res.Unreadable > 0 {
				// Named rather than folded into the count: these kept the
				// size they already had, so the total below is only as
				// correct as those older measurements.
				line += fmt.Sprintf(", %d unreadable (size left as recorded)", res.Unreadable)
			}
			fmt.Println(line)
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		fatal("storage", fmt.Errorf("unknown subcommand %q", args[0]))
	}
}

// setUserDisabled switches an account off or back on by login name.
//
// It resolves the name through the users table rather than
// LookupLocalCredential, because an account with no password is exactly one an
// operator may need to disable: an identity a provider issued has no hash to
// remove, which is why disabling is a column at all (migration 017).
func setUserDisabled(ctx context.Context, st *store.SQLiteStore, name string, disabled bool) {
	verb := "enable"
	if disabled {
		verb = "disable"
	}
	if name == "" {
		fatal("user "+verb, fmt.Errorf("needs a login name"))
	}
	user, err := st.GetUserByExternalID(ctx, auth.LocalExternalID(name))
	if errors.Is(err, store.ErrNotFound) {
		fatal("find user", fmt.Errorf("%q has no account on this instance — `user list` shows the ones that do", name))
	} else if err != nil {
		fatal("find user", err)
	}
	switch err := st.SetUserDisabled(ctx, user.ID, disabled); {
	case errors.Is(err, store.ErrLastAdmin):
		fatal("user "+verb, fmt.Errorf(
			"%q is the last admin who can still sign in — promote or enable another account first", name))
	case err != nil:
		fatal("user "+verb, err)
	}
	if disabled {
		fmt.Fprintf(os.Stderr, "disabled %s — signed out everywhere, and no further sign-in. `user enable %s` undoes it.\n",
			user.Email, name)
		return
	}
	fmt.Fprintf(os.Stderr, "enabled %s — they can sign in again with their existing password\n", user.Email)
}

// hashFromStdin reads a password and returns its hash, so the plaintext lives
// in one local variable and never reaches a store call, a log line, or an
// error string.
func hashFromStdin(who string) string {
	hash, err := auth.HashPassword(readPassword("Enter a password for " + who + ", then press Enter and ctrl-D:"))
	if err != nil {
		fatal("hash password", err)
	}
	return hash
}

// hashPassword prints the LOGIN_PASSWORD_HASH value for a password read from
// stdin.
//
//	docker compose run --rm app hash-password
//
// Everything printed to stderr is guidance; the single line on stdout is the
// hash, so the command pipes cleanly.
func hashPassword() {
	hash, err := auth.HashPassword(readPassword("Enter the password, then press Enter and ctrl-D:"))
	if err != nil {
		fatal("hash password", err)
	}
	fmt.Fprintln(os.Stderr, "\nSet this as LOGIN_PASSWORD_HASH (with LOGIN_USERNAME):")
	fmt.Println(hash)
}

// readPassword takes a password from stdin rather than an argument, so it never
// lands in a shell history, a process list, or this container's own argv.
func readPassword(prompt string) string {
	fmt.Fprintln(os.Stderr, prompt)
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
	return password
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
