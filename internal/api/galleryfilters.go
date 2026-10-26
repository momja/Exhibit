package api

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/momja/Exhibit/internal/color"
	"github.com/momja/Exhibit/internal/store"
)

var errGalleryFilter = errors.New("invalid gallery filter")

type galleryFilterOption struct {
	Name, Value, Label, Color string
	Active                    bool
	PageURL, FragmentURL      string
}

type galleryFilterSection struct {
	Label   string
	Options []galleryFilterOption
}

type galleryFilters struct {
	Sections                       []galleryFilterSection
	Chips                          []galleryFilterOption
	ClearPageURL, ClearFragmentURL string
}

// Filter choices always come from the principal's complete library, independent
// of search and the selected space. Incoming shares expose their owners instead.
func (ro *Router) loadGalleryFilters(r *http.Request, gallery galleryDefinition, grid *galleryGridData) error {
	owner := ownerIDFromCtx(r.Context())
	var sections []galleryFilterSection
	if gallery.ReadOnly {
		owners, err := ro.cfg.Store.SharedWithOwners(r.Context(), store.ViewerID(owner))
		if err != nil {
			return err
		}
		section := galleryFilterSection{Label: "Owners"}
		for _, owner := range owners {
			section.Options = append(section.Options, galleryFilterOption{Name: "owner", Value: strconv.FormatInt(owner.ID, 10), Label: owner.Name})
		}
		sections = append(sections, section)
	} else {
		collections, err := ro.cfg.Store.ListCollections(r.Context(), owner)
		if err != nil {
			return err
		}
		tags, err := ro.cfg.Store.ListTags(r.Context(), owner)
		if err != nil {
			return err
		}
		collectionSection := galleryFilterSection{Label: "Collections"}
		for _, collection := range collections {
			collectionSection.Options = append(collectionSection.Options, galleryFilterOption{Name: "collection", Value: collection.ID, Label: collection.Name})
		}
		tagSection := galleryFilterSection{Label: "Tags"}
		for _, tag := range tags {
			tagSection.Options = append(tagSection.Options, galleryFilterOption{Name: "tag", Value: tag.Name, Label: tag.Name, Color: color.Normalize(tag.Color)})
		}
		sections = append(sections, collectionSection, tagSection)
	}
	for _, section := range sections {
		if len(section.Options) > 0 {
			grid.Filters.Sections = append(grid.Filters.Sections, section)
		}
	}
	// Incoming owner validation/canonicalization belongs to selectSharedWith.
	if gallery.ReadOnly {
		return nil
	}
	params := url.Values{}
	for _, section := range grid.Filters.Sections {
		name := section.Options[0].Name
		for _, value := range r.URL.Query()[name] {
			if !slices.ContainsFunc(section.Options, func(option galleryFilterOption) bool { return option.Value == value }) {
				return errGalleryFilter
			}
			if !slices.Contains(params[name], value) {
				params.Add(name, value)
			}
		}
	}
	// A missing axis must reject requested values too.
	for _, name := range []string{"tag", "collection"} {
		if len(r.URL.Query()[name]) > 0 && len(params[name]) == 0 {
			return errGalleryFilter
		}
	}
	grid.Parameters = params

	return nil
}

func (grid *galleryGridData) prepareFilterLinks(gallery galleryDefinition) {
	for i := range grid.Filters.Sections {
		for j := range grid.Filters.Sections[i].Options {
			option := &grid.Filters.Sections[i].Options[j]
			option.Active = slices.Contains(grid.Parameters[option.Name], option.Value)
			next := *grid
			next.Parameters = cloneGalleryParameters(grid.Parameters)
			if option.Active {
				next.Parameters[option.Name] = slices.DeleteFunc(next.Parameters[option.Name], func(value string) bool { return value == option.Value })
			} else {
				next.Parameters.Add(option.Name, option.Value)
			}
			option.PageURL, option.FragmentURL = next.url(gallery.PagePath, 0), next.url(gallery.FragmentPath, 0)
			if option.Active {
				grid.Filters.Chips = append(grid.Filters.Chips, *option)
			}
		}
	}
	clear := *grid
	clear.Parameters = nil
	grid.Filters.ClearPageURL, grid.Filters.ClearFragmentURL = clear.url(gallery.PagePath, 0), clear.url(gallery.FragmentPath, 0)
}

func cloneGalleryParameters(parameters url.Values) url.Values {
	copy := url.Values{}
	for name, values := range parameters {
		copy[name] = append([]string(nil), values...)
	}
	return copy
}
