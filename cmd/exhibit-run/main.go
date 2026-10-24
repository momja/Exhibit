// exhibit-run serves an exported browser app with a local, credential-aware
// relay. It executes no artifact code and needs no JavaScript runtime.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/momja/Exhibit/internal/bundle"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address (loopback by default)")
	publicHost := flag.String("public-host", "", "additional exact host:port accepted by the local runner")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: exhibit-run [-addr host:port] [-public-host name:port] <app.zip | directory>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	filename := flag.Arg(0)
	app, err := bundle.Read(filename)
	if err != nil {
		log.Fatal(err)
	}
	statePath := filepath.Join(filename, "state.json")
	if info, err := os.Stat(filename); err == nil && !info.IsDir() {
		statePath = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".state.json"
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		log.Fatal(err)
	}
	hosts := []string{net.JoinHostPort(host, port)}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		for _, alias := range []string{"localhost", "127.0.0.1", "::1"} {
			hosts = append(hosts, net.JoinHostPort(alias, port))
		}
	}
	if *publicHost != "" {
		hosts = append(hosts, *publicHost)
	}
	handler, err := bundle.NewServer(app, bundle.ServerOptions{AllowedHosts: hosts, StatePath: statePath, ConnectionsJSON: os.Getenv("EXHIBIT_CONNECTIONS")})
	if err != nil {
		log.Fatal(err)
	}
	defer handler.Close()
	display := listener.Addr().String()
	if *publicHost != "" {
		display = *publicHost
	}
	log.Printf("%q is running at http://%s/", app.Manifest.Title, display)
	log.Printf("Open Connections in the page to enter local credentials; saved data: %s", statePath)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 25 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 64 << 10}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
