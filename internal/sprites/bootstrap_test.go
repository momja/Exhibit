package sprites

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureRuntime(t *testing.T, root, nodeVersion string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"bin/exhibit-agent": "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_ARGV_FILE\"\nenv > \"$TEST_ENV_FILE\"\n",
		"bin/node":          "#!/bin/sh\nprintf '%s\\n' '" + nodeVersion + "'\n",
		"bin/pi":            "#!/bin/sh\nprintf '%s\\n' '1.3.0'\n",
		"EXHIBIT_RELEASE":   "revision-123\n",
	}
	for path, data := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
}
func packFixture(t *testing.T, root, path string) string {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	archive := tar.NewWriter(gz)
	for _, path := range []string{"bin/exhibit-agent", "bin/node", "bin/pi", "EXHIBIT_RELEASE"} {
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if err := archive.WriteHeader(&tar.Header{Name: path, Mode: 0700, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data.Bytes())
	return hex.EncodeToString(sum[:])
}

func TestBootstrapVerifiesReleasePinsAndDaemonArguments(t *testing.T) {
	for _, scenario := range []string{"valid", "checksum", "node-version", "release-version"} {
		t.Run(scenario, func(t *testing.T) {
			temp := t.TempDir()
			runtime := filepath.Join(temp, "prepared")
			nodeVersion := "v22.22.0"
			if scenario == "node-version" {
				nodeVersion = "v99.0.0"
			}
			fixtureRuntime(t, runtime, nodeVersion)
			if scenario == "release-version" {
				if err := os.WriteFile(filepath.Join(runtime, "EXHIBIT_RELEASE"), []byte("wrong-version"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			archive := filepath.Join(temp, "release.tar.gz")
			checksum := packFixture(t, runtime, archive)
			root := filepath.Join(temp, "worker")
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "endpoint-token"), []byte(endpointToken), 0600); err != nil {
				t.Fatal(err)
			}
			curlDirectory := filepath.Join(temp, "fake-tools")
			if err := os.Mkdir(curlDirectory, 0700); err != nil {
				t.Fatal(err)
			}
			curl := `#!/bin/sh
set -eu
output=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '-o' ]; then shift; output=$1; fi
  shift
done
cp "$FAKE_ARCHIVE" "$output"
printf 'download\n' >> "$FAKE_DOWNLOAD_LOG"
`
			if err := os.WriteFile(filepath.Join(curlDirectory, "curl"), []byte(curl), 0700); err != nil {
				t.Fatal(err)
			}
			config := Config{Token: managementToken, Prefix: "bootstrap-test", Release: Release{Version: "revision-123", URL: "https://releases.example/pinned.tar.gz", SHA256: checksum}, Environment: map[string]string{"TEST_ARGV_FILE": filepath.Join(temp, "argv"), "TEST_ENV_FILE": filepath.Join(temp, "env")}}
			if scenario == "checksum" {
				config.Release.SHA256 = strings.Repeat("0", 64)
			}
			p, err := New(config)
			if err != nil {
				t.Fatal(err)
			}
			identity := testRequest().Identity
			identity.SessionID = "conversation'; printf 'unsafe' > /should-never-exist; '"
			script := strings.ReplaceAll(string(p.bootstrapScript(identity)), workerDirectory, root)
			scriptPath := filepath.Join(temp, "bootstrap.sh")
			if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			run := func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, "/bin/sh", scriptPath)
				command.Env = append(os.Environ(), "PATH="+curlDirectory+":"+os.Getenv("PATH"), "FAKE_ARCHIVE="+archive, "FAKE_DOWNLOAD_LOG="+filepath.Join(temp, "downloads"), "SPRITE_TOKEN="+managementToken, "PROVIDER_API_KEY=unassigned-key")
				return command.Run()
			}
			err = run()
			if scenario != "valid" {
				if err == nil {
					t.Fatal("unverified or mismatched runtime was executed")
				}
				if _, err := os.Stat(filepath.Join(temp, "argv")); !os.IsNotExist(err) {
					t.Error("failed verification executed worker")
				}
				installs, _ := filepath.Glob(filepath.Join(root, "releases", ".install-*"))
				if len(installs) != 0 {
					t.Error("failed install left scratch download")
				}
				return
			}
			if err != nil {
				t.Fatalf("verified bootstrap failed: %v", err)
			}
			if err := run(); err != nil {
				t.Fatalf("cold bootstrap failed: %v", err)
			}
			downloads, err := os.ReadFile(filepath.Join(temp, "downloads"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(downloads), "download") != 1 {
				t.Error("cold restart downloaded already pinned runtime again")
			}
			arguments, err := os.ReadFile(filepath.Join(temp, "argv"))
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range []string{"--addr\n127.0.0.1:8080", "--work-root\n" + root + "/scratch", "--pi\n" + root + "/releases/revision-123-" + checksum[:16] + "/bin/pi", "--execution-id\n" + identity.ExecutionID, "--session-id\n" + identity.SessionID, "--token-file\n" + root + "/endpoint-token"} {
				if !strings.Contains(string(arguments), expected) {
					t.Error("bootstrap does not match standalone daemon CLI")
				}
			}
			environment, err := os.ReadFile(filepath.Join(temp, "env"))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(environment, []byte(managementToken)) || bytes.Contains(environment, []byte("unassigned-key")) || bytes.Contains(environment, []byte(endpointToken)) {
				t.Error("bootstrap inherited infrastructure/provider credentials into supervisor environment")
			}
			info, err := os.Stat(root)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Error("worker directory must be private")
			}
		})
	}
}

func TestPackageReleaseReproducibleAndRefusesOverwrite(t *testing.T) {
	temp := t.TempDir()
	runtime := filepath.Join(temp, "runtime")
	fixtureRuntime(t, runtime, "v22.22.0")
	script, err := filepath.Abs("../../scripts/agent-workers-sprites/package-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	var sums [2][32]byte
	for i, name := range []string{"one.tar.gz", "two.tar.gz"} {
		output := filepath.Join(temp, name)
		cmd := exec.Command("bash", script, "revision-123", runtime, output)
		if err := cmd.Run(); err != nil {
			t.Fatalf("package runtime: %v", err)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		sums[i] = sha256.Sum256(data)
		checksum, err := os.ReadFile(output + ".sha256")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(checksum), hex.EncodeToString(sums[i][:])) {
			t.Error("release checksum differs from archived runtime")
		}
	}
	if sums[0] != sums[1] {
		t.Error("same runtime produced different archive digests")
	}
	cmd := exec.Command("bash", script, "revision-123", runtime, filepath.Join(temp, "one.tar.gz"))
	if err := cmd.Run(); err == nil {
		t.Error("packaging overwrote immutable release")
	}
	if err := os.Remove(filepath.Join(runtime, "bin", "node")); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("bash", script, "revision-123", runtime, filepath.Join(temp, "missing.tar.gz"))
	if err := cmd.Run(); err == nil {
		t.Error("packaging accepted incomplete runtime")
	}
}
