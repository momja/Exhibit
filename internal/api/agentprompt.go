package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/momja/Exhibit/internal/agent"
)

const (
	maxAgentMessageBytes    = 256 << 10
	maxAgentImages          = 8
	maxAgentImageBytes      = 5 << 20
	maxAgentTotalImageBytes = 16 << 20
)

// Validate before Prompt touches selection files, credentials, or subprocess
// stdin. The service-wide body ceiling remains a separate transport bound.
func (req agentPromptRequest) validate() ([]agent.ImageContent, error) {
	if len(req.Message) > maxAgentMessageBytes {
		return nil, fmt.Errorf("message exceeds the %d byte limit", maxAgentMessageBytes)
	}
	if strings.TrimSpace(req.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}
	return promptImages(req.Images)
}

// Prompt and question replies share attachment validation before either path
// writes a selection file or delivers user input to the waiting sidecar.
func promptImages(input []agentPromptImage) ([]agent.ImageContent, error) {
	if len(input) > maxAgentImages {
		return nil, fmt.Errorf("at most %d images may be attached", maxAgentImages)
	}
	images := make([]agent.ImageContent, 0, len(input))
	total := 0
	for i, im := range input {
		mime := im.MimeType
		if mime == "" {
			mime = "image/png"
		}
		switch mime {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
		default:
			return nil, fmt.Errorf("image %d has an unsupported MIME type", i+1)
		}
		if len(im.Data) == 0 {
			return nil, fmt.Errorf("image %d is empty", i+1)
		}
		if len(im.Data) > base64.StdEncoding.EncodedLen(maxAgentImageBytes) {
			return nil, fmt.Errorf("image %d exceeds the %d byte decoded limit", i+1, maxAgentImageBytes)
		}
		data, err := base64.StdEncoding.Strict().DecodeString(im.Data)
		if err != nil {
			return nil, fmt.Errorf("image %d is not valid base64", i+1)
		}
		if len(data) > maxAgentImageBytes {
			return nil, fmt.Errorf("image %d exceeds the %d byte decoded limit", i+1, maxAgentImageBytes)
		}
		total += len(data)
		if total > maxAgentTotalImageBytes {
			return nil, fmt.Errorf("images exceed the %d byte total decoded limit", maxAgentTotalImageBytes)
		}
		if http.DetectContentType(data) != mime {
			return nil, fmt.Errorf("image %d does not match its declared MIME type", i+1)
		}
		images = append(images, agent.ImageContent{Type: "image", Data: im.Data, MimeType: mime})
	}
	return images, nil
}
