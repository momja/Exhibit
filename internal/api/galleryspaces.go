package api

// Spaces describe provenance, independently of the current search or filters.
// A nil slice leaves the original single-library page completely unchanged.
type gallerySpace struct {
	Path       string
	Label      string
	ShortLabel string
	Icon       string
	Count      int
	ShowCount  bool
	Active     bool
}

func newGallerySpaces(activePath string, sharedWith, sharedBy int) []gallerySpace {
	if sharedWith == 0 && sharedBy == 0 {
		return nil
	}
	spaces := []gallerySpace{
		{Path: "/", Label: "My library", ShortLabel: "My library", Icon: "ph-books"},
		{Path: "/shared", Label: "Shared with me", ShortLabel: "With me", Icon: "ph-tray-arrow-down", Count: sharedWith, ShowCount: true},
		{Path: "/shared-by-me", Label: "Shared by me", ShortLabel: "By me", Icon: "ph-share-network", Count: sharedBy, ShowCount: true},
	}
	for i := range spaces {
		spaces[i].Active = spaces[i].Path == activePath
	}
	return spaces
}
