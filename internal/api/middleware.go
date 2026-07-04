package api

import (
	"context"
	"net/http"
)

type contextKey string

const ownerIDKey contextKey = "ownerID"

const defaultOwnerID int64 = 1

func authMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware allows cross-origin state calls from the render origin (where
// the storage shim runs) to the app origin. It answers preflight OPTIONS
// requests directly. If renderOrigin is empty, no CORS headers are emitted.
func corsMiddleware(renderOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if renderOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", renderOrigin)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ownerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), ownerIDKey, defaultOwnerID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ownerIDFromCtx(ctx context.Context) int64 {
	if v, ok := ctx.Value(ownerIDKey).(int64); ok {
		return v
	}
	return defaultOwnerID
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
