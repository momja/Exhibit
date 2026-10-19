package artifacticon

import (
	"encoding/base64"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

type iconRectangle struct {
	Fill   string  `xml:"fill,attr"`
	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
}

func iconRectangles(t *testing.T, tags []*store.Tag) []iconRectangle {
	t.Helper()
	svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(DataURI(tags), "data:image/svg+xml;base64,"))
	require.NoError(t, err)
	var image struct {
		Rectangles []iconRectangle `xml:"rect"`
	}
	require.NoError(t, xml.Unmarshal(svg, &image))
	require.Len(t, image.Rectangles, 5, "background, three color panels, and a white panel")
	return image.Rectangles
}

func TestTagMosaic(t *testing.T) {
	tags := []*store.Tag{{Color: "#f00"}, {Color: "#00ff00"}, {Color: `"/><script/>`}, {Color: "#123456"}}
	for count := 1; count <= len(tags); count++ {
		rects := iconRectangles(t, tags[:count])
		for i, panel := range rects[1:4] {
			want := "#fff"
			if i < count {
				want = color.Normalize(tags[i].Color)
			}
			require.Equal(t, want, panel.Fill)
		}
		require.Equal(t, "#fff", rects[4].Fill, "retain the white brand panel")
	}
}

func TestUntaggedMosaicUsesBrandPalette(t *testing.T) {
	rects := iconRectangles(t, nil)
	require.Equal(t, color.BrandRed, rects[1].Fill)
	require.Equal(t, color.BrandBlue, rects[2].Fill)
	require.Equal(t, color.BrandYellow, rects[3].Fill)
}

func TestTagPanelsHaveBalancedAreas(t *testing.T) {
	rects := iconRectangles(t, nil)
	var smallest, largest float64
	for i, panel := range rects[1:4] {
		area := panel.Width * panel.Height
		if i == 0 || area < smallest {
			smallest = area
		}
		largest = max(largest, area)
	}
	require.LessOrEqual(t, largest/smallest, 1.2, "each tag should have similar visual weight")
}
