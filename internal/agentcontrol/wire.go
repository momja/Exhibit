package agentcontrol

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

const (
	CheckpointRevisionHeader      = "X-Exhibit-Checkpoint-Revision"
	CheckpointKindHeader          = "X-Exhibit-Checkpoint-Kind"
	ConversationTitleHeader       = "X-Exhibit-Conversation-Title"
	ArtifactVersionHeader         = "X-Exhibit-Artifact-Version"
	PreviousExecutionStatusHeader = "X-Exhibit-Previous-Execution-Status"
	MaxUsageReportBytes           = 16 << 10
)

var ErrReportTooLarge = errors.New("usage report exceeds 16 KiB")

// ReadCheckpointRequest is the standalone supervisor protocol's bounded raw
// body decoder. It is intentionally separate from the browser API's generic
// JSON decoder/32 MiB limit: JSON escaping must not multiply snapshot size.
func ReadCheckpointRequest(w http.ResponseWriter, r *http.Request) (Checkpoint, error) {
	if r.ContentLength > MaxSnapshotBytes {
		return Checkpoint{}, ErrTooLarge
	}
	revision, err := strconv.ParseInt(r.Header.Get(CheckpointRevisionHeader), 10, 64)
	if err != nil || revision <= 0 {
		return Checkpoint{}, ErrInvalid
	}
	p := Checkpoint{Revision: revision, Kind: r.Header.Get(CheckpointKindHeader), Title: r.Header.Get(ConversationTitleHeader)}
	if len(p.Title) > 512 || (p.Kind != CheckpointTool && p.Kind != CheckpointSettled && p.Kind != CheckpointInterrupted) {
		return Checkpoint{}, ErrInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxSnapshotBytes)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return Checkpoint{}, ErrTooLarge
		}
		return Checkpoint{}, ErrInvalid
	}
	p.SessionFile = string(b)
	return p, ValidateCheckpoint(p)
}
func ReadUsageRequest(w http.ResponseWriter, r *http.Request) (Usage, error) {
	if r.ContentLength > MaxUsageReportBytes {
		return Usage{}, ErrReportTooLarge
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUsageReportBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var u Usage
	if err := decoder.Decode(&u); err != nil {
		return Usage{}, usageDecodeError(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Usage{}, usageDecodeError(err)
	}
	return u, ValidateUsage(u)
}
func usageDecodeError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return ErrReportTooLarge
	}
	return ErrInvalid
}
