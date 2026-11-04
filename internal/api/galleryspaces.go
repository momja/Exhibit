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
		{Path: "/", Label: "Starred", Icon: "ph-star"},
		{Path: "/search", Label: "Search", Icon: "ph-magnifying-glass"},
		{Path: "/shared", Label: "Shared", Icon: "ph-share-network", Count: sharedWith + sharedBy, ShowCount: true},
		{Path: "/activity", Label: "Activity", Icon: "ph-clock-counter-clockwise"},
	}
	for i := range spaces {
		spaces[i].Active = spaces[i].Path == activePath
	}
	return spaces
}
