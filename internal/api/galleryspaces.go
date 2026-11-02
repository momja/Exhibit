package api

// Spaces describe provenance, independently of the current search or filters.
// Destinations stay available even when their current count is zero.
type gallerySpace struct {
	Path      string
	Label     string
	Icon      string
	Count     int
	ShowCount bool
	Active    bool
}

func newGallerySpaces(activePath string, sharedWith, sharedBy int) []gallerySpace {
	spaces := []gallerySpace{
		{Path: "/", Label: "Library", Icon: "ph-books"},
		{Path: "/shared", Label: "Shared with me", Icon: "ph-tray-arrow-down", Count: sharedWith, ShowCount: true},
		{Path: "/shared-by-me", Label: "Shared by me", Icon: "ph-share-network", Count: sharedBy, ShowCount: true},
	}
	for i := range spaces {
		spaces[i].Active = spaces[i].Path == activePath
	}
	return spaces
}
