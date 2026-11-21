package main

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testWorker(t *testing.T, f *fixture) *worker {
	t.Helper()
	bin := os.Getenv("PI_SPIKE_BIN")
	if bin == "" {
		t.Skip("set PI_SPIKE_BIN to checksum-verified Pi Bolt 1.0.8 for real-runtime tests")
	}
	bin, _ = filepath.Abs(bin)
	version, err := exec.Command(bin, "--version").Output()
	if err != nil || !strings.Contains(string(version), "1.4.0 (Pi-Bolt 1.0.8,") {
		t.Fatalf("requires pinned Pi Bolt 1.0.8: %v %s", err, version)
	}
	ext, _ := filepath.Abs("extension.ts")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	w, err := startWorker(ctx, bin, ext, f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.close)
	return w
}

func callTest(w *worker, req request) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.call(ctx, req)
}

func successful(t *testing.T, result map[string]any, err error) {
	t.Helper()
	if err != nil || result["stopReason"] != "stop" {
		t.Fatalf("call failed: %v %+v", err, result)
	}
}

func TestWorkerConcurrencyCredentialsUsage(t *testing.T) {
	f := newFixture()
	defer f.server.Close()
	w := testWorker(t, f)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tenant := "tenant-A"
			key := "test-key-A"
			if i%2 == 1 {
				tenant = "tenant-B"
				key = "test-key-B"
			}
			kind, model := "complete", "openai/gpt-4.1-nano"
			if i%3 == 0 {
				kind = "classify"
				model = "typesafe/jev-1.14"
			}
			result, err := callTest(w, sampleRequest(kind, model, key, tenant+" mock-inference-slow"))
			successful(t, result, err)
			usage, ok := result["usage"].(map[string]any)
			if !ok {
				t.Error("missing usage")
				return
			}
			cost := usage["cost"].(map[string]any)
			if kind == "classify" {
				if math.Abs(cost["total"].(float64)-0.0000042) > 1e-15 || usage["input"] != float64(100) || cost["totalFromProvider"] != false {
					t.Errorf("wrong classifier catalog cost: %v", usage)
				}
				answers := result["answers"].(map[string]any)
				positive := answers["positive"].(map[string]any)
				sentiment := answers["sentiment"].(map[string]any)
				rating := answers["rating"].(map[string]any)
				probabilities := sentiment["probabilities"].(map[string]any)
				if positive["type"] != "bool" || positive["probability"] != 0.98 ||
					sentiment["choice"] != "positive" || sentiment["confidence"] != 0.96 ||
					probabilities["positive"] != 0.98 || probabilities["negative"] != 0.02 ||
					rating["score"] != 0.9 || rating["confidence"] != 0.95 {
					t.Errorf("wrong typed answers: %v", answers)
				}
			} else {
				if cost["totalFromProvider"] != true || cost["total"] != 0.0000104 {
					t.Errorf("wrong reported chat cost: %v", cost)
				}
				content := result["content"].([]any)
				if content[0].(map[string]any)["text"] != "OK "+tenant {
					t.Error("response correlation failed")
				}
			}
		}(i)
	}
	wg.Wait()
	state, err := callTest(w, request{Op: "ping"})
	if err != nil || state["messages"] != float64(0) || state["active"] != float64(0) {
		t.Fatalf("requests entered agent history or leaked jobs: %v %v", err, state)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.leaks != 0 || f.calls != 12 || f.peak < 2 {
		t.Fatalf("leaks=%d calls=%d peak=%d", f.leaks, f.calls, f.peak)
	}
	t.Logf("12 overlapping mixed calls; peak=%d; credential mismatches=%d", f.peak, f.leaks)
	// No transcript/auth/payload files anywhere in the worker's private HOME.
	_ = filepath.WalkDir(w.dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "test-key-") || strings.Contains(string(b), "tenant-") {
			t.Errorf("sensitive request persisted: %s", path)
		}
		return nil
	})
}

func TestWorkerCancelOnlyOneRequest(t *testing.T) {
	for _, kind := range []string{"complete", "classify"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture()
			defer f.server.Close()
			w := testWorker(t, f)
			model := "openai/gpt-4.1-nano"
			if kind == "classify" {
				model = "typesafe/jev-1.14"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			type outcome struct {
				result map[string]any
				err    error
			}
			aborted := make(chan outcome, 1)
			sibling := make(chan outcome, 1)
			go func() {
				r, e := w.call(ctx, sampleRequest(kind, model, "test-key-A", "tenant-A mock-inference-slow"))
				aborted <- outcome{r, e}
			}()
			go func() {
				r, e := callTest(w, sampleRequest(kind, model, "test-key-B", "tenant-B mock-inference-slow"))
				sibling <- outcome{r, e}
			}()
			deadline := time.Now().Add(time.Second)
			for {
				f.mu.Lock()
				active := f.active
				f.mu.Unlock()
				if active == 2 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("no overlap")
				}
				time.Sleep(time.Millisecond)
			}
			cancel()
			a := <-aborted
			if a.err != context.Canceled || a.result["stopReason"] != "aborted" {
				t.Fatalf("cancel not propagated: %v %v", a.err, a.result)
			}
			s := <-sibling
			successful(t, s.result, s.err)
			if _, err := callTest(w, request{Op: "ping"}); err != nil {
				t.Fatal("worker died on ordinary cancellation", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.canceled != 1 {
				t.Fatalf("transport cancellations=%d", f.canceled)
			}
		})
	}
}

func TestWorkerFailClosedRetainsUsage(t *testing.T) {
	f := newFixture()
	defer f.server.Close()
	w := testWorker(t, f)
	// Valid JSON scalars/null and malformed JSON must all be consumed by the hook,
	// not throw out of it and fall through to Pi's ordinary prompt path.
	for _, input := range []string{"null", "42", "[]", "bad-json"} {
		line, _ := json.Marshal(map[string]string{"type": "prompt", "id": "malformed-envelope", "message": input})
		w.queue <- append(line, '\n')
	}
	if _, err := callTest(w, request{Op: "ping"}); err != nil {
		t.Fatal("malformed input escaped the hook", err)
	}
	r, err := callTest(w, sampleRequest("classify", "typesafe/jev-1.14", "test-key-A", "mock-inference-malformed"))
	if err != nil || r["stopReason"] != "error" || r["usage"] == nil {
		t.Fatalf("lost usage on malformed billed result: %v %v", err, r)
	}
	usage := r["usage"].(map[string]any)
	cost := usage["cost"].(map[string]any)
	if usage["input"] != float64(100) || math.Abs(cost["total"].(float64)-0.0000042) > 1e-15 {
		t.Fatalf("wrong usage/cost on malformed billed result: %v", usage)
	}
	for _, kind := range []string{"complete", "classify"} {
		model := "openai/gpt-4.1-nano"
		if kind == "classify" {
			model = "typesafe/jev-1.14"
		}
		r, err = callTest(w, sampleRequest(kind, model, "test-key-A", "mock-inference-http-error"))
		if err != nil || r["stopReason"] != "error" {
			t.Fatalf("HTTP failure did not fail closed: %v %v", err, r)
		}
		b, _ := json.Marshal(r)
		if strings.Contains(string(b), "test-key-") {
			t.Fatal("provider error leaked credential")
		}
	}
	_, err = callTest(w, sampleRequest("complete", "openai/gpt-4.1-nano", "", ""))
	if err == nil {
		t.Fatal("accepted missing credentials")
	}
	_, err = callTest(w, sampleRequest("classify", "openai/gpt-4.1-nano", "test-key-A", ""))
	if err == nil {
		t.Fatal("chat model used as classifier")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != 3 {
		t.Fatalf("invalid requests reached provider or retries enabled: %d", f.calls)
	}
}

func TestWorkerCrashWedgeRestart(t *testing.T) {
	for _, fault := range []string{"crash", "wedge"} {
		t.Run(fault, func(t *testing.T) {
			f := newFixture()
			defer f.server.Close()
			w := testWorker(t, f)
			pending := make(chan error, 1)
			go func() {
				_, err := callTest(w, sampleRequest("classify", "typesafe/jev-1.14", "test-key-A", "mock-inference-slow"))
				pending <- err
			}()
			deadline := time.Now().Add(time.Second)
			for {
				f.mu.Lock()
				active := f.active
				f.mu.Unlock()
				if active > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("provider not started")
				}
				time.Sleep(time.Millisecond)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			started := time.Now()
			_, err := w.call(ctx, request{Op: fault})
			failureMS := float64(time.Since(started).Microseconds()) / 1000
			if err == nil {
				t.Fatal("fault succeeded")
			}
			if err := <-pending; err == nil {
				t.Fatal("in-flight call succeeded during fault")
			}
			if time.Since(started) > time.Second {
				t.Fatal("wedged worker did not fail promptly")
			}
			restartStarted := time.Now()
			restarted := testWorker(t, f)
			restartMS := float64(time.Since(restartStarted).Microseconds()) / 1000
			r, err := callTest(restarted, sampleRequest("classify", "typesafe/jev-1.14", "test-key-B", "tenant-B"))
			successful(t, r, err)
			t.Logf("%s: fail pending calls %.3f ms; fresh worker ready %.3f ms; recovery call succeeded", fault, failureMS, restartMS)
		})
	}
}

func TestWorkerBoundsAdmission(t *testing.T) {
	f := newFixture()
	defer f.server.Close()
	w := testWorker(t, f)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := callTest(w, sampleRequest("classify", "typesafe/jev-1.14", "test-key-A", "mock-inference-slow"))
			successful(t, r, e)
		}()
	}
	deadline := time.Now().Add(time.Second)
	for {
		f.mu.Lock()
		active := f.active
		f.mu.Unlock()
		if active == 16 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("admission test did not overlap")
		}
		time.Sleep(time.Millisecond)
	}
	_, err := callTest(w, sampleRequest("classify", "typesafe/jev-1.14", "test-key-A", ""))
	if err == nil || err.Error() != "busy" {
		t.Fatalf("overload did not fail closed: %v", err)
	}
	wg.Wait()
	oversize := sampleRequest("complete", "openai/gpt-4.1-nano", "test-key-A", strings.Repeat("x", 70_000))
	if _, err := callTest(w, oversize); err == nil {
		t.Fatal("accepted oversized request")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != 16 || f.peak != 16 {
		t.Fatalf("admission calls=%d peak=%d", f.calls, f.peak)
	}
}
