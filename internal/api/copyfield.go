package api

// copyFieldView supplies a labelled, read-only single-line value with an inline
// clipboard action. IDs must be unique within the document.
type copyFieldView struct {
	ID        string
	Label     string
	CopyLabel string
	Value     string
	HideLabel bool
}
