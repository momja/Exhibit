package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWarningsFlagProxyAndNoCORS(t *testing.T) {
	body := `<script>fetch('https://corsproxy.io/?url=https://api.example.test', { mode: 'no-cors' }); fetch('https://corsproxy.io/again')</script>`
	warnings := Warnings(body, "")
	assert.Len(t, warnings, 2)
	assert.Equal(t, "public_cors_proxy", warnings[0].Code)
	assert.Equal(t, "https://corsproxy.io", warnings[0].Origin)
	assert.Contains(t, warnings[0].Message, "third party")
	assert.Equal(t, "no_cors", warnings[1].Code)
	assert.Contains(t, warnings[1].Message, "opaque, unreadable")
	assert.Contains(t, warnings[1].Message, "not routed")
}

func TestWarningsMatchProxyHostnamesAndModeOptions(t *testing.T) {
	for _, body := range []string{
		`const proxy = 'corsproxy.io';`,
		`fetch('https://api.allorigins.win/raw?url=x')`,
		`fetch('https://CORS-ANYWHERE.HEROKUAPP.COM/api')`,
	} {
		assert.Equal(t, "public_cors_proxy", Warnings(body, "")[0].Code, body)
	}
	for _, body := range []string{`{"mode":"no-cors"}`, "{mode: `no-cors`}", "{ 'mode' : 'no-cors' }"} {
		assert.Equal(t, "no_cors", Warnings(body, "")[0].Code, body)
	}
	assert.Empty(t, Warnings(`fetch('https://corsproxy.io.example/api', {mode: 'cors'})`, ""))
	assert.Empty(t, Warnings(`const text = 'no-cors';`, ""))
}

func TestWarningsReportOriginalSiteAPICalls(t *testing.T) {
	for _, body := range []string{
		`<script>fetch('/api/me')</script>`,
		`<base href="https://source.example/tool/"><script>fetch('../api/me')</script>`,
		`<script>fetch('https://source.example/api/me')</script>`,
	} {
		warnings := Warnings(body, "https://source.example/tools/app")
		assert.Len(t, warnings, 1, body)
		assert.Equal(t, "source_site_api", warnings[0].Code)
		assert.Equal(t, "https://source.example", warnings[0].Origin)
	}
	assert.Empty(t, Warnings(`<img src="https://source.example/logo.png"><script>fetch('https://public-api.example/data')</script>`, "https://source.example/tools/app"))
	assert.Empty(t, Warnings(`<script>fetch('/api/me')</script>`, ""))
}

func TestFetchOriginsOnlyReportsAPICalls(t *testing.T) {
	body := `<base href="https://source.example/app/"><img src="https://images.example/a"><script>fetch('/api'); fetch('https://api.example/data'); fetch('https://api.example/other')</script>`
	assert.Equal(t, []string{"https://api.example", "https://source.example"}, FetchOrigins(body))
}
