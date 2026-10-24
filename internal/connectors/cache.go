package connectors

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/momja/Exhibit/internal/egress"
)

// Read cached body data as encoded text first, so an old oversized cache entry
// cannot allocate an oversized decoded body before the response limit runs.
func decodeCachedResponse(plain string) (*egress.Response, error) {
	if len(plain) > maxConnectionCacheJSONBytes {
		return nil, ErrResponseSize
	}
	var envelope struct {
		Status     int
		StatusText string
		Header     http.Header
		Body       string
		URL        string
	}
	if json.Unmarshal([]byte(plain), &envelope) != nil {
		return nil, errors.New("connection cache contains an invalid response")
	}
	response := &egress.Response{Status: envelope.Status, StatusText: envelope.StatusText, Header: envelope.Header, URL: envelope.URL}
	if err := responseSizeAllowed(response); err != nil {
		return nil, err
	}
	encoded := envelope.Body
	if len(encoded)%4 != 0 || strings.ContainsAny(encoded, "\r\n") {
		return nil, errors.New("connection cache contains an invalid response")
	}
	padding := 0
	for i := len(encoded) - 1; i >= 0 && encoded[i] == '='; i-- {
		padding++
	}
	if padding > 2 || strings.Contains(encoded[:len(encoded)-padding], "=") {
		return nil, errors.New("connection cache contains an invalid response")
	}
	size := base64.StdEncoding.DecodedLen(len(encoded)) - padding
	if size > maxConnectionResponseBytes {
		return nil, ErrResponseSize
	}
	// With padding restricted to the last two positions, the exact decoded
	// length is known before allocation, including the 4 MiB boundary case.
	body := make([]byte, size)
	n, err := base64.StdEncoding.Decode(body, []byte(encoded))
	if err != nil {
		return nil, errors.New("connection cache contains an invalid response")
	}
	response.Body = body[:n]
	return response, nil
}
