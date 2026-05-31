// Command watcher watches a directory for new artifact files and ingests them
// into the artifact viewer service via the API.
//
// Files matching *.artifact.html or residing in an /artifacts/ directory
// are auto-ingested when detected.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	watchDir := getenv("WATCH_DIR", "./artifacts")
	apiBase := getenv("API_BASE", "http://localhost:8080")
	authToken := getenv("AUTH_TOKEN", "dev-token")
	interval := 5 * time.Second

	log.Printf("Watching %s, posting to %s", watchDir, apiBase)

	seen := make(map[string]time.Time)

	for {
		entries, err := os.ReadDir(watchDir)
		if err != nil {
			log.Printf("read dir %s: %v", watchDir, err)
			time.Sleep(interval)
			continue
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !isArtifactFile(name) {
				continue
			}

			info, err := e.Info()
			if err != nil {
				continue
			}
			path := filepath.Join(watchDir, name)

			if prev, ok := seen[path]; ok && !info.ModTime().After(prev) {
				continue
			}
			seen[path] = info.ModTime()

			if err := ingest(path, apiBase, authToken); err != nil {
				log.Printf("ingest %s: %v", path, err)
			} else {
				log.Printf("ingested %s", path)
			}
		}

		time.Sleep(interval)
	}
}

func isArtifactFile(name string) bool {
	if strings.HasSuffix(name, ".artifact.html") {
		return true
	}
	if strings.HasSuffix(name, ".html") {
		return true
	}
	return false
}

type ingestRequest struct {
	Title            string   `json:"title"`
	Body             string   `json:"body"`
	NetworkAllowlist []string `json:"network_allowlist"`
}

func ingest(path, apiBase, authToken string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	title = strings.TrimSuffix(title, ".artifact")

	req := ingestRequest{
		Title:            title,
		Body:             string(data),
		NetworkAllowlist: []string{},
	}
	body, _ := json.Marshal(req)

	r, err := http.NewRequest(http.MethodPost, apiBase+"/api/artifacts", bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+authToken)

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
