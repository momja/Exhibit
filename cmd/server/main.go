package main

import (
	"log"
	"net/http"
	"os"

	"github.com/artifact-viewer/artifact-viewer/internal/api"
	"github.com/artifact-viewer/artifact-viewer/internal/blob"
	"github.com/artifact-viewer/artifact-viewer/internal/store"
)

func main() {
	dataDir := getenv("DATA_DIR", "./data")
	dbPath := dataDir + "/app.db"
	blobDir := dataDir + "/blobs"
	appOrigin := getenv("APP_ORIGIN", "http://localhost:8080")
	renderOrigin := getenv("RENDER_ORIGIN", "http://localhost:8081")
	authToken := getenv("AUTH_TOKEN", "dev-token")
	addr := getenv("ADDR", ":8080")
	renderAddr := getenv("RENDER_ADDR", ":8081")

	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	bl, err := blob.NewFSStore(blobDir)
	if err != nil {
		log.Fatalf("open blob store: %v", err)
	}

	router := api.NewRouter(api.Config{
		Store:        st,
		Blob:         bl,
		AppOrigin:    appOrigin,
		RenderOrigin: renderOrigin,
		AuthToken:    authToken,
	})

	log.Printf("App server on %s (app origin: %s, render origin: %s)", addr, appOrigin, renderOrigin)
	go func() {
		log.Fatal(http.ListenAndServe(renderAddr, router.RenderHandler()))
	}()
	log.Fatal(http.ListenAndServe(addr, router))
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
