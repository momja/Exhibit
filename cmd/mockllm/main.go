// Command mockllm runs the deterministic stand-in LLM (internal/mockllm) as a
// standalone server, for driving the agent surface by hand or from a
// non-Go test harness. Go tests mount internal/mockllm.Handler() directly.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/momja/Exhibit/internal/mockllm"
)

func main() {
	addr := ":9095"
	if v := os.Getenv("ADDR"); v != "" {
		addr = v
	}
	log.Printf("mock LLM listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mockllm.Handler())) //nolint:gosec // dev/test server
}
