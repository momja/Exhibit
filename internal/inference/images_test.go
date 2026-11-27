package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
)

// Images in complete() (Exh-v81w).

func encodedImage(t *testing.T, mimeType string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	var err error
	switch mimeType {
	case "image/png":
		err = png.Encode(&buf, img)
	case "image/jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "image/gif":
		err = gif.Encode(&buf, img, nil)
	case "image/webp":
		// The smallest prefix http.DetectContentType reads as WebP.
		buf.WriteString("RIFF\x1a\x00\x00\x00WEBPVP8 \x0e\x00\x00\x00")
		buf.Write(make([]byte, 14))
	default:
		t.Fatalf("no fixture for %s", mimeType)
	}
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// sized is a PNG signature padded to n bytes: DetectContentType reads only
// the prefix, which is all the size limits need.
func sized(n int) string {
	raw := make([]byte, n)
	copy(raw, "\x89PNG\r\n\x1a\n")
	return base64.StdEncoding.EncodeToString(raw)
}

func imageCall(t *testing.T, viewer int64, payload any) Call {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return Call{ArtifactID: "art-1", ViewerID: viewer, Kind: KindComplete, Payload: b}
}

func imageMessage(parts ...map[string]any) map[string]any {
	return map[string]any{"messages": []any{map[string]any{"role": "user", "content": parts}}}
}

func textPart(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

func imgPart(mimeType, data string) map[string]any {
	return map[string]any{"type": "image", "mimeType": mimeType, "data": data}
}

// piContext is the part of a job's context these tests read.
type piContext struct {
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

func decodeContext(t *testing.T, raw json.RawMessage) piContext {
	t.Helper()
	var c piContext
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestStringContentKeepsItsShape(t *testing.T) {
	s, rt, _ := newTestService(t, nil)
	if _, err := s.Run(context.Background(), completeCall(owner)); err != nil {
		t.Fatal(err)
	}
	c := decodeContext(t, rt.jobs[0].Context)
	if string(c.Messages[0].Content) != `"Hi"` {
		t.Fatalf("a string message reached Pi as %s; want the plain string it always was", c.Messages[0].Content)
	}
}

func TestImagePartsReachTheProviderInOrder(t *testing.T) {
	s, rt, led := newTestService(t, nil)
	jpg := encodedImage(t, "image/jpeg")
	call := imageCall(t, recipient, imageMessage(
		textPart("What is in this receipt?"),
		imgPart("image/png", encodedImage(t, "image/png")),
		// A data: URL, as canvas.toDataURL() returns, needs no mimeType.
		map[string]any{"type": "image", "data": "data:image/jpeg;base64," + jpg},
	))
	if _, err := s.Run(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]string
	if err := json.Unmarshal(decodeContext(t, rt.jobs[0].Context).Messages[0].Content, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 3 || blocks[0]["type"] != "text" || blocks[1]["type"] != "image" || blocks[1]["mimeType"] != "image/png" ||
		blocks[2]["type"] != "image" || blocks[2]["mimeType"] != "image/jpeg" || blocks[2]["data"] != jpg {
		t.Fatalf("blocks = %v", blocks)
	}
	// Metered like any other call, against the viewer who sent it.
	if len(led.rows) != 1 || led.rows[0].OwnerID != recipient || led.rows[0].Kind != "complete" || led.rows[0].CostMicros != 1000 {
		t.Fatalf("rows = %+v", led.rows)
	}
}

func TestEveryImageTypeIsAccepted(t *testing.T) {
	s, _, _ := newTestService(t, nil)
	for _, mimeType := range []string{"image/png", "image/jpeg", "image/webp", "image/gif"} {
		if _, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(imgPart(mimeType, encodedImage(t, mimeType))))); err != nil {
			t.Errorf("%s: %v", mimeType, err)
		}
	}
}

func TestImageLimitsAreRefusedBeforeAnyCall(t *testing.T) {
	png1 := encodedImage(t, "image/png")
	tooMany := []map[string]any{}
	for range MaxCompleteImages + 1 {
		tooMany = append(tooMany, imgPart("image/png", png1))
	}
	manyParts := []map[string]any{}
	for range MaxContentParts + 1 {
		manyParts = append(manyParts, textPart("x"))
	}
	big := sized(MaxImageBytes + 1)
	third := sized(MaxImageTotalBytes/3 + 1)
	for _, tc := range []struct {
		name    string
		payload any
		want    string // a phrase the message must contain
	}{
		{"svg", imageMessage(imgPart("image/svg+xml", png1)), "PNG, JPEG, WebP or GIF"},
		{"no type", imageMessage(imgPart("", png1)), "PNG, JPEG, WebP or GIF"},
		{"mislabelled", imageMessage(imgPart("image/jpeg", png1)), "not the image/jpeg"},
		{"not base64", imageMessage(imgPart("image/png", "!!not base64!!")), "base64"},
		{"empty", imageMessage(imgPart("image/png", "")), "base64"},
		{"data url type mismatch", imageMessage(imgPart("image/jpeg", "data:image/png;base64,"+png1)), "does not match"},
		{"data url not base64", imageMessage(imgPart("image/png", "data:image/png,"+png1)), "base64"},
		{"too big", imageMessage(imgPart("image/png", big)), "2 MB"},
		{"too many", imageMessage(tooMany...), "at most 4 images"},
		{"too much in total", imageMessage(imgPart("image/png", third), imgPart("image/png", third), imgPart("image/png", third)), "4 MB in total"},
		{"assistant image", map[string]any{"messages": []any{
			map[string]any{"role": "assistant", "content": []any{imgPart("image/png", png1)}},
			map[string]any{"role": "user", "content": "and this?"},
		}}, "Only user messages"},
		{"unknown part field", imageMessage(map[string]any{"type": "image", "mimeType": "image/png", "data": png1, "url": "x"}), "content must be text"},
		{"unknown part type", imageMessage(map[string]any{"type": "audio", "data": png1}), "type must be"},
		{"empty text part", imageMessage(textPart("  "), imgPart("image/png", png1)), "non-empty text"},
		{"text and data", imageMessage(map[string]any{"type": "image", "text": "x", "mimeType": "image/png", "data": png1}), "image part is"},
		{"empty list", map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{}}}}, "between 1 and"},
		{"too many parts", imageMessage(manyParts...), "between 1 and"},
		{"object content", map[string]any{"messages": []any{map[string]any{"role": "user", "content": map[string]any{"type": "text"}}}}, "content must be text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, rt, led := newTestService(t, nil)
			call := imageCall(t, owner, tc.payload)
			_, err := s.Run(context.Background(), call)
			if code(err) != CodeInvalidRequest {
				t.Fatalf("err = %v; want invalid_request", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("message %q does not name the rule (%q)", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), png1) || strings.Contains(err.Error(), big[:64]) {
				t.Fatal("an error message quoted image data")
			}
			if rt.calls() != 0 || len(led.rows) != 0 {
				t.Fatal("a refused image call reached the provider or the ledger")
			}
		})
	}
}

func TestImageBytesDoNotCountAgainstTheTextCap(t *testing.T) {
	s, _, _ := newTestService(t, nil)
	// Far more base64 than MaxCompleteTextBytes, under the image limits.
	data := sized(MaxImageBytes)
	if _, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(textPart("Describe it."), imgPart("image/png", data)))); err != nil {
		t.Fatal(err)
	}
	// Text parts still do.
	long := strings.Repeat("a", MaxCompleteTextBytes+1)
	if _, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(textPart(long), imgPart("image/png", encodedImage(t, "image/png"))))); code(err) != CodeInvalidRequest {
		t.Fatalf("over-long text beside an image = %v", err)
	}
}

func TestATextOnlyModelRefusesImagesBeforeAnyCall(t *testing.T) {
	s, rt, led := newTestService(t, func(c *Config) { c.ChatModel = "text-only-1" })
	if av := s.Availability(context.Background(), owner); !av.Complete || av.Images {
		t.Fatalf("availability = %+v; want complete without images", av)
	}
	if s.InstanceImages(context.Background()) {
		t.Fatal("a text-only platform model offered images to agents")
	}
	_, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(imgPart("image/png", encodedImage(t, "image/png")))))
	if code(err) != CodeUnavailable || !strings.Contains(err.Error(), "available().images") {
		t.Fatalf("err = %v", err)
	}
	if rt.calls() != 0 || len(led.rows) != 0 {
		t.Fatal("an image reached a text-only model")
	}
	// Text still works on the same model.
	if _, err := s.Run(context.Background(), completeCall(owner)); err != nil {
		t.Fatal(err)
	}
}

func TestAVisionModelOffersImages(t *testing.T) {
	s, _, _ := newTestService(t, nil)
	if av := s.Availability(context.Background(), owner); !av.Images {
		t.Fatalf("availability = %+v", av)
	}
	if !s.InstanceImages(context.Background()) {
		t.Fatal("a vision platform model did not offer images to agents")
	}
}

// The guardrail screens text. Rather than let images pass a screen that
// cannot read them, a screened instance does not offer them.
func TestAScreenedInstanceRefusesImages(t *testing.T) {
	s, rt, _ := newTestService(t, configureTestGuardrail)
	if av := s.Availability(context.Background(), owner); av.Images || !av.Complete {
		t.Fatalf("availability = %+v", av)
	}
	if s.InstanceImages(context.Background()) {
		t.Fatal("a screened instance offered images to agents")
	}
	_, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(imgPart("image/png", encodedImage(t, "image/png")))))
	if code(err) != CodeUnavailable || rt.calls() != 0 || len(rt.screens) != 0 {
		t.Fatalf("err = %v, calls = %d, screens = %d", err, rt.calls(), len(rt.screens))
	}
}

func TestBYOKImagesFollowTheViewersOwnModel(t *testing.T) {
	keys := map[int64]Credential{owner: {Provider: "openrouter", APIKey: "owner-key"}, recipient: {Provider: "openrouter", APIKey: "recipient-key"}}
	s, rt, _ := newTestService(t, func(c *Config) {
		c.Platform = nil
		c.ChatModel = "text-only-1"
		c.Keys = func(_ context.Context, v int64) (Credential, bool, error) { k, ok := keys[v]; return k, ok, nil }
	})
	if !s.InstanceImages(context.Background()) {
		t.Fatal("BYOK guidance must teach images: availability is per viewer")
	}
	if s.Availability(context.Background(), owner).Images {
		t.Fatal("an OpenRouter viewer on a text-only INFERENCE_MODEL was offered images")
	}
	keys[recipient] = Credential{Provider: "anthropic", APIKey: "recipient-key"}
	if !s.Availability(context.Background(), recipient).Images {
		t.Fatal("an Anthropic viewer falling back to a vision default was not offered images")
	}
	if _, err := s.Run(context.Background(), imageCall(t, recipient, imageMessage(imgPart("image/png", encodedImage(t, "image/png"))))); err != nil {
		t.Fatal(err)
	}
	if rt.jobs[0].APIKey != "recipient-key" {
		t.Fatalf("job = %+v", rt.jobs[0])
	}
}

// A schema repair fixes the format of an answer already written. Sending the
// image again would bill a second read of it, so the repair carries none.
func TestASchemaRepairDoesNotResendImages(t *testing.T) {
	s, rt, led := newTestService(t, nil)
	n := 0
	rt.reply = func(Job) (Result, Outcome, error) {
		n++
		text := "not json"
		if n == 2 {
			text = `{"total": 12.5}`
		}
		return Result{StopReason: "stop", Text: text}, Outcome{Dispatched: true, Usage: &Usage{Input: 900, Output: 5, Cost: UsageCost{Total: 0.002}}}, nil
	}
	data := encodedImage(t, "image/png")
	payload := imageMessage(textPart("Read the total."), imgPart("image/png", data))
	payload["schema"] = map[string]any{"type": "object", "properties": map[string]any{"total": map[string]any{"type": "number"}}, "required": []any{"total"}}
	out, err := s.Run(context.Background(), imageCall(t, owner, payload))
	if err != nil || string(out) != `{"total":12.5}` {
		t.Fatalf("run = %s, %v", out, err)
	}
	if rt.calls() != 2 {
		t.Fatalf("calls = %d", rt.calls())
	}
	if !strings.Contains(string(rt.jobs[0].Context), data) {
		t.Fatal("the first call did not carry the image")
	}
	repair := string(rt.jobs[1].Context)
	if strings.Contains(repair, data) || strings.Contains(repair, `"type":"image"`) {
		t.Fatal("the repair re-sent the image")
	}
	if !strings.Contains(repair, repairImageNote) {
		t.Fatal("the repair does not tell the model an image was left out")
	}
	// Both calls metered, as for text.
	if len(led.rows) != 2 || led.rows[0].SessionID != led.rows[1].SessionID {
		t.Fatalf("rows = %+v", led.rows)
	}
}

func TestImageCallsAreRefusedOnceTheAllowanceIsSpent(t *testing.T) {
	capMicros := int64(500)
	s, rt, led := newTestService(t, func(c *Config) { c.Caps = SpendCaps{OwnerMicrosPerMonth: &capMicros} })
	led.rows = append(led.rows, store.AgentUsage{OwnerID: owner, PaidBy: store.PaidByPlatform, CostMicros: 500})
	_, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(imgPart("image/png", encodedImage(t, "image/png")))))
	if code(err) != CodeLimitReached || rt.calls() != 0 {
		t.Fatalf("err = %v, calls = %d", err, rt.calls())
	}
}
