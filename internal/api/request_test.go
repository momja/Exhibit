package api

import (
	"net/http"
	"net/http/httptest"
)

// withOwnerResolved applies the real owner middleware to a test request.
func withOwnerResolved(ro *Router, req *http.Request) *http.Request {
	var resolved *http.Request
	ro.ownerMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		resolved = r
	})).ServeHTTP(httptest.NewRecorder(), req)
	return resolved
}
