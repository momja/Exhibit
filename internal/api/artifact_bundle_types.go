package api

import "github.com/momja/Exhibit/internal/store"

const artifactBundleFormat = "exhibit.artifact"
const artifactBundleVersion = 1

// The manifest names logical pages rather than assuming one document. Source
// paths retain editable bytes; portable paths are self-contained HTML previews.
// Readers must reject unsupported versions and unsupported page configurations.
type artifactBundleManifest struct {
	Format       string                 `json:"format"`
	Version      int                    `json:"version"`
	ArtifactID   string                 `json:"artifact_id"`
	RenderOrigin string                 `json:"render_origin"`
	Artifact     artifactBundleMetadata `json:"artifact"`
	EntryPage    string                 `json:"entry_page"`
	Pages        []artifactBundlePage   `json:"pages"`
	Widget       *artifactBundlePage    `json:"widget,omitempty"`
	StatePath    string                 `json:"state_path"`
	Assets       []exportedAsset        `json:"assets"`
}

type artifactBundlePage struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	SourcePath string `json:"source_path"`
}

// Permissions are recorded for archival fidelity. Import deliberately starts
// with fresh network/capability approvals and private sharing defaults.
type artifactBundleMetadata struct {
	Title               string     `json:"title"`
	SourceURL           string     `json:"source_url"`
	Tier                store.Tier `json:"tier"`
	NetworkAllowlist    []string   `json:"network_allowlist"`
	DownloadsApproved   bool       `json:"downloads_approved"`
	ClipboardApproved   bool       `json:"clipboard_approved"`
	LinksApproved       bool       `json:"links_approved"`
	CameraApproved      bool       `json:"camera_approved"`
	MicrophoneApproved  bool       `json:"microphone_approved"`
	GeolocationApproved bool       `json:"geolocation_approved"`
	ShareStateMode      string     `json:"share_state_mode"`
}
