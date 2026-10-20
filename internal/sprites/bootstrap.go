package sprites

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const workerDirectory = "/home/sprite/.exhibit-worker"
const serviceName = "exhibit-worker"

// No management credentials, provider keys, tool grants, or conversation payloads
// belong in this manifest. It commits the immutable bootstrap configuration.
type manifest struct {
	Schema              int      `json:"schema"`
	Identity            Identity `json:"identity"`
	BootstrapSHA256     string   `json:"bootstrap_sha256"`
	EndpointTokenSHA256 string   `json:"endpoint_token_sha256"`
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func (p *Provisioner) bootstrapScript(identity Identity) []byte {
	release := p.config.Release
	releaseDirectory := workerDirectory + "/releases/" + release.Version + "-" + release.SHA256[:16]
	var script strings.Builder
	// curl follows only HTTPS redirects and never receives coordinator secrets.
	// Empty environment on exec keeps inherited infrastructure credentials out of
	// the supervisor and its children. The supervisor still must give Pi a minimal
	// per-execution environment, and must not forward endpoint authentication.
	fmt.Fprintf(&script, `#!/bin/sh
set -eu
umask 077
root=%s
release=%s
mkdir -p "$root/releases" "$root/scratch"
chmod 700 "$root" "$root/scratch"
if [ ! -f "$release/.release-sha256" ] || [ "$(cat "$release/.release-sha256")" != %s ]; then
  staging=$(mktemp -d "$root/releases/.install-XXXXXXXX")
  trap 'rm -rf "$staging"' EXIT HUP INT TERM
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 10 --max-time 90 %s -o "$staging/release.tar.gz"
  printf '%%s  %%s\n' %s "$staging/release.tar.gz" | sha256sum --check --status
  mkdir "$staging/runtime"
  tar -xzf "$staging/release.tar.gz" -C "$staging/runtime" --no-same-owner
  test "$(cat "$staging/runtime/EXHIBIT_RELEASE")" = %s
  test -x "$staging/runtime/bin/exhibit-agent"
  test -x "$staging/runtime/bin/node"
  test -x "$staging/runtime/bin/pi"
  test "$(env -i PATH="$staging/runtime/bin:/usr/bin:/bin" timeout 10 "$staging/runtime/bin/node" --version)" = %s
  test "$(env -i PATH="$staging/runtime/bin:/usr/bin:/bin" timeout 10 "$staging/runtime/bin/pi" --version)" = %s
  printf '%%s\n' %s > "$staging/runtime/.release-sha256"
  rm -rf "$release"
  mv "$staging/runtime" "$release"
  rm -rf "$staging"
  trap - EXIT HUP INT TERM
fi
cd "$release"
exec env -i`, shellQuote(workerDirectory), shellQuote(releaseDirectory), shellQuote(release.SHA256), shellQuote(release.URL), shellQuote(release.SHA256), shellQuote(release.Version), shellQuote("v"+release.NodeVersion), shellQuote(release.PiVersion), shellQuote(release.SHA256))
	environment := map[string]string{
		"PATH":                        releaseDirectory + "/bin:/usr/local/bin:/usr/bin:/bin",
		"HOME":                        workerDirectory,
		"TMPDIR":                      workerDirectory + "/scratch",
		"EXHIBIT_WORKER_TOKEN_FILE":   workerDirectory + "/endpoint-token",
		"EXHIBIT_WORKER_OWNER_ID":     identity.OwnerID,
		"EXHIBIT_WORKER_SESSION_ID":   identity.SessionID,
		"EXHIBIT_WORKER_EXECUTION_ID": identity.ExecutionID,
		"EXHIBIT_WORKER_VERSION":      release.Version,
		"EXHIBIT_WORKER_LISTEN":       fmt.Sprintf("127.0.0.1:%d", p.config.Port),
	}
	for key, value := range p.config.Environment {
		environment[key] = value
	}
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&script, " %s", shellQuote(key+"="+environment[key]))
	}
	fmt.Fprintf(&script, " %s", shellQuote(releaseDirectory+"/bin/exhibit-agent"))
	for _, argument := range []string{"--addr", fmt.Sprintf("127.0.0.1:%d", p.config.Port), "--work-root", workerDirectory + "/scratch", "--pi", releaseDirectory + "/bin/pi", "--execution-id", identity.ExecutionID, "--session-id", identity.SessionID, "--token-file", workerDirectory + "/endpoint-token"} {
		fmt.Fprintf(&script, " %s", shellQuote(argument))
	}
	for _, argument := range p.config.WorkerArgs {
		fmt.Fprintf(&script, " %s", shellQuote(argument))
	}
	script.WriteByte('\n')
	return []byte(script.String())
}

func expectedManifest(identity Identity, token string, script []byte) manifest {
	tokenHash := sha256.Sum256([]byte(token))
	scriptHash := sha256.Sum256(script)
	return manifest{Schema: 1, Identity: identity, BootstrapSHA256: hex.EncodeToString(scriptHash[:]), EndpointTokenSHA256: hex.EncodeToString(tokenHash[:])}
}

func (p *Provisioner) readManifest(ctx context.Context, name string) (manifest, error) {
	var result manifest
	query := url.Values{"path": {workerDirectory + "/manifest.json"}, "workingDir": {"/"}}
	err := p.jsonRequest(ctx, "GET", "/v1/sprites/"+name+"/fs/read?"+query.Encode(), nil, &result)
	return result, err
}

func serviceDefinition(port int) map[string]any {
	return map[string]any{"cmd": "/bin/sh", "args": []string{workerDirectory + "/bootstrap.sh"}, "needs": []string{}, "http_port": port}
}

func (p *Provisioner) install(ctx context.Context, request Request, name string, fresh bool) (bool, error) {
	script := p.bootstrapScript(request.Identity)
	expected := expectedManifest(request.Identity, request.EndpointToken, script)
	if !fresh {
		existing, err := p.readManifest(ctx, name)
		if err == nil {
			if existing != expected {
				return false, ErrConflict
			}
			// Existing complete assignment: don't rewrite its credential or restart a
			// live service on a repeated Provision call.
			var service struct {
				Cmd      string   `json:"cmd"`
				Args     []string `json:"args"`
				HTTPPort int      `json:"http_port"`
			}
			err = p.jsonRequest(ctx, "GET", "/v1/sprites/"+name+"/services/"+serviceName, nil, &service)
			if err == nil {
				if service.Cmd != "/bin/sh" || len(service.Args) != 1 || service.Args[0] != workerDirectory+"/bootstrap.sh" || service.HTTPPort != p.config.Port {
					return false, ErrConflict
				}
				return false, nil
			}
			if !errors.Is(err, ErrNotFound) {
				return false, err
			}
		} else if !errors.Is(err, ErrNotFound) {
			return false, err
		}
	}
	// The prefix and deterministic identity reserve this incomplete Sprite for
	// this assignment. Write the manifest last, before registering the service.
	for _, file := range []struct {
		path, mode string
		data       []byte
	}{
		{"bootstrap.sh", "0700", script},
		{"endpoint-token", "0600", []byte(request.EndpointToken)},
		{"manifest.json", "0600", mustJSON(expected)},
	} {
		if err := p.writeFile(ctx, name, workerDirectory+"/"+file.path, file.mode, file.data); err != nil {
			return true, err
		}
	}
	body := mustJSON(serviceDefinition(p.config.Port))
	err := p.retry(ctx, func() error {
		data, err := p.request(ctx, "PUT", "/v1/sprites/"+name+"/services/"+serviceName, "application/json", body)
		if err != nil {
			return err
		}
		return serviceStarted(data)
	})
	return true, err
}

func mustJSON(value any) []byte { data, _ := json.Marshal(value); return data }

// Current services return NDJSON; OpenAPI also describes a JSON service object.
// Discard stdout/stderr and upstream error data, even on startup failure.
func serviceStarted(data []byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), maxResponseBytes)
	succeeded := false
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var event struct {
			Type     string `json:"type"`
			ExitCode *int   `json:"exit_code"`
			Name     string `json:"name"`
			State    *struct {
				Status string `json:"status"`
			} `json:"state"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			// A formatted JSON service object is valid OpenAPI output too.
			if json.Unmarshal(data, &event) != nil || event.Type != "" {
				return errors.New("invalid Sprite service startup response")
			}
			if event.Name == serviceName && event.State != nil && (event.State.Status == "running" || event.State.Status == "starting") {
				return nil
			}
			return errors.New("Sprite service did not start")
		}
		if event.Type == "error" || event.Type == "exit" || (event.State != nil && event.State.Status == "failed") {
			return errors.New("Sprite worker service failed during startup; inspect protected service logs")
		}
		if event.Type == "complete" || event.Type == "started" || (event.Name == serviceName && event.State != nil && (event.State.Status == "running" || event.State.Status == "starting")) {
			succeeded = true
		}
	}
	if scanner.Err() != nil || !succeeded {
		return errors.New("incomplete Sprite service startup response")
	}
	return nil
}
