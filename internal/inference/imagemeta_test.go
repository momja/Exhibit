package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// Metadata is stripped from every image before it reaches a provider
// (Exh-v81w). The strip rules themselves are held to the shared corpus
// (imagemeta_corpus_test.go); these tests are about the service around them.
// Fixtures plant a marker string in every metadata container, so "gone" is a
// byte search.

const metaMarker = "GPS-MARKER-v81w"

func noisyImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 37)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

func plainJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, noisyImage(), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegSegment(marker byte, payload string) []byte {
	seg := []byte{0xFF, marker}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	return append(seg, payload...)
}

func plainPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, noisyImage()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func webpChunk(kind string, data []byte) []byte {
	out := append([]byte(kind), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
	out = append(out, data...)
	if len(data)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func riff(chunks ...[]byte) []byte {
	body := bytes.Join(chunks, nil)
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(body)))...)
	out = append(out, "WEBP"...)
	return append(out, body...)
}

// vp8x is an extended-format header with the given flags and a 1x1 canvas.
func vp8x(flags byte) []byte { return webpChunk("VP8X", []byte{flags, 0, 0, 0, 0, 0, 0, 0, 0, 0}) }

func corpusCases(t *testing.T) []corpusCase {
	t.Helper()
	var cases []corpusCase
	readCorpusJSON(t, "cases.json", &cases)
	return cases
}

// A refused file never reaches the provider or the ledger, and the refusal
// never quotes the data.
func TestRefusedImagesNeverReachTheProvider(t *testing.T) {
	for _, tc := range corpusCases(t) {
		if tc.Refused == "" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			s, rt, led := newTestService(t, nil)
			data := base64.StdEncoding.EncodeToString(readCorpusFile(t, tc.Input))
			_, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(imgPart(tc.MimeType, data))))
			want := msgImageMalformed(tc.MimeType)
			if tc.Refused == "animated" {
				want = msgImageAnimated
			}
			var e *Error
			if code(err) != CodeInvalidRequest || !errors.As(err, &e) || e.Message != want {
				t.Fatalf("err = %v; want %q", err, want)
			}
			if strings.Contains(err.Error(), data[:min(16, len(data))]) || strings.Contains(err.Error(), metaMarker) {
				t.Fatal("the message quoted image data")
			}
			if rt.calls() != 0 || len(led.rows) != 0 {
				t.Fatal("a refused image reached the provider or the ledger")
			}
		})
	}
}

// The provider receives the stripped bytes, whichever way the data arrived.
func TestTheProviderReceivesStrippedImages(t *testing.T) {
	byName := map[string]corpusCase{}
	for _, c := range corpusCases(t) {
		byName[c.Name] = c
	}
	jpg, pngIn, webp := byName["jpeg-all-metadata"], byName["png-all-metadata"], byName["webp-extended-metadata"]
	b64 := func(name string) string { return base64.StdEncoding.EncodeToString(readCorpusFile(t, name)) }
	s, rt, _ := newTestService(t, nil)
	call := imageCall(t, owner, imageMessage(
		textPart("What is this?"),
		imgPart("image/jpeg", b64(jpg.Input)),
		map[string]any{"type": "image", "data": "data:image/png;base64," + b64(pngIn.Input)},
		imgPart(" IMAGE/WEBP ", b64(webp.Input)),
	))
	if _, err := s.Run(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]string
	if err := json.Unmarshal(decodeContext(t, rt.jobs[0].Context).Messages[0].Content, &blocks); err != nil {
		t.Fatal(err)
	}
	for i, c := range []corpusCase{jpg, pngIn, webp} {
		if blocks[i+1]["data"] != b64(c.Output) || blocks[i+1]["mimeType"] != c.MimeType {
			t.Fatalf("image %d reached the provider unstripped", i)
		}
	}
	if strings.Contains(string(rt.jobs[0].Context), base64.StdEncoding.EncodeToString([]byte(metaMarker))[:12]) {
		t.Fatal("the job context still carries metadata")
	}
}

// The size bound counts the bytes as received: an image over it is refused
// even if most of it is metadata the strip would have removed.
func TestTheSizeBoundCountsBytesBeforeStripping(t *testing.T) {
	plain := plainPNG(t)
	over := append([]byte{}, plain[:8+25]...)
	over = appendPNGChunk(over, "tEXt", append([]byte("Comment\x00"), make([]byte, MaxImageBytes)...))
	over = append(over, plain[8+25:]...)
	s, rt, _ := newTestService(t, nil)
	_, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(imgPart("image/png", base64.StdEncoding.EncodeToString(over)))))
	if code(err) != CodeInvalidRequest || !strings.Contains(err.Error(), "2 MB") || rt.calls() != 0 {
		t.Fatalf("err = %v", err)
	}
}

// Keys are read as spelled. encoding/json alone would read "Messages" or
// "DATA" as the field, and the host page, which reads keys exactly, would
// have sent that image as it came (Exh-v81w).
func TestCompleteRefusesKeysNotSpelledExactly(t *testing.T) {
	jpg := base64.StdEncoding.EncodeToString(plainJPEG(t))
	part := `{"type":"image","mimeType":"image/jpeg","data":"` + jpg + `"}`
	for _, tc := range []struct{ name, payload, want string }{
		{"payload key", `{"Messages":[{"role":"user","content":[` + part + `]}]}`, msgPayloadShape},
		{"payload key upper", `{"MESSAGES":[{"role":"user","content":"Hi"}]}`, msgPayloadShape},
		{"payload key folded", `{"meſſages":[{"role":"user","content":"Hi"}]}`, msgPayloadShape},
		{"payload maxTokens", `{"messages":[{"role":"user","content":"Hi"}],"MaxTokens":5}`, msgPayloadShape},
		{"payload duplicate in case", `{"messages":[{"role":"user","content":"Hi"}],"Messages":[{"role":"user","content":[` + part + `]}]}`, msgPayloadShape},
		{"payload duplicate exact", `{"messages":[{"role":"user","content":"Hi"}],"messages":[{"role":"user","content":"Hi"}]}`, msgPayloadShape},
		{"message key", `{"messages":[{"Role":"user","content":"Hi"}]}`, msgMessageShape},
		{"message content key", `{"messages":[{"role":"user","Content":[` + part + `]}]}`, msgMessageShape},
		{"message duplicate", `{"messages":[{"role":"user","content":"Hi","CONTENT":[` + part + `]}]}`, msgMessageShape},
		{"part type", `{"messages":[{"role":"user","content":[{"Type":"image","mimeType":"image/jpeg","data":"` + jpg + `"}]}]}`, msgContentShape},
		{"part data", `{"messages":[{"role":"user","content":[{"type":"image","mimeType":"image/jpeg","Data":"` + jpg + `"}]}]}`, msgContentShape},
		{"part mimeType", `{"messages":[{"role":"user","content":[{"type":"image","MimeType":"image/jpeg","data":"` + jpg + `"}]}]}`, msgContentShape},
		{"part mimetype lower", `{"messages":[{"role":"user","content":[{"type":"image","mimetype":"image/jpeg","data":"` + jpg + `"}]}]}`, msgContentShape},
		{"part text", `{"messages":[{"role":"user","content":[{"type":"text","Text":"Hi"}]}]}`, msgContentShape},
		{"part duplicate", `{"messages":[{"role":"user","content":[{"type":"text","text":"Hi","data":null,"DATA":"` + jpg + `"}]}]}`, msgContentShape},
		{"part unknown", `{"messages":[{"role":"user","content":[{"type":"image","mimeType":"image/jpeg","data":"` + jpg + `","url":"x"}]}]}`, msgContentShape},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, rt, _ := newTestService(t, nil)
			_, err := s.Run(context.Background(), Call{Kind: KindComplete, ArtifactID: "art-1", ViewerID: owner, Payload: json.RawMessage(tc.payload)})
			var e *Error
			if !errors.As(err, &e) || e.Code != CodeInvalidRequest || e.Message != tc.want {
				t.Fatalf("err = %v; want %q", err, tc.want)
			}
			if rt.calls() != 0 {
				t.Fatal("the call reached the provider")
			}
		})
	}
	// The exact names still work, null where a field may be absent included.
	for _, payload := range []string{
		`{"messages":[{"role":"user","content":[{"type":"text","text":"Hi","data":null,"mimeType":null}]}],"system":null,"schema":null,"maxTokens":null,"prompt":null}`,
		`{"prompt":"Hi","messages":null}`,
	} {
		if _, err := prepareComplete(json.RawMessage(payload)); err != nil {
			t.Fatalf("%s: %v", payload, err)
		}
	}
}
