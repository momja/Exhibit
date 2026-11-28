package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// A kind is what is particular to one family of call: how its payload is
// checked and bounded before anything is spent, how the provider's answer is
// checked before the artifact sees it, and its per-call defaults. Payer,
// limits and metering are the core's (inference.go).
type kindSpec struct {
	timeout time.Duration
	prepare func(payload json.RawMessage) (*prepared, error)
	run     func(ctx context.Context, rt Runtime, job Job, req *prepared, meter func(Outcome), gate func(Job) error) (json.RawMessage, error)
}

// prepared is a validated payload.
type prepared struct {
	complete *completeRequest
	decide   *decideRequest
}

var kinds = map[Kind]kindSpec{
	KindComplete: {timeout: 60 * time.Second, prepare: prepareComplete, run: runComplete},
	KindDecide:   {timeout: 20 * time.Second, prepare: prepareDecide, run: runDecide},
}

// Bounds. Each is well inside what the providers accept; together they keep
// one call's input under the worker's envelope limit.
const (
	MaxCompleteTextBytes  = 48 << 10
	MaxCompleteMessages   = 40
	DefaultMaxTokens      = 1024
	MaxMaxTokens          = 4096
	MaxSchemaBytes        = 8 << 10
	MaxDecideStateBytes   = 16 << 10
	MaxDecideQuestions    = 8
	MaxChoiceOptions      = 16
	MaxScoreLevels        = 11
	MaxInstructionsBytes  = 2000
	MaxCriterionBytes     = 500
	maxSchemaDepth        = 8
	repairInstructionText = "Your previous reply did not match the required JSON Schema (%s). Reply again with only a JSON value that matches it: no prose, no code fences."
)

// systemPreamble is the fixed, server-authored start of every complete()
// system prompt. It frames the artifact's own text as a request from a tool,
// not as instructions from Exhibit. internal/mockllm recognizes it.
const systemPreamble = "You are answering a request from a small web tool that a person is using in Exhibit, a personal library of web tools. The tool's author wrote the instructions below; follow them as the tool's instructions."

func invalid(message string) error { return refuse(CodeInvalidRequest, message) }

// --- complete -------------------------------------------------------------

// completeMessage is one turn of a complete() conversation. Its parts keep
// the order the artifact gave them.
type completeMessage struct {
	Role  string
	Parts []contentPart
}

// contentPart is a run of text or one image; Image is nil for text.
type contentPart struct {
	Text  string
	Image *imagePart
}

// imagePart is a validated image (Exh-v81w): an allowed type whose bytes
// were decoded, sniffed and stripped of metadata (imagemeta.go), carried on
// as canonical base64.
type imagePart struct {
	MimeType string
	Data     string
	// size is the decoded length as received, what the image bounds count.
	size int
}

func textMessage(role, text string) completeMessage {
	return completeMessage{Role: role, Parts: []contentPart{{Text: text}}}
}

type completeRequest struct {
	System   string
	Messages []completeMessage
	// images counts the image parts across all messages.
	images    int
	Schema    map[string]any
	SchemaRaw string
	MaxTokens int
}

// Image bounds (Exh-v81w), separate from the text cap. MaxImageTotalBytes,
// as base64, is what sizes the worker's envelope (maxEnvelope).
const (
	MaxCompleteImages  = 4
	MaxImageBytes      = 2 << 20
	MaxImageTotalBytes = 4 << 20
	MaxContentParts    = 16
)

// imageTypes are the image types complete() accepts: the ones every
// provider Pi reaches reads (Gemini takes no GIF, so GIF is refused here
// rather than at the provider), and that http.DetectContentType sniffs.
var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true}

// repairImageNote replaces an image in a schema repair's context: the repair
// fixes the format of an answer already written, and re-sending the image
// would bill a second read of it.
const repairImageNote = "[An image was attached here. It is not sent again with this retry.]"

func prepareComplete(payload json.RawMessage) (*prepared, error) {
	type wireMessage struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	var in struct {
		Messages  []wireMessage   `json:"messages"`
		Prompt    *string         `json:"prompt"`
		System    string          `json:"system"`
		Schema    json.RawMessage `json:"schema"`
		MaxTokens *int            `json:"maxTokens"`
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return nil, invalid("complete() takes { messages, system?, schema?, maxTokens? }.")
	}
	if in.Prompt != nil {
		if len(in.Messages) > 0 {
			return nil, invalid("Pass either prompt or messages, not both.")
		}
		content, _ := json.Marshal(*in.Prompt)
		in.Messages = []wireMessage{{Role: "user", Content: content}}
	}
	if len(in.Messages) == 0 || len(in.Messages) > MaxCompleteMessages {
		return nil, invalid(fmt.Sprintf("complete() needs between 1 and %d messages.", MaxCompleteMessages))
	}
	req := &completeRequest{System: in.System, MaxTokens: DefaultMaxTokens}
	size := len(in.System)
	budget := &imageBudget{}
	for i, m := range in.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			return nil, invalid("Each message's role must be \"user\" or \"assistant\".")
		}
		if i == len(in.Messages)-1 && m.Role != "user" {
			return nil, invalid("The last message must be from the user.")
		}
		parts, err := parseContent(m.Role, m.Content, budget)
		if err != nil {
			return nil, err
		}
		for _, part := range parts {
			if part.Image == nil {
				size += len(part.Text)
			}
		}
		req.Messages = append(req.Messages, completeMessage{Role: m.Role, Parts: parts})
	}
	if size > MaxCompleteTextBytes {
		return nil, invalid(fmt.Sprintf("complete() input is limited to %d KB of text.", MaxCompleteTextBytes>>10))
	}
	req.images = budget.count
	if in.MaxTokens != nil {
		if *in.MaxTokens < 1 {
			return nil, invalid("maxTokens must be a positive integer.")
		}
		req.MaxTokens = min(*in.MaxTokens, MaxMaxTokens)
	}
	if len(in.Schema) > 0 && string(in.Schema) != "null" {
		if len(in.Schema) > MaxSchemaBytes {
			return nil, invalid(fmt.Sprintf("A schema is limited to %d KB.", MaxSchemaBytes>>10))
		}
		var schema map[string]any
		if json.Unmarshal(in.Schema, &schema) != nil {
			return nil, invalid("schema must be a JSON Schema object.")
		}
		if err := checkSchema(schema, 0); err != nil {
			return nil, invalid("Unsupported schema: " + err.Error() + ".")
		}
		compact, _ := json.Marshal(schema)
		req.Schema, req.SchemaRaw = schema, string(compact)
	}
	return &prepared{complete: req}, nil
}

// imageBudget is the request's image bounds, spent as parts are read, so an
// image over the count is refused before its bytes are decoded.
type imageBudget struct{ count, bytes int }

func (b *imageBudget) admit() error {
	if b.count++; b.count > MaxCompleteImages {
		return invalid(fmt.Sprintf("complete() takes at most %d images.", MaxCompleteImages))
	}
	return nil
}

func (b *imageBudget) spend(size int) error {
	if b.bytes += size; b.bytes > MaxImageTotalBytes {
		return invalid(fmt.Sprintf("complete() images are limited to %d MB in total. Downscale them in a canvas before sending.", MaxImageTotalBytes>>20))
	}
	return nil
}

// parseContent reads one message's content: a string, as before images, or
// a list of text and image parts. Only a user message may carry an image.
func parseContent(role string, raw json.RawMessage, budget *imageBudget) ([]contentPart, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if strings.TrimSpace(text) == "" {
			return nil, invalid("A message's content must be non-empty text.")
		}
		return []contentPart{{Text: text}}, nil
	}
	type wirePart struct {
		Type     string  `json:"type"`
		Text     *string `json:"text"`
		MimeType string  `json:"mimeType"`
		Data     *string `json:"data"`
	}
	shape := invalid("A message's content must be text, or a list of { type: 'text', text } and { type: 'image', mimeType, data } parts.")
	count := invalid(fmt.Sprintf("A message's content list needs between 1 and %d parts.", MaxContentParts))
	// Parts are read one at a time, so a list far over the bound is refused
	// at its first extra part, never decoded whole.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
		return nil, shape
	}
	var wire []wirePart
	for dec.More() {
		if len(wire) == MaxContentParts {
			return nil, count
		}
		var w wirePart
		if dec.Decode(&w) != nil {
			return nil, shape
		}
		wire = append(wire, w)
	}
	if _, err := dec.Token(); err != nil {
		return nil, shape
	}
	if len(wire) == 0 {
		return nil, count
	}
	parts := make([]contentPart, 0, len(wire))
	for _, w := range wire {
		switch w.Type {
		case "text":
			if w.Text == nil || strings.TrimSpace(*w.Text) == "" || w.Data != nil || w.MimeType != "" {
				return nil, invalid("A text part is { type: 'text', text } with non-empty text.")
			}
			parts = append(parts, contentPart{Text: *w.Text})
		case "image":
			if role != "user" {
				return nil, invalid("Only user messages may carry images.")
			}
			if w.Text != nil || w.Data == nil {
				return nil, invalid("An image part is { type: 'image', mimeType, data } with base64 data.")
			}
			if err := budget.admit(); err != nil {
				return nil, err
			}
			img, err := parseImage(w.MimeType, *w.Data)
			if err != nil {
				return nil, err
			}
			if err := budget.spend(img.size); err != nil {
				return nil, err
			}
			parts = append(parts, contentPart{Image: img})
		default:
			return nil, invalid("A content part's type must be \"text\" or \"image\".")
		}
	}
	return parts, nil
}

// parseImage checks one image: an allowed type, base64 that decodes, within
// the per-image bound, and bytes that really are that type, which are then
// stripped of their metadata. data may also be a base64 data: URL, as
// canvas.toDataURL() returns, of the same type. Messages never quote the
// data.
func parseImage(mimeType, data string) (*imagePart, error) {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if rest, ok := strings.CutPrefix(data, "data:"); ok {
		header, encoded, ok := strings.Cut(rest, ",")
		urlType, isBase64 := strings.CutSuffix(strings.ToLower(header), ";base64")
		if !ok || !isBase64 {
			return nil, invalid("An image's data must be base64, or a base64 data: URL.")
		}
		if mimeType == "" {
			mimeType = urlType
		} else if urlType != mimeType {
			return nil, invalid("An image's mimeType does not match its data: URL.")
		}
		data = encoded
	}
	if !imageTypes[mimeType] {
		return nil, invalid("Images must be PNG, JPEG or WebP (mimeType image/png, image/jpeg or image/webp).")
	}
	limit := fmt.Sprintf("Each image is limited to %d MB. Downscale it in a canvas before sending.", MaxImageBytes>>20)
	// The decoder skips line breaks, so they do not count toward the size.
	data = strings.NewReplacer("\r", "", "\n", "").Replace(data)
	if base64.StdEncoding.DecodedLen(len(data)) > MaxImageBytes+2 {
		return nil, invalid(limit)
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(raw) == 0 {
		return nil, invalid("An image's data must be non-empty base64.")
	}
	if len(raw) > MaxImageBytes {
		return nil, invalid(limit)
	}
	if http.DetectContentType(raw) != mimeType {
		return nil, invalid("An image's data is not the " + mimeType + " its mimeType says.")
	}
	// The bounds above count the bytes as received, and so does size:
	// stripping only ever makes an image smaller.
	stripped, err := stripImageMetadata(mimeType, raw)
	if err != nil {
		return nil, invalid("An image's data is not a well-formed " + mimeType + " file.")
	}
	return &imagePart{MimeType: mimeType, Data: base64.StdEncoding.EncodeToString(stripped), size: len(raw)}, nil
}

// context is the Pi request context for the conversation plus extra turns.
// withImages false replaces each image with repairImageNote.
func (r *completeRequest) context(withImages bool, extra ...completeMessage) json.RawMessage {
	system := systemPreamble
	if r.System != "" {
		system += "\n\n" + r.System
	}
	if r.Schema != nil {
		system += "\n\nReply with only a JSON value, no prose and no code fences, that matches this JSON Schema:\n" + r.SchemaRaw
	}
	type piMessage struct {
		Role      string `json:"role"`
		Content   any    `json:"content"`
		Timestamp int64  `json:"timestamp"`
	}
	msgs := make([]piMessage, 0, len(r.Messages)+len(extra))
	now := time.Now().UnixMilli()
	for _, m := range append(append([]completeMessage{}, r.Messages...), extra...) {
		// A user message of one text part stays a plain string, the shape
		// every call had before images.
		if m.Role == "user" && len(m.Parts) == 1 && m.Parts[0].Image == nil {
			msgs = append(msgs, piMessage{Role: "user", Content: m.Parts[0].Text, Timestamp: now})
			continue
		}
		blocks := make([]map[string]string, 0, len(m.Parts))
		for _, part := range m.Parts {
			switch {
			case part.Image == nil:
				blocks = append(blocks, map[string]string{"type": "text", "text": part.Text})
			case withImages:
				blocks = append(blocks, map[string]string{"type": "image", "data": part.Image.Data, "mimeType": part.Image.MimeType})
			default:
				blocks = append(blocks, map[string]string{"type": "text", "text": repairImageNote})
			}
		}
		msgs = append(msgs, piMessage{Role: m.Role, Content: blocks, Timestamp: now})
	}
	out, _ := json.Marshal(map[string]any{"systemPrompt": system, "messages": msgs})
	return out
}

func runComplete(ctx context.Context, rt Runtime, job Job, p *prepared, meter func(Outcome), gate func(Job) error) (json.RawMessage, error) {
	req := p.complete
	job.MaxTokens = req.MaxTokens
	job.Context = req.context(true)
	res, err := callOnce(ctx, rt, job, meter, gate)
	if err != nil {
		return nil, err
	}
	// A reasoning model can spend all of maxTokens thinking and write nothing.
	// The call is billed and metered, but "" is not an answer, and a repair
	// cannot fix a reply that was never written.
	if strings.TrimSpace(res.Text) == "" {
		if res.StopReason == "length" {
			return nil, refuse(CodeInvalidOutput, "The AI ran out of room before it wrote an answer. Try a larger maxTokens.")
		}
		return nil, refuse(CodeInvalidOutput, "The AI did not write an answer.")
	}
	if req.Schema == nil {
		out, _ := json.Marshal(res.Text)
		return out, nil
	}
	value, verr := parseStructured(res.Text, req.Schema)
	if verr == nil {
		return value, nil
	}
	// One repair attempt: the model sees its own answer and what was wrong,
	// in server-authored words. The repair is a second call, metered as one.
	// It carries no images: they were read once, for the answer being fixed.
	job.Context = req.context(false,
		textMessage("assistant", res.Text),
		textMessage("user", fmt.Sprintf(repairInstructionText, verr.Error())),
	)
	res, err = callOnce(ctx, rt, job, meter, gate)
	if err != nil {
		return nil, err
	}
	value, verr = parseStructured(res.Text, req.Schema)
	if verr != nil {
		return nil, refuse(CodeInvalidOutput, "The AI's answer did not match the requested format.")
	}
	return value, nil
}

// callOnce runs one provider call, meters it, then maps its failure. The
// order matters: a call that failed may still have been billed. gate runs
// against the final job before every platform-paid dispatch, including repairs.
func callOnce(ctx context.Context, rt Runtime, job Job, meter func(Outcome), gate func(Job) error) (Result, error) {
	if gate != nil {
		if err := gate(job); err != nil {
			return Result{}, err
		}
	}
	res, outcome, err := rt.Run(ctx, job)
	meter(outcome)
	if err != nil {
		return Result{}, mapRuntimeError(ctx, err)
	}
	switch res.StopReason {
	case "stop", "length", "":
		return res, nil
	case "aborted":
		return Result{}, refuse(CodeProviderError, "The AI request took too long and was stopped.")
	default:
		return Result{}, refuse(CodeProviderError, "The AI service could not answer this request.")
	}
}

func mapRuntimeError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, ErrRuntimeBusy):
		return &Error{Code: CodeRateLimited, Message: "AI is busy right now. Try again in a moment.", RetryAfter: 2 * time.Second}
	case errors.Is(err, ErrRuntimeUnavailable), errors.Is(err, ErrNoModel):
		return refuse(CodeUnavailable, "AI is not available right now.")
	case ctx.Err() != nil:
		return refuse(CodeProviderError, "The AI request took too long and was stopped.")
	default:
		return refuse(CodeProviderError, "The AI service could not answer this request.")
	}
}

var fence = regexp.MustCompile("(?s)^```[a-zA-Z]*\\s*\n(.*?)\n?```$")

// parseStructured extracts one JSON value from a model's reply and validates
// it. A reply wrapped in a code fence is accepted; anything else around the
// value is not.
func parseStructured(text string, schema map[string]any) (json.RawMessage, error) {
	text = strings.TrimSpace(text)
	if m := fence.FindStringSubmatch(text); m != nil {
		text = strings.TrimSpace(m[1])
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, errors.New("the reply was not JSON")
	}
	if dec.More() {
		return nil, errors.New("the reply had text after the JSON value")
	}
	if err := validate(schema, v, "value"); err != nil {
		return nil, err
	}
	out, _ := json.Marshal(v)
	return out, nil
}

// The JSON Schema subset complete() accepts. A schema using anything else is
// refused before any call, rather than accepted and silently not enforced.
var schemaKeywords = map[string]bool{
	"type": true, "properties": true, "required": true, "items": true, "enum": true, "const": true,
	"additionalProperties": true, "minItems": true, "maxItems": true, "minimum": true, "maximum": true,
	"minLength": true, "maxLength": true, "description": true, "title": true, "$schema": true,
}

var schemaTypes = map[string]bool{"object": true, "array": true, "string": true, "number": true, "integer": true, "boolean": true, "null": true}

func checkSchema(s map[string]any, depth int) error {
	if depth > maxSchemaDepth {
		return fmt.Errorf("nested deeper than %d levels", maxSchemaDepth)
	}
	for k, v := range s {
		if !schemaKeywords[k] {
			return fmt.Errorf("the %q keyword is not supported", k)
		}
		switch k {
		case "type":
			names := []any{v}
			if list, ok := v.([]any); ok {
				names = list
			}
			for _, n := range names {
				if name, _ := n.(string); !schemaTypes[name] {
					return fmt.Errorf("unknown type %v", n)
				}
			}
		case "properties":
			props, ok := v.(map[string]any)
			if !ok {
				return errors.New("properties must be an object")
			}
			for _, sub := range props {
				m, ok := sub.(map[string]any)
				if !ok {
					return errors.New("each property must be a schema")
				}
				if err := checkSchema(m, depth+1); err != nil {
					return err
				}
			}
		case "items":
			m, ok := v.(map[string]any)
			if !ok {
				return errors.New("items must be a schema")
			}
			if err := checkSchema(m, depth+1); err != nil {
				return err
			}
		case "required":
			list, ok := v.([]any)
			if !ok {
				return errors.New("required must be a list of names")
			}
			for _, n := range list {
				if _, ok := n.(string); !ok {
					return errors.New("required must be a list of names")
				}
			}
		case "enum":
			if _, ok := v.([]any); !ok {
				return errors.New("enum must be a list")
			}
		case "additionalProperties":
			if _, ok := v.(bool); !ok {
				return errors.New("additionalProperties must be true or false")
			}
		case "minItems", "maxItems", "minimum", "maximum", "minLength", "maxLength":
			if _, ok := v.(float64); !ok {
				return fmt.Errorf("%s must be a number", k)
			}
		}
	}
	return nil
}

// validate checks a decoded value (numbers as json.Number) against a schema
// that passed checkSchema. Messages name a path, never the value.
func validate(s map[string]any, v any, path string) error {
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if jsonEqual(e, v) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s is not one of the allowed values", path)
		}
	}
	if c, ok := s["const"]; ok && !jsonEqual(c, v) {
		return fmt.Errorf("%s is not the required value", path)
	}
	if t, ok := s["type"]; ok {
		names := []any{t}
		if list, ok := t.([]any); ok {
			names = list
		}
		match := false
		for _, n := range names {
			if hasType(n.(string), v) {
				match = true
				break
			}
		}
		if !match {
			return fmt.Errorf("%s has the wrong type", path)
		}
	}
	switch val := v.(type) {
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		if req, ok := s["required"].([]any); ok {
			for _, n := range req {
				if _, ok := val[n.(string)]; !ok {
					return fmt.Errorf("%s is missing %q", path, n)
				}
			}
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sub, ok := props[k].(map[string]any)
			if !ok {
				if ap, set := s["additionalProperties"].(bool); set && !ap {
					return fmt.Errorf("%s has an unexpected property %q", path, k)
				}
				continue
			}
			if err := validate(sub, val[k], path+"."+k); err != nil {
				return err
			}
		}
	case []any:
		if n, ok := s["minItems"].(float64); ok && float64(len(val)) < n {
			return fmt.Errorf("%s has too few items", path)
		}
		if n, ok := s["maxItems"].(float64); ok && float64(len(val)) > n {
			return fmt.Errorf("%s has too many items", path)
		}
		if items, ok := s["items"].(map[string]any); ok {
			for i, item := range val {
				if err := validate(items, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case string:
		n := float64(utf8.RuneCountInString(val))
		if m, ok := s["minLength"].(float64); ok && n < m {
			return fmt.Errorf("%s is too short", path)
		}
		if m, ok := s["maxLength"].(float64); ok && n > m {
			return fmt.Errorf("%s is too long", path)
		}
	case json.Number:
		f, _ := val.Float64()
		if m, ok := s["minimum"].(float64); ok && f < m {
			return fmt.Errorf("%s is below the minimum", path)
		}
		if m, ok := s["maximum"].(float64); ok && f > m {
			return fmt.Errorf("%s is above the maximum", path)
		}
	}
	return nil
}

func hasType(name string, v any) bool {
	switch name {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "number":
		_, ok := v.(json.Number)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		f, err := n.Float64()
		return err == nil && f == math.Trunc(f)
	}
	return false
}

func jsonEqual(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	var na, nb any
	_ = json.Unmarshal(ja, &na)
	_ = json.Unmarshal(jb, &nb)
	ca, _ := json.Marshal(na)
	cb, _ := json.Marshal(nb)
	return bytes.Equal(ca, cb)
}

// --- decide ---------------------------------------------------------------

type decideQuestion struct {
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria"`

	options []string // choice labels, sorted
	levels  int      // score levels
}

type decideRequest struct {
	State     json.RawMessage
	Questions map[string]*decideQuestion
}

var questionID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,39}$`)

func prepareDecide(payload json.RawMessage) (*prepared, error) {
	var in struct {
		State     json.RawMessage            `json:"state"`
		Questions map[string]*decideQuestion `json:"questions"`
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return nil, invalid("decide() takes { state, questions }.")
	}
	var state map[string]any
	if len(in.State) == 0 || json.Unmarshal(in.State, &state) != nil || state == nil {
		return nil, invalid("decide() state must be a JSON object: the thing being judged.")
	}
	compactState, _ := json.Marshal(state)
	if len(compactState) > MaxDecideStateBytes {
		return nil, invalid(fmt.Sprintf("decide() state is limited to %d KB.", MaxDecideStateBytes>>10))
	}
	if len(in.Questions) == 0 || len(in.Questions) > MaxDecideQuestions {
		return nil, invalid(fmt.Sprintf("decide() takes between 1 and %d questions.", MaxDecideQuestions))
	}
	for id, q := range in.Questions {
		if !questionID.MatchString(id) {
			return nil, invalid("Question names must be identifiers of up to 40 letters, digits and underscores.")
		}
		if q == nil {
			return nil, invalid("Each question must be an object.")
		}
		if strings.TrimSpace(q.Instructions) == "" || len(q.Instructions) > MaxInstructionsBytes {
			return nil, invalid(fmt.Sprintf("Each question needs instructions of up to %d characters.", MaxInstructionsBytes))
		}
		switch q.Type {
		case "bool":
			var c map[string]string
			if json.Unmarshal(q.Criteria, &c) != nil || len(c) != 2 || c["true"] == "" || c["false"] == "" {
				return nil, invalid("A bool question's criteria are { true: \"…\", false: \"…\" }.")
			}
			if len(c["true"]) > MaxCriterionBytes || len(c["false"]) > MaxCriterionBytes {
				return nil, invalid("A criterion is too long.")
			}
		case "choice":
			var c map[string]string
			if json.Unmarshal(q.Criteria, &c) != nil || len(c) < 2 || len(c) > MaxChoiceOptions {
				return nil, invalid(fmt.Sprintf("A choice question's criteria map between 2 and %d options to what each means.", MaxChoiceOptions))
			}
			for label, meaning := range c {
				if label == "" || len(label) > 80 || strings.TrimSpace(meaning) == "" || len(meaning) > MaxCriterionBytes {
					return nil, invalid("Each choice needs a short label and a description.")
				}
				q.options = append(q.options, label)
			}
			sort.Strings(q.options)
		case "score":
			var levels []string
			if json.Unmarshal(q.Criteria, &levels) != nil || len(levels) < 2 || len(levels) > MaxScoreLevels {
				return nil, invalid(fmt.Sprintf("A score question's criteria list between 2 and %d levels, lowest first.", MaxScoreLevels))
			}
			for _, l := range levels {
				if strings.TrimSpace(l) == "" || len(l) > MaxCriterionBytes {
					return nil, invalid("Each score level needs a description.")
				}
			}
			q.levels = len(levels)
		default:
			return nil, invalid("Question type must be \"bool\", \"choice\" or \"score\".")
		}
	}
	return &prepared{decide: &decideRequest{State: compactState, Questions: in.Questions}}, nil
}

func runDecide(ctx context.Context, rt Runtime, job Job, p *prepared, meter func(Outcome), gate func(Job) error) (json.RawMessage, error) {
	req := p.decide
	questions := map[string]any{}
	for id, q := range req.Questions {
		questions[id] = map[string]any{"type": q.Type, "instructions": q.Instructions, "criteria": q.Criteria}
	}
	job.Context, _ = json.Marshal(map[string]any{"state": req.State, "questions": questions})
	res, err := callOnce(ctx, rt, job, meter, gate)
	if err != nil {
		return nil, err
	}
	answers, err := checkAnswers(req, res.Answers)
	if err != nil {
		// No repair: a decision model either answers the questions asked or
		// it failed. The call was billed all the same, and was metered.
		return nil, refuse(CodeProviderError, "The AI service returned an answer that did not fit the questions.")
	}
	return answers, nil
}

// checkAnswers validates and normalizes a classifier's answers: every
// question answered, each in its own shape, probabilities and confidence in
// range, a choice from the offered options and a score on the scale.
func checkAnswers(req *decideRequest, raw json.RawMessage) (json.RawMessage, error) {
	var got map[string]map[string]json.RawMessage
	if json.Unmarshal(raw, &got) != nil {
		return nil, errors.New("answers are not an object")
	}
	// Pointers distinguish missing and null numbers from explicit zeroes.
	inRange := func(n *float64, max float64) bool {
		return n != nil && *n >= 0 && *n <= max
	}
	unit := func(raw json.RawMessage) (float64, bool) {
		var f *float64
		if json.Unmarshal(raw, &f) != nil || !inRange(f, 1) {
			return 0, false
		}
		return *f, true
	}
	out := map[string]any{}
	for id, q := range req.Questions {
		a, ok := got[id]
		if !ok {
			return nil, fmt.Errorf("question %s unanswered", id)
		}
		switch q.Type {
		case "bool":
			p, ok := unit(a["probability"])
			if !ok {
				return nil, fmt.Errorf("question %s: bad probability", id)
			}
			out[id] = map[string]any{"type": "bool", "probability": p}
		case "choice":
			var choice string
			if json.Unmarshal(a["choice"], &choice) != nil {
				return nil, fmt.Errorf("question %s: no choice", id)
			}
			idx := sort.SearchStrings(q.options, choice)
			if idx >= len(q.options) || q.options[idx] != choice {
				return nil, fmt.Errorf("question %s: choice not offered", id)
			}
			conf, ok := unit(a["confidence"])
			if !ok {
				return nil, fmt.Errorf("question %s: bad confidence", id)
			}
			var probs map[string]*float64
			if json.Unmarshal(a["probabilities"], &probs) != nil {
				return nil, fmt.Errorf("question %s: bad probabilities", id)
			}
			clean := map[string]float64{}
			for _, label := range q.options {
				p := probs[label]
				if !inRange(p, 1) {
					return nil, fmt.Errorf("question %s: bad probability", id)
				}
				clean[label] = *p
			}
			out[id] = map[string]any{"type": "choice", "choice": choice, "probabilities": clean, "confidence": conf}
		case "score":
			var score *float64
			if json.Unmarshal(a["score"], &score) != nil || !inRange(score, float64(q.levels-1)) {
				return nil, fmt.Errorf("question %s: score off the scale", id)
			}
			conf, ok := unit(a["confidence"])
			if !ok {
				return nil, fmt.Errorf("question %s: bad confidence", id)
			}
			out[id] = map[string]any{"type": "score", "score": *score, "confidence": conf}
		}
	}
	return json.Marshal(out)
}
