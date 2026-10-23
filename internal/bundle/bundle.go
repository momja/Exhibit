// Package bundle exports browser-only apps with a local relay. Bundles contain
// connection requirements, never Exhibit connection IDs, credentials or server
// scripts. Both runners enforce the same origin and public-address boundary.
package bundle

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/origin"
)

//go:embed files/serve.mjs
var serveMJS []byte

//go:embed files/shim.js
var shimJS string

//go:embed files/host.js
var hostJS string

//go:embed files/connect.js
var connectJS string

const FormatVersion = 1
const maxBundleFile = 64 << 20

// ConnectionRequirement is intentionally incapable of representing a secret or
// a reference to an Exhibit's user's connection. Local credentials are acquired
// again by the runner, outside the artifact frame.
type ConnectionRequirement struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Origin       string   `json:"origin"`
	Header       string   `json:"header,omitempty"`
	DailyLimit   int      `json:"daily_limit,omitempty"`
	CacheMinutes int      `json:"cache_minutes,omitempty"`
	DataOrigins  []string `json:"data_origins,omitempty"`
}
type Files struct {
	Page  string `json:"page"`
	State string `json:"state"`
}
type Manifest struct {
	Format           string                  `json:"format"`
	FormatVersion    int                     `json:"format_version"`
	Title            string                  `json:"title"`
	ExportedAt       time.Time               `json:"exported_at"`
	Files            Files                   `json:"files"`
	NetworkAllowlist []string                `json:"network_allowlist"`
	Connections      []ConnectionRequirement `json:"connections"`
}
type Bundle struct {
	Manifest Manifest
	Page     string
	State    map[string]string
}

func New(title, page string, state map[string]string, origins []string, requirements []ConnectionRequirement) *Bundle {
	if state == nil {
		state = map[string]string{}
	}
	if origins == nil {
		origins = []string{}
	}
	if requirements == nil {
		requirements = []ConnectionRequirement{}
	}
	return &Bundle{Manifest: Manifest{Format: "exhibit-relay", FormatVersion: FormatVersion, Title: title, ExportedAt: time.Now().UTC().Truncate(time.Second), Files: Files{Page: "index.html", State: "state.json"}, NetworkAllowlist: origins, Connections: requirements}, Page: page, State: state}
}

func (b *Bundle) validate() error {
	if b.Manifest.Format != "exhibit-relay" || b.Manifest.FormatVersion != FormatVersion {
		return errors.New("unsupported Exhibit relay bundle format")
	}
	if b.Manifest.Files != (Files{Page: "index.html", State: "state.json"}) {
		return errors.New("bundle must use the standard page and state files")
	}
	origins, err := origin.NormalizeOrigins(b.Manifest.NetworkAllowlist)
	if err != nil {
		return err
	}
	b.Manifest.NetworkAllowlist = origins
	allowed := make(map[string]bool)
	for _, o := range origins {
		allowed[o] = true
	}
	for i := range b.Manifest.Connections {
		requirement := &b.Manifest.Connections[i]
		o, err := origin.NormalizeOrigin(requirement.Origin)
		if err != nil || !strings.HasPrefix(o, "https://") || !allowed[o] {
			return errors.New("local connection origin must be an allowed https origin")
		}
		requirement.Origin = o
		switch requirement.Kind {
		case "bearer", "basic", "api-key":
		default:
			return errors.New("unsupported local connection kind; export again and reconnect using a generic connection")
		}
		if requirement.Kind == "api-key" && !credentialHeader(requirement.Header) {
			return errors.New("invalid API key header")
		}
		if requirement.DailyLimit < 0 || requirement.CacheMinutes < 0 || requirement.CacheMinutes > 525600 {
			return errors.New("invalid connection request or response reuse limit")
		}
		if len(requirement.DataOrigins) == 0 {
			requirement.DataOrigins = []string{o}
		}
		requirement.DataOrigins, err = origin.NormalizeOrigins(append(requirement.DataOrigins, o))
		if err != nil {
			return errors.New("invalid connection data destinations")
		}
		for _, destination := range requirement.DataOrigins {
			if !allowed[destination] {
				return errors.New("connection data destination must be in the bundle allowlist")
			}
		}
	}
	if b.State == nil {
		b.State = map[string]string{}
	}
	return nil
}

func (b *Bundle) Write(w io.Writer, dirName string) error {
	if err := b.validate(); err != nil {
		return err
	}
	if dirName == "" || path.Base(dirName) != dirName || strings.ContainsAny(dirName, "\\\x00") || dirName == "." || dirName == ".." {
		return errors.New("invalid bundle directory name")
	}
	manifest, err := json.MarshalIndent(b.Manifest, "", "  ")
	if err != nil {
		return err
	}
	state, err := json.MarshalIndent(b.State, "", "  ")
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	for _, entry := range []struct {
		name string
		data []byte
		mode os.FileMode
	}{
		{"exhibit.json", manifest, 0644}, {"index.html", []byte(b.Page), 0644}, {"state.json", state, 0600},
		{"serve.mjs", serveMJS, 0755}, {"shim.js", []byte(shimJS), 0644}, {"host.js", []byte(hostJS), 0644}, {"connect.js", []byte(connectJS), 0644}, {"README.md", []byte(b.readme()), 0644},
	} {
		header := &zip.FileHeader{Name: dirName + "/" + entry.name, Method: zip.Deflate, Modified: b.Manifest.ExportedAt}
		header.SetMode(entry.mode)
		file, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := file.Write(entry.data); err != nil {
			return err
		}
	}
	return zw.Close()
}
func (b *Bundle) Bytes(dirName string) ([]byte, error) {
	var buf bytes.Buffer
	err := b.Write(&buf, dirName)
	return buf.Bytes(), err
}

func (b *Bundle) readme() string {
	origins := strings.Join(b.Manifest.NetworkAllowlist, ", ")
	if origins == "" {
		origins = "none"
	}
	return fmt.Sprintf(`# %s

Run this browser app with Node 18 or newer, Bun, or Deno (with network and file
permissions):

    node serve.mjs

Open the address printed by the runner. PORT changes the default port, 8787.
Or run the same folder or ZIP with the Go runner:

    exhibit-run .

Use Connections in the local page to enter credentials again. They stay in the
runner's memory until it stops; they never reach the app frame or state.json.
EXHIBIT_CONNECTIONS can optionally hold JSON entries with origin, kind, header,
secret, username, password, access_url, daily_limit, cache_minutes and data_origins
fields instead of using the form. A Basic connection can accept an HTTPS URL
with embedded credentials; its private base path stays in the runner. Do not put that
environment variable or a secret file in a shared bundle.

Every connection defaults to keeping data at its source origin. Use the trusted
Connections form to explicitly approve any other allowed destinations. These
data policies survive disconnects and restarts beside state.json. Request limits
and response reuse apply while this runner is running; reconnect after restarting.
Older unsupported connector requirements must be exported again and reconnected
using a generic connection. No setup token is redeemed by the runner.

Allowed relay origins: %s. Both runners require HTTPS, check each redirect and
block private, loopback, metadata and other reserved addresses. Existing browser
assets may still load from these origins. The page runs in an opaque sandbox;
connections and saved data are handled by the local runner. No artifact code
runs on the server. Saved data persists in state.json; the ZIP runner writes a
separate .state.json file beside its ZIP. Stop the runner to stop local access.
`, b.Manifest.Title, origins)
}

// Read accepts a standard ZIP or directory. It never extracts files or follows
// archive paths supplied by the bundle's manifest.
func Read(filename string) (*Bundle, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	var read func(string) ([]byte, error)
	if info.IsDir() {
		read = func(name string) ([]byte, error) {
			f, err := os.Open(filepath.Join(filename, name))
			if err != nil {
				return nil, err
			}
			defer f.Close()
			return readBounded(f, maxBundleFile)
		}
	} else {
		zr, err := zip.OpenReader(filename)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		entries := map[string]*zip.File{}
		root := ""
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			if f.Mode()&os.ModeSymlink != 0 || strings.ContainsAny(f.Name, "\\\x00") || path.Clean(f.Name) != f.Name || strings.HasPrefix(f.Name, "/") {
				return nil, errors.New("unsafe bundle archive path")
			}
			parts := strings.Split(f.Name, "/")
			if len(parts) != 2 || parts[0] == ".." || parts[0] == "." {
				return nil, errors.New("bundle entries require one common directory")
			}
			if root == "" {
				root = parts[0]
			}
			if parts[0] != root {
				return nil, errors.New("bundle entries have different roots")
			}
			if _, exists := entries[parts[1]]; exists {
				return nil, errors.New("duplicate bundle archive entry")
			}
			if f.UncompressedSize64 > maxBundleFile {
				return nil, errors.New("bundle entry exceeds size limit")
			}
			entries[parts[1]] = f
		}
		read = func(name string) ([]byte, error) {
			f := entries[name]
			if f == nil {
				return nil, fmt.Errorf("bundle missing %s", name)
			}
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer r.Close()
			return readBounded(r, maxBundleFile)
		}
	}
	raw, err := read("exhibit.json")
	if err != nil {
		return nil, err
	}
	b := &Bundle{}
	if err = json.Unmarshal(raw, &b.Manifest); err != nil {
		return nil, fmt.Errorf("exhibit.json: %w", err)
	}
	if err = b.validate(); err != nil {
		return nil, err
	}
	page, err := read("index.html")
	if err != nil {
		return nil, err
	}
	b.Page = string(page)
	state, err := read("state.json")
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(state, &b.State); err != nil {
		return nil, fmt.Errorf("state.json: %w", err)
	}
	return b, nil
}
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("bundle file exceeds size limit")
	}
	return b, nil
}
