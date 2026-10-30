package render

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/kit"
)

// An artifact that names a kit contract is served with a kit's stylesheet in
// place of the marker, and so is its widget, so a card's tile and the tool it
// opens share one look (Exh-61ix). Which kit is decided outside the body, on
// every request: the artifact's own choice, else its owner's library choice,
// else the default.
func TestServeInlinesTheKitChosenOutsideTheSource(t *testing.T) {
	marked := `<html><head><meta name="exhibit-kit" content="v1"></head><body>X</body></html>`
	rd := newWidgetRenderer(t, "abc", marked, marked)
	st := rd.cfg.Store
	ctx := context.Background()

	served := func() map[string]string {
		artifact := httptest.NewRecorder()
		rd.ServeArtifact(artifact, renderRequest("/a/abc", "abc", 1))
		widget := serveWidget(t, rd, "abc")
		out := map[string]string{}
		for name, w := range map[string]*httptest.ResponseRecorder{"artifact": artifact, "widget": widget} {
			if w.Code != 200 {
				t.Fatalf("%s: expected 200, got %d: %s", name, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), `<meta name="exhibit-kit"`) {
				t.Fatalf("%s: marker must be replaced", name)
			}
			out[name] = w.Body.String()
		}
		return out
	}
	expect := func(why, id string) {
		t.Helper()
		k, _ := kit.Get(id)
		for name, body := range served() {
			if !strings.Contains(body, `<style data-exhibit-kit="`+id+`">`+k.Stylesheet()+`</style>`) {
				t.Fatalf("%s: %s: expected %s inlined", why, name, id)
			}
		}
	}

	expect("no choice anywhere", "exhibit/v1")
	if err := st.SetLibraryKit(ctx, 1, "paper/v1"); err != nil {
		t.Fatal(err)
	}
	expect("the library's choice", "paper/v1")
	if err := st.UpdateArtifact(ctx, 1, "abc", map[string]any{"kit": "brutal/v1"}); err != nil {
		t.Fatal(err)
	}
	expect("the artifact's own choice beats the library's", "brutal/v1")

	// The kit's base gives body a background, but it sits in a cascade layer
	// and the widget's own base stylesheet does not, so a tile stays
	// transparent on its card whatever the kit says.
	if !strings.Contains(served()["widget"], widgetBaseCSS) {
		t.Fatal("widget must keep its unlayered base stylesheet alongside the kit")
	}
	if strings.Contains(widgetBaseCSS, "@layer") {
		t.Fatal("widget base stylesheet must stay unlayered to outrank the kit")
	}
}
