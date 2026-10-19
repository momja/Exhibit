// Package artifacticon builds the tab icon shared by artifact viewers and renders.
package artifacticon

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/store"
)

// DataURI draws the logo's E with up to three tag colors, in gallery order.
// The path is extracted from internal/api/logo.svg, without its frame or wire.
func DataURI(tags []*store.Tag) string {
	var svg strings.Builder
	svg.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">`)
	x := 1.0
	if len(tags) == 0 {
		x = 4
	}
	fmt.Fprintf(&svg, `<path transform="translate(%g 3) translate(-36.298 -47.044)" fill="%s" stroke="#000" stroke-width="1.98523" stroke-linecap="round" paint-order="stroke fill" d="M 60.383501,72.781407 H 37.290736 V 48.036637 H 60.383501 V 52.82273 H 45.481485 v 4.270925 h 13.829928 v 4.786094 H 45.481485 v 6.115565 h 14.902016 z"/>`, x, color.BrandRed)
	dotCount := min(len(tags), 3)
	for i, tag := range tags {
		if i == 3 {
			break
		}
		// Five-unit spacing overlaps the seven-unit dots into a centered stack.
		y := 16 - float64(dotCount-1)*2.5 + float64(i)*5
		fmt.Fprintf(&svg, `<circle cx="28" cy="%g" r="3.5" fill="%s" stroke="#fff" stroke-width=".5"/>`, y, color.Normalize(tag.Color))
	}
	svg.WriteString(`</svg>`)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg.String()))
}
