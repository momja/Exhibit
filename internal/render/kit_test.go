package render

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/kit"
)

// An artifact that pins the component kit is served with that exact kit
// version inlined, and so is its widget, so a card's tile and the tool it
// opens share one look (Exh-61ix). The stored body keeps only the marker.
func TestServeInlinesThePinnedKit(t *testing.T) {
	marked := `<html><head><meta name="exhibit-kit" content="exhibit/v1"></head><body>X</body></html>`
	rd := newWidgetRenderer(t, "abc", marked, marked)
	v1, _ := kit.Get("exhibit/v1")

	artifact := httptest.NewRecorder()
	rd.ServeArtifact(artifact, renderRequest("/a/abc", "abc", 1))
	widget := serveWidget(t, rd, "abc")

	for name, w := range map[string]*httptest.ResponseRecorder{"artifact": artifact, "widget": widget} {
		if w.Code != 200 {
			t.Fatalf("%s: expected 200, got %d: %s", name, w.Code, w.Body.String())
		}
		body := w.Body.String()
		if strings.Contains(body, `<meta name="exhibit-kit"`) {
			t.Fatalf("%s: marker must be replaced, got %s", name, body)
		}
		if !strings.Contains(body, `<style data-exhibit-kit="exhibit/v1">`+v1.Stylesheet()+`</style>`) {
			t.Fatalf("%s: kit exhibit/v1 stylesheet must be inlined", name)
		}
	}

	// The kit's base gives body a background, but it sits in a cascade layer
	// and the widget's own base stylesheet does not, so a tile stays
	// transparent on its card whatever the kit says.
	if !strings.Contains(widget.Body.String(), widgetBaseCSS) {
		t.Fatal("widget must keep its unlayered base stylesheet alongside the kit")
	}
	if strings.Contains(widgetBaseCSS, "@layer") {
		t.Fatal("widget base stylesheet must stay unlayered to outrank the kit")
	}
}
