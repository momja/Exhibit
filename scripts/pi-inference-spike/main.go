// Run with go run ./scripts/pi-inference-spike -h. See docs/pi-inference-registry-spike.md.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func direct(ctx context.Context, client *http.Client, baseURL string, req request) (map[string]any, error) {
	path := "/api/v1/systemone"
	body := map[string]any{"model": req.Model}
	for k, v := range req.Context {
		body[k] = v
	}
	if req.Op == "classify" {
		questions := map[string]any{}
		for id, raw := range req.Context["questions"].(map[string]any) {
			q := map[string]any{}
			for k, v := range raw.(map[string]any) {
				q[k] = v
			}
			if q["type"] == "bool" {
				q["type"] = "noul"
			}
			questions[id] = q
		}
		body["questions"] = questions
	} else {
		path = "/api/v1/chat/completions"
		body["stream"] = true
		body["stream_options"] = map[string]bool{"include_usage": true}
		body["max_tokens"] = 8
		// Pi removes its context-only timestamp before sending.
		messages := []any{}
		for _, raw := range req.Context["messages"].([]any) {
			msg := raw.(map[string]any)
			messages = append(messages, map[string]any{"role": msg["role"], "content": msg["content"]})
		}
		body["messages"] = messages
	}
	b, _ := json.Marshal(body)
	r, err := http.NewRequestWithContext(ctx, "POST", baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+req.APIKey)
	r.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(r)
	if err != nil {
		return nil, errors.New("direct transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("provider HTTP %d", resp.StatusCode)
	}
	if req.Op == "classify" {
		var out map[string]any
		err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
		if out["answers"] == nil {
			return nil, errors.New("no classifier answers")
		}
		return out, err
	}
	out := map[string]any{"text": ""}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var chunk struct {
			Usage   any
			Choices []struct{ Delta struct{ Content string } }
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			return nil, err
		}
		if chunk.Usage != nil {
			out["usage"] = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			out["text"] = out["text"].(string) + choice.Delta.Content
		}
	}
	if out["text"] == "" || out["usage"] == nil {
		return nil, errors.New("incomplete direct chat response")
	}
	return out, scanner.Err()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	binary := flag.String("pi", "pi", "exact Pi Bolt 1.0.8 binary path")
	extension := flag.String("extension", "scripts/pi-inference-spike/extension.ts", "extension path")
	local := flag.Bool("local", false, "use loopback fixture; no provider requests")
	n := flag.Int("n", 5, "measured samples per kind/path, excludes one warmup each (1..20)")
	chat := flag.String("chat-model", "openai/gpt-4.1-nano", "catalog chat model")
	classifier := flag.String("classifier-model", "typesafe/jev-1.14", "catalog classifier model")
	flag.Parse()
	if *n < 1 || *n > 20 {
		return errors.New("n must be 1..20")
	}
	key := os.Getenv("OPENROUTER_API_KEY")
	baseURL, testURL := "https://openrouter.ai", ""
	if *local {
		f := newFixture()
		defer f.server.Close()
		baseURL = f.server.URL
		testURL = baseURL
		key = "test-key-A"
	} else if key == "" {
		return errors.New("OPENROUTER_API_KEY is required for live measurements")
	}
	bin, err := exec.LookPath(*binary)
	if err != nil {
		return err
	}
	bin, _ = filepath.Abs(bin)
	ext, _ := filepath.Abs(*extension)
	version, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return err
	}
	if !strings.Contains(string(version), "1.4.0 (Pi-Bolt 1.0.8,") {
		return errors.New("benchmark requires checksum-verified Pi Bolt 1.0.8 / Pi 1.4.0")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	started := time.Now()
	w, err := startWorker(ctx, bin, ext, testURL)
	startupMS := float64(time.Since(started).Microseconds()) / 1000
	cancel()
	if err != nil {
		return err
	}
	defer w.close()
	client := &http.Client{Timeout: 20 * time.Second}
	defer client.CloseIdleConnections()
	report := map[string]any{"at": time.Now().UTC().Format(time.RFC3339), "local_fixture": *local, "runtime": strings.TrimSpace(string(version)), "samples_per_path": *n, "startup_ms": startupMS, "worker_ready_rss_kib": rssKiB(w.cmd.Process.Pid), "go_ready_rss_kib": rssKiB(os.Getpid()), "chat_model": *chat, "classifier_model": *classifier}
	runs := []any{}
	for _, kind := range []string{"complete", "classify"} {
		model := *chat
		if kind == "classify" {
			model = *classifier
		}
		req := sampleRequest(kind, model, key, "")
		values := map[string][]float64{"worker": {}, "direct": {}}
		last := map[string]any{}
		// Alternate order to reduce (not eliminate) provider/time bias. i=0 warms both clients/adapters.
		for i := 0; i <= *n; i++ {
			paths := []string{"worker", "direct"}
			if i%2 == 1 {
				paths = []string{"direct", "worker"}
			}
			for _, path := range paths {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				started := time.Now()
				var result map[string]any
				var err error
				if path == "worker" {
					result, err = w.call(ctx, req)
				} else {
					result, err = direct(ctx, client, baseURL, req)
				}
				elapsed := float64(time.Since(started).Microseconds()) / 1000
				cancel()
				if err != nil {
					return fmt.Errorf("%s %s: %w", kind, path, err)
				}
				if path == "worker" && result["stopReason"] != "stop" {
					return fmt.Errorf("%s worker stopReason=%v", kind, result["stopReason"])
				}
				if i > 0 {
					values[path] = append(values[path], elapsed)
				}
				last[path] = result
			}
		}
		for _, path := range []string{"worker", "direct"} {
			ordered := append([]float64{}, values[path]...)
			sort.Float64s(ordered)
			runs = append(runs, map[string]any{"kind": kind, "path": path, "latency_ms": values[path], "median_ms": ordered[len(ordered)/2], "min_ms": ordered[0], "max_ms": ordered[len(ordered)-1], "last_result": last[path]})
		}
	}
	report["runs"] = runs
	report["worker_warm_rss_kib"] = rssKiB(w.cmd.Process.Pid)
	report["go_warm_rss_kib"] = rssKiB(os.Getpid())
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
