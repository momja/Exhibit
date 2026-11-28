package inference

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
)

// The shared image corpus (Exh-v81w). testdata/imagemeta holds image files,
// what the metadata strip must make of each (cases.json), and complete()
// payloads with the result or refusal the server must give (payloads.json).
// This test and web/gallery/image-metadata.test.mjs both read it, so the Go
// and the host-page implementations cannot drift apart.
//
// The image files are generated: go test ./internal/inference -run
// TestImageMetaCorpus -update-imagemeta-corpus rewrites them from the specs
// below and records this implementation's output, which is then reviewed in
// the diff. payloads.json is written by hand.

var updateImageMetaCorpus = flag.Bool("update-imagemeta-corpus", false, "rewrite testdata/imagemeta from the specs in imagemeta_corpus_test.go")

const corpusDir = "testdata/imagemeta"

type corpusCase struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Input    string `json:"input"`
	Output   string `json:"output,omitempty"`
	Refused  string `json:"refused,omitempty"` // "malformed" or "animated"
}

// Bytes no stripped image may contain: the markers the fixtures plant, and
// the signatures of every container the strip removes.
var forbiddenInOutput = []string{
	metaMarker, "Exif\x00\x00", "http://ns.adobe.com/xap", "<x:xmpmeta", "ICC_PROFILE", "JFIF\x00", "JFXX\x00",
	"MPF\x00", "Photoshop 3.0", "eXIf", "tEXt", "iTXt", "zTXt", "tIME", "iCCP", "ICCP", "EXIF", "XMP ",
}

func TestImageMetaCorpus(t *testing.T) {
	if *updateImageMetaCorpus {
		writeImageMetaCorpus(t)
	}
	var cases []corpusCase
	readCorpusJSON(t, "cases.json", &cases)
	if len(cases) < 20 {
		t.Fatalf("only %d corpus cases", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			in := readCorpusFile(t, tc.Input)
			got, err := stripImageMetadata(tc.MimeType, in)
			switch tc.Refused {
			case "malformed":
				if !errors.Is(err, errMalformedImage) {
					t.Fatalf("err = %v; want malformed", err)
				}
				return
			case "animated":
				if !errors.Is(err, errAnimatedImage) {
					t.Fatalf("err = %v; want animated", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := readCorpusFile(t, tc.Output); !bytes.Equal(got, want) {
				t.Fatalf("stripped to %d bytes; the corpus has %d", len(got), len(want))
			}
			for _, s := range forbiddenInOutput {
				if bytes.Contains(got, []byte(s)) {
					t.Fatalf("the output still carries %q", s)
				}
			}
			if again, err := stripImageMetadata(tc.MimeType, got); err != nil || !bytes.Equal(again, got) {
				t.Fatalf("stripping twice changed the file (%v)", err)
			}
			if len(got) > len(in) {
				t.Fatal("the strip made the file larger")
			}
			assertSamePixels(t, tc.MimeType, in, got)
		})
	}
}

// assertSamePixels decodes both files where Go can, and requires the strip
// to leave the picture exactly as it was.
func assertSamePixels(t *testing.T, mimeType string, in, out []byte) {
	t.Helper()
	decode := map[string]func([]byte) (image.Image, error){
		"image/jpeg": func(b []byte) (image.Image, error) { return jpeg.Decode(bytes.NewReader(b)) },
		"image/png":  func(b []byte) (image.Image, error) { return png.Decode(bytes.NewReader(b)) },
	}[mimeType]
	if decode == nil {
		return
	}
	before, err := decode(in)
	if err != nil {
		return // a synthetic structure, not a picture
	}
	after, err := decode(out)
	if err != nil {
		t.Fatalf("the input decodes and the output does not: %v", err)
	}
	if before.Bounds() != after.Bounds() {
		t.Fatal("the bounds changed")
	}
	for y := before.Bounds().Min.Y; y < before.Bounds().Max.Y; y++ {
		for x := before.Bounds().Min.X; x < before.Bounds().Max.X; x++ {
			r1, g1, b1, a1 := before.At(x, y).RGBA()
			r2, g2, b2, a2 := after.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				t.Fatalf("pixel %d,%d changed", x, y)
			}
		}
	}
}

// --- payloads ---------------------------------------------------------------

type payloadCase struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
	// Error is the refusal's exact message; Images, when there is none, is
	// what each image part must carry, in order.
	Error  string `json:"error,omitempty"`
	Images []struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	} `json:"images,omitempty"`
}

func TestImageMetaPayloadCorpus(t *testing.T) {
	var cases []payloadCase
	readCorpusJSON(t, "payloads.json", &cases)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			payload := expandCorpusPlaceholders(t, string(tc.Payload))
			p, err := prepareComplete(json.RawMessage(payload))
			if tc.Error != "" {
				var e *Error
				if !errors.As(err, &e) || e.Code != CodeInvalidRequest || e.Message != tc.Error {
					t.Fatalf("err = %v; want %q", err, tc.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var images []*imagePart
			for _, m := range p.complete.Messages {
				for _, part := range m.Parts {
					if part.Image != nil {
						images = append(images, part.Image)
					}
				}
			}
			if len(images) != len(tc.Images) {
				t.Fatalf("%d images; the corpus has %d", len(images), len(tc.Images))
			}
			for i, want := range tc.Images {
				if images[i].MimeType != want.MimeType || images[i].Data != expandCorpusPlaceholders(t, want.Data) {
					t.Fatalf("image %d differs from the corpus", i)
				}
			}
		})
	}
}

// The placeholders a payload uses for image data, expanded alike by the
// Node test: $b64(file) is a corpus file in base64, and $png(n) a
// well-formed PNG of exactly n bytes (IHDR, one IDAT of zeros, IEND; CRCs
// zero, which nothing checks).
var corpusPlaceholder = regexp.MustCompile(`\$(b64|png)\(([^)]*)\)`)

func expandCorpusPlaceholders(t *testing.T, s string) string {
	t.Helper()
	return corpusPlaceholder.ReplaceAllStringFunc(s, func(m string) string {
		parts := corpusPlaceholder.FindStringSubmatch(m)
		if parts[1] == "b64" {
			return base64.StdEncoding.EncodeToString(readCorpusFile(t, parts[2]))
		}
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(corpusPNG(n))
	})
}

func corpusPNG(n int) []byte {
	chunk := func(out []byte, kind string, size int) []byte {
		out = binary.BigEndian.AppendUint32(out, uint32(size))
		out = append(out, kind...)
		if kind == "IHDR" {
			out = append(out, 0, 0, 0, 1, 0, 0, 0, 1, 8, 0, 0, 0, 0)
		} else {
			out = append(out, make([]byte, size)...)
		}
		return append(out, 0, 0, 0, 0)
	}
	out := chunk([]byte(pngSignature), "IHDR", 13)
	out = chunk(out, "IDAT", n-8-25-12-12)
	return chunk(out, "IEND", 0)
}

func readCorpusFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(corpusDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func readCorpusJSON(t *testing.T, name string, into any) {
	t.Helper()
	if err := json.Unmarshal(readCorpusFile(t, name), into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// --- the generator ----------------------------------------------------------

type corpusSpec struct {
	name, mimeType string
	build          func(t *testing.T) []byte
	refused        string
}

var corpusExt = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

func writeImageMetaCorpus(t *testing.T) {
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(corpusDir, "*.*.*"))
	for _, f := range old {
		_ = os.Remove(f)
	}
	var cases []corpusCase
	for _, spec := range corpusSpecs() {
		in := spec.build(t)
		c := corpusCase{Name: spec.name, MimeType: spec.mimeType, Input: spec.name + ".in" + corpusExt[spec.mimeType], Refused: spec.refused}
		writeCorpusFile(t, c.Input, in)
		out, err := stripImageMetadata(spec.mimeType, in)
		switch {
		case spec.refused == "" && err != nil:
			t.Fatalf("%s: %v", spec.name, err)
		case spec.refused != "" && err == nil:
			t.Fatalf("%s: stripped; want %s", spec.name, spec.refused)
		case spec.refused == "":
			c.Output = spec.name + ".out" + corpusExt[spec.mimeType]
			writeCorpusFile(t, c.Output, out)
		}
		cases = append(cases, c)
	}
	b, _ := json.MarshalIndent(cases, "", "  ")
	writeCorpusFile(t, "cases.json", append(b, '\n'))
}

func writeCorpusFile(t *testing.T, name string, b []byte) {
	if err := os.WriteFile(filepath.Join(corpusDir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// jpegAfterSOI inserts segments after a JPEG's SOI.
func jpegAfterSOI(jpg []byte, segments ...[]byte) []byte {
	return cat(jpg[:2], cat(segments...), jpg[2:])
}

// jpegSegments splits a JPEG into SOI, its marker segments with the scan
// data that follows each SOS, and EOI.
func jpegSegments(t *testing.T, jpg []byte) [][]byte {
	t.Helper()
	var out [][]byte
	i := 2
	for i < len(jpg)-2 {
		start := i
		marker := jpg[i+1]
		i += 2 + int(binary.BigEndian.Uint16(jpg[i+2:]))
		if marker == 0xDA {
			for jpg[i] != 0xFF || jpg[i+1] == 0x00 || (jpg[i+1] >= 0xD0 && jpg[i+1] <= 0xD7) {
				i++
			}
		}
		out = append(out, jpg[start:i])
	}
	return out
}

func goTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "src", "image", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func corpusSpecs() []corpusSpec {
	const icc = "ICC_PROFILE\x00\x01\x01" + "\x00\x00\x02\x0cappl\x02\x10\x00\x00mntrRGB XYZ \x07\xea\x00\x0b\x00\x1c" + metaMarker
	seg := jpegSegment
	exif := seg(0xE1, "Exif\x00\x00MM\x00*GPSLatitude "+metaMarker)
	xmp := seg(0xE1, "http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>"+metaMarker+"</x:xmpmeta>")
	jfif := seg(0xE0, "JFIF\x00\x01\x02\x00\x00\x01\x00\x01\x00\x00")
	jfxx := seg(0xE0, "JFXX\x00\x13thumbnail "+metaMarker)
	adobe := seg(0xEE, "Adobe\x00d\x00\x00\x00\x00\x01")
	vp8 := webpChunk("VP8 ", make([]byte, 14))
	vp8l := webpChunk("VP8L", []byte{0x2F, 0, 0, 0, 0x10, 0x07, 0x10, 0x11, 0x11, 0x88, 0x88, 0xFE, 0x07})
	alph := webpChunk("ALPH", []byte{0, 1, 2}) // odd, padded
	png := func(t *testing.T, chunks ...[]byte) []byte {
		plain := plainPNG(t)
		return cat(plain[:8+25], cat(chunks...), plain[8+25:])
	}
	pngChunk := func(kind, data string) []byte { return appendPNGChunk(nil, kind, []byte(data)) }
	minimalPNG := func(chunks ...[]byte) []byte {
		return cat([]byte(pngSignature), pngChunk("IHDR", "\x00\x00\x00\x01\x00\x00\x00\x01\x08\x00\x00\x00\x00"), cat(chunks...))
	}
	idat := pngChunk("IDAT", "x\x9cc\x00\x01\x00\x00\x05\x00\x01")
	iend := pngChunk("IEND", "")
	return []corpusSpec{
		// JPEG
		{name: "jpeg-plain", mimeType: "image/jpeg", build: plainJPEG},
		{name: "jpeg-all-metadata", mimeType: "image/jpeg", build: func(t *testing.T) []byte {
			return cat(jpegAfterSOI(plainJPEG(t), jfif, jfxx, exif, xmp, seg(0xE2, "MPF\x00"+metaMarker), seg(0xE2, icc),
				seg(0xED, "Photoshop 3.0\x00"+metaMarker), adobe, seg(0xEF, metaMarker), seg(0xFE, "camera comment "+metaMarker)),
				[]byte("motion-photo-trailer "+metaMarker))
		}},
		{name: "jpeg-fill-before-marker", mimeType: "image/jpeg", build: func(t *testing.T) []byte {
			s := jpegSegments(t, plainJPEG(t))
			return cat([]byte{0xFF, 0xD8, 0xFF, 0xFF}, s[0], []byte{0xFF, 0xFF, 0xFF}, exif, cat(s[1:]...), []byte{0xFF, 0xD9})
		}},
		{name: "jpeg-progressive", mimeType: "image/jpeg", build: func(t *testing.T) []byte {
			// A real progressive file, with a comment between its scans.
			s := jpegSegments(t, goTestdata(t, "video-001.q50.420.progressive.jpeg"))
			var out [][]byte
			scans := 0
			for _, part := range s {
				if part[1] == 0xDA {
					if scans++; scans == 2 {
						out = append(out, seg(0xFE, metaMarker))
					}
				}
				out = append(out, part)
			}
			if scans < 3 {
				t.Fatalf("only %d scans", scans)
			}
			return cat([]byte{0xFF, 0xD8}, exif, cat(out...), []byte{0xFF, 0xD9})
		}},
		{name: "jpeg-restart-markers", mimeType: "image/jpeg", build: func(t *testing.T) []byte {
			return jpegAfterSOI(goTestdata(t, "video-001.restart2.jpeg"), exif)
		}},
		{name: "jpeg-adobe-cmyk", mimeType: "image/jpeg", build: func(t *testing.T) []byte {
			return jpegAfterSOI(goTestdata(t, "video-001.cmyk.jpeg"), xmp)
		}},
		{name: "jpeg-trailer", mimeType: "image/jpeg", build: func(t *testing.T) []byte {
			return cat(plainJPEG(t), []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x10}, []byte(metaMarker))
		}},
		{name: "jpeg-truncated-segment", mimeType: "image/jpeg", refused: "malformed", build: func(t *testing.T) []byte {
			return jpegAfterSOI(plainJPEG(t), exif)[:30]
		}},
		{name: "jpeg-truncated-scan", mimeType: "image/jpeg", refused: "malformed", build: func(t *testing.T) []byte {
			p := plainJPEG(t)
			return p[:len(p)-40]
		}},
		{name: "jpeg-no-eoi", mimeType: "image/jpeg", refused: "malformed", build: func(t *testing.T) []byte {
			p := plainJPEG(t)
			return p[:len(p)-2]
		}},
		{name: "jpeg-no-frame", mimeType: "image/jpeg", refused: "malformed", build: func(t *testing.T) []byte {
			var keep [][]byte
			for _, s := range jpegSegments(t, plainJPEG(t)) {
				if !isJPEGFrame(s[1]) {
					keep = append(keep, s)
				}
			}
			return cat([]byte{0xFF, 0xD8}, cat(keep...), []byte{0xFF, 0xD9})
		}},
		{name: "jpeg-no-scan", mimeType: "image/jpeg", refused: "malformed", build: func(t *testing.T) []byte {
			var keep [][]byte
			for _, s := range jpegSegments(t, plainJPEG(t)) {
				if s[1] != 0xDA {
					keep = append(keep, s)
				}
			}
			return cat([]byte{0xFF, 0xD8}, cat(keep...), []byte{0xFF, 0xD9})
		}},
		{name: "jpeg-length-past-end", mimeType: "image/jpeg", refused: "malformed", build: func(*testing.T) []byte {
			return append([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x7F, 0xFF}, metaMarker...)
		}},
		{name: "jpeg-junk-between-segments", mimeType: "image/jpeg", refused: "malformed", build: func(t *testing.T) []byte {
			return cat([]byte{0xFF, 0xD8}, seg(0xFE, "x"), []byte{0x00}, plainJPEG(t)[2:])
		}},
		// PNG
		{name: "png-plain", mimeType: "image/png", build: plainPNG},
		{name: "png-all-metadata", mimeType: "image/png", build: func(t *testing.T) []byte {
			return cat(png(t,
				pngChunk("iCCP", "Display P3\x00\x00x\x9c"+metaMarker),
				pngChunk("eXIf", "MM\x00*"+metaMarker),
				pngChunk("tEXt", "Comment\x00"+metaMarker),
				pngChunk("iTXt", "XML:com.adobe.xmp\x00\x00\x00\x00\x00"+metaMarker),
				pngChunk("zTXt", "Raw profile type exif\x00\x00x\x9c"+metaMarker),
				pngChunk("tIME", "\x07\xea\x0b\x1c\x0c\x00\x00"),
				pngChunk("prVt", metaMarker),
				pngChunk("gAMA", "\x00\x00\xb1\x8f"),
				pngChunk("pHYs", "\x00\x00\x0b\x13\x00\x00\x0b\x13\x01"),
			), []byte("trailer "+metaMarker))
		}},
		{name: "png-colour-chunks-kept", mimeType: "image/png", build: func(t *testing.T) []byte {
			return png(t,
				pngChunk("cICP", "\x09\x10\x00\x01"),
				pngChunk("mDCV", string(make([]byte, 24))),
				pngChunk("cLLI", "\x00\x00\x03\xe8\x00\x00\x01\x90"),
				pngChunk("sRGB", "\x00"),
				pngChunk("cHRM", string(make([]byte, 32))),
				pngChunk("sBIT", "\x08\x08\x08\x08"),
				pngChunk("bKGD", "\x00\x00\x00\x00\x00\x00"),
				pngChunk("tEXt", "Software\x00"+metaMarker),
			)
		}},
		{name: "png-apng", mimeType: "image/png", refused: "animated", build: func(t *testing.T) []byte {
			return png(t, pngChunk("acTL", "\x00\x00\x00\x02\x00\x00\x00\x00"), pngChunk("fcTL", string(make([]byte, 26))))
		}},
		{name: "png-apng-fdat-only", mimeType: "image/png", refused: "animated", build: func(*testing.T) []byte {
			return minimalPNG(idat, pngChunk("fdAT", "\x00\x00\x00\x01"+metaMarker), iend)
		}},
		{name: "png-iend-not-empty", mimeType: "image/png", refused: "malformed", build: func(*testing.T) []byte {
			return minimalPNG(idat, pngChunk("IEND", metaMarker))
		}},
		{name: "png-no-idat", mimeType: "image/png", refused: "malformed", build: func(*testing.T) []byte {
			return minimalPNG(pngChunk("tEXt", "Comment\x00"+metaMarker), iend)
		}},
		{name: "png-ihdr-wrong-length", mimeType: "image/png", refused: "malformed", build: func(*testing.T) []byte {
			return cat([]byte(pngSignature), pngChunk("IHDR", "\x00\x00\x00\x01\x00\x00\x00\x01\x08\x00\x00\x00\x00\x00"), idat, iend)
		}},
		{name: "png-ihdr-not-first", mimeType: "image/png", refused: "malformed", build: func(*testing.T) []byte {
			return cat([]byte(pngSignature), pngChunk("tEXt", metaMarker), pngChunk("IHDR", string(make([]byte, 13))), idat, iend)
		}},
		{name: "png-no-iend", mimeType: "image/png", refused: "malformed", build: func(t *testing.T) []byte {
			p := plainPNG(t)
			return p[:len(p)-12]
		}},
		{name: "png-truncated", mimeType: "image/png", refused: "malformed", build: func(t *testing.T) []byte {
			p := png(t, pngChunk("eXIf", "MM\x00*"+metaMarker))
			return p[:len(p)/2]
		}},
		{name: "png-length-past-end", mimeType: "image/png", refused: "malformed", build: func(*testing.T) []byte {
			b := minimalPNG(idat, iend)
			binary.BigEndian.PutUint32(b[8+25:], 1<<30)
			return b
		}},
		// WebP
		{name: "webp-simple-lossy", mimeType: "image/webp", build: func(*testing.T) []byte { return riff(vp8) }},
		{name: "webp-simple-lossless", mimeType: "image/webp", build: func(*testing.T) []byte { return riff(vp8l) }},
		{name: "webp-simple-trailer", mimeType: "image/webp", build: func(*testing.T) []byte {
			return append(riff(vp8), "trailer "+metaMarker...)
		}},
		{name: "webp-extended-metadata", mimeType: "image/webp", build: func(*testing.T) []byte {
			return riff(vp8x(webpFlagICC|0x10|webpFlagEXIF|webpFlagXMP), webpChunk("ICCP", []byte(icc)), alph, vp8,
				webpChunk("EXIF", []byte("MM\x00*"+metaMarker+"!")), // odd, padded
				webpChunk("XMP ", []byte("<x:xmpmeta>"+metaMarker+"</x:xmpmeta>")),
				webpChunk("ZZZZ", []byte(metaMarker)))
		}},
		{name: "webp-odd-chunk-padded", mimeType: "image/webp", build: func(*testing.T) []byte {
			return riff(vp8x(0x10), webpChunk("ZZZ1", []byte(metaMarker+"!!")), alph, vp8l)
		}},
		{name: "webp-odd-chunk-unpadded", mimeType: "image/webp", refused: "malformed", build: func(*testing.T) []byte {
			b := riff(vp8, webpChunk("EXIF", []byte(metaMarker)))
			b = b[:len(b)-1] // the last chunk's pad byte
			binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
			return b
		}},
		{name: "webp-anim", mimeType: "image/webp", refused: "animated", build: func(*testing.T) []byte {
			return riff(vp8x(0x02), webpChunk("ANIM", make([]byte, 6)), webpChunk("ANMF", cat(make([]byte, 16), webpChunk("EXIF", []byte(metaMarker)), vp8)))
		}},
		{name: "webp-anim-flag-only", mimeType: "image/webp", refused: "animated", build: func(*testing.T) []byte {
			return riff(vp8x(0x02), vp8)
		}},
		{name: "webp-anmf-without-flag", mimeType: "image/webp", refused: "animated", build: func(*testing.T) []byte {
			return riff(vp8x(0), webpChunk("ANMF", cat(make([]byte, 16), vp8)))
		}},
		{name: "webp-no-image", mimeType: "image/webp", refused: "malformed", build: func(*testing.T) []byte {
			return riff(vp8x(webpFlagEXIF), webpChunk("EXIF", []byte(metaMarker)))
		}},
		{name: "webp-short-vp8x", mimeType: "image/webp", refused: "malformed", build: func(*testing.T) []byte {
			return riff(webpChunk("VP8X", []byte{webpFlagEXIF}), vp8)
		}},
		{name: "webp-riff-size-past-end", mimeType: "image/webp", refused: "malformed", build: func(*testing.T) []byte {
			b := riff(vp8)
			binary.LittleEndian.PutUint32(b[4:], uint32(len(b)))
			return b
		}},
		{name: "webp-chunk-past-end", mimeType: "image/webp", refused: "malformed", build: func(*testing.T) []byte {
			b := riff(vp8)
			binary.LittleEndian.PutUint32(b[16:], 1000)
			return b
		}},
		{name: "webp-truncated", mimeType: "image/webp", refused: "malformed", build: func(*testing.T) []byte {
			b := riff(vp8x(webpFlagEXIF), vp8, webpChunk("EXIF", []byte(metaMarker)))
			return b[:len(b)-5]
		}},
	}
}
