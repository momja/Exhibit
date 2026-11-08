package api

// Spaces describe provenance, independently of the current search or filters.
// Destinations stay available even when their current count is zero.
type gallerySpace struct {
	MobileOnly bool
	Path       string
	Label      string
	Icon       string
	Count      int
	ShowCount  bool
	Active     bool
}

func newGallerySpaces(activePath string, sharedWith, sharedBy int) []gallerySpace {
	spaces := []gallerySpace{
		{Path: "/", Label: "Gallery", Icon: "ph-books"},
		{Path: "/search", Label: "Search", Icon: "ph-magnifying-glass", MobileOnly: true},
		{Path: "/shared", Label: "Shared with me", Icon: "ph-tray-arrow-down", Count: sharedWith, ShowCount: true},
		{Path: "/shared-by-me", Label: "Shared by me", Icon: "ph-share-network", Count: sharedBy, ShowCount: true},
	}
	for i := range spaces {
		spaces[i].Active = spaces[i].Path == activePath
	}
	return spaces
}
