// Package artifacticon builds the tab icon shared by artifact viewers and renders.
package artifacticon

import (
	"encoding/base64"
	"fmt"

	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/store"
)

// DataURI draws an Exhibit-style Mondrian grid using up to three tag colors
// in gallery order. The fixed panels have similar areas (49, 48, 42 units),
// with a smaller white panel and offset dividers retaining the logo's asymmetry.
// Untagged artifacts use the brand palette; unused tag panels stay white.
func DataURI(tags []*store.Tag) string {
	palette := [3]string{"#fff", "#fff", "#fff"}
	if len(tags) == 0 {
		palette = [3]string{color.BrandRed, color.BrandBlue, color.BrandYellow}
	} else {
		for i, tag := range tags[:min(len(tags), len(palette))] {
			palette[i] = color.Normalize(tag.Color)
		}
	}

	// Integer coordinates keep the one-pixel grid crisp at browser-tab size.
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="#111"/><rect x="1" y="1" width="7" height="7" fill="%s"/><rect x="9" y="7" width="6" height="8" fill="%s"/><rect x="1" y="9" width="7" height="6" fill="%s"/><rect x="9" y="1" width="6" height="5" fill="#fff"/></svg>`, palette[0], palette[1], palette[2])
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}
