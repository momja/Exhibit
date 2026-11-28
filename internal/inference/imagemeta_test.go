package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

// Metadata is stripped from every image before it reaches a provider
// (Exh-v81w). The fixtures carry real metadata containers whose payloads hold
// a marker string, so "gone" is a byte search.

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

const iccSegmentPayload = "ICC_PROFILE\x00\x01\x01fake-profile-bytes"

// metadataJPEG is plain with an EXIF (carrying GPS-like data), an XMP packet,
// an MPF index, a Photoshop block and a comment after SOI, an ICC profile
// that is kept, and a trailer after EOI. It returns the file and what the
// strip must produce.
func metadataJPEG(t *testing.T) (withMeta, want []byte) {
	t.Helper()
	plain := plainJPEG(t)
	head := [][]byte{
		jpegSegment(0xE1, "Exif\x00\x00MM\x00*GPSLatitude "+metaMarker),
		jpegSegment(0xE1, "http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>"+metaMarker+"</x:xmpmeta>"),
		jpegSegment(0xE2, "MPF\x00"+metaMarker),
		jpegSegment(0xE2, iccSegmentPayload),
		jpegSegment(0xED, "Photoshop 3.0\x00"+metaMarker),
		jpegSegment(0xFE, "camera comment "+metaMarker),
	}
	withMeta = append([]byte{}, plain[:2]...)
	withMeta = append(withMeta, bytes.Join(head, nil)...)
	withMeta = append(withMeta, plain[2:]...)
	withMeta = append(withMeta, "motion-photo-trailer "+metaMarker...)
	want = append([]byte{}, plain[:2]...)
	want = append(want, jpegSegment(0xE2, iccSegmentPayload)...)
	want = append(want, plain[2:]...)
	return withMeta, want
}

func plainPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, noisyImage()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// metadataPNG inserts rendering chunks, which are kept, and descriptive ones,
// which are not, after IHDR, plus a trailer after IEND.
func metadataPNG(t *testing.T) (withMeta, want []byte) {
	t.Helper()
	plain := plainPNG(t)
	ihdrEnd := 8 + 25
	var kept, dropped []byte
	kept = appendPNGChunk(kept, "gAMA", []byte{0, 0, 0xB1, 0x8F})
	kept = appendPNGChunk(kept, "pHYs", []byte{0, 0, 0x0B, 0x13, 0, 0, 0x0B, 0x13, 1})
	dropped = appendPNGChunk(dropped, "eXIf", []byte("MM\x00*"+metaMarker))
	dropped = appendPNGChunk(dropped, "tEXt", []byte("Comment\x00"+metaMarker))
	dropped = appendPNGChunk(dropped, "iTXt", []byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00"+metaMarker))
	dropped = appendPNGChunk(dropped, "zTXt", []byte("Raw profile type exif\x00\x00x\x9c"+metaMarker))
	dropped = appendPNGChunk(dropped, "tIME", []byte{0x07, 0xEA, 11, 28, 12, 0, 0})
	dropped = appendPNGChunk(dropped, "prVt", []byte(metaMarker)) // unknown ancillary
	withMeta = append([]byte{}, plain[:ihdrEnd]...)
	withMeta = append(withMeta, dropped...)
	withMeta = append(withMeta, kept...)
	withMeta = append(withMeta, plain[ihdrEnd:]...)
	withMeta = append(withMeta, "trailer "+metaMarker...)
	want = append([]byte{}, plain[:ihdrEnd]...)
	want = append(want, kept...)
	want = append(want, plain[ihdrEnd:]...)
	return withMeta, want
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

const (
	webpICC  = 0x20
	webpEXIF = 0x08
	webpXMP  = 0x04
)

func metadataWebP() (withMeta, want []byte) {
	icc := webpChunk("ICCP", []byte("fake-profile"))
	vp8 := webpChunk("VP8 ", make([]byte, 14))
	withMeta = riff(vp8x(webpICC|webpEXIF|webpXMP), icc, vp8,
		webpChunk("EXIF", []byte("MM\x00*"+metaMarker+"!")), // odd length, padded
		webpChunk("XMP ", []byte("<x:xmpmeta>"+metaMarker+"</x:xmpmeta>")),
		webpChunk("ZZZZ", []byte(metaMarker))) // unknown
	withMeta = append(withMeta, "trailer "+metaMarker...)
	return withMeta, riff(vp8x(webpICC), icc, vp8)
}

func TestStripImageMetadata(t *testing.T) {
	jpg, jpgWant := metadataJPEG(t)
	pngIn, pngWant := metadataPNG(t)
	webp, webpWant := metadataWebP()
	for _, tc := range []struct {
		mimeType string
		in, want []byte
	}{
		{"image/jpeg", jpg, jpgWant},
		{"image/png", pngIn, pngWant},
		{"image/webp", webp, webpWant},
	} {
		t.Run(tc.mimeType, func(t *testing.T) {
			got, err := stripImageMetadata(tc.mimeType, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(got, []byte(metaMarker)) {
				t.Fatal("metadata survived the strip")
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("stripped %d bytes to %d; want %d", len(tc.in), len(got), len(tc.want))
			}
			if http.DetectContentType(got) != tc.mimeType {
				t.Fatalf("the stripped file sniffs as %s", http.DetectContentType(got))
			}
			// A file with nothing to strip comes back as it was.
			again, err := stripImageMetadata(tc.mimeType, got)
			if err != nil || !bytes.Equal(again, got) {
				t.Fatalf("stripping twice changed the file (%v)", err)
			}
		})
	}
	// Pixels decode exactly as before.
	got, _ := stripImageMetadata("image/jpeg", jpg)
	if _, err := jpeg.Decode(bytes.NewReader(got)); err != nil {
		t.Fatalf("stripped JPEG: %v", err)
	}
	got, _ = stripImageMetadata("image/png", pngIn)
	decoded, err := png.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatalf("stripped PNG: %v", err)
	}
	want := noisyImage()
	for y := range 16 {
		for x := range 16 {
			r1, g1, b1, _ := decoded.At(x, y).RGBA()
			r2, g2, b2, _ := color.RGBAModel.Convert(want.At(x, y)).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				t.Fatalf("pixel %d,%d changed", x, y)
			}
		}
	}
	// WebP: the flags and the RIFF size are rewritten.
	got, _ = stripImageMetadata("image/webp", webp)
	if got[20] != webpICC {
		t.Fatalf("VP8X flags = %#x; want only ICC", got[20])
	}
	if int(binary.LittleEndian.Uint32(got[4:])) != len(got)-8 {
		t.Fatal("RIFF size does not match the file")
	}
}

// A progressive file has several scans, with tables between them; the walk
// resumes after each scan and keeps applying the rules.
func TestStripJPEGWalksEveryScan(t *testing.T) {
	plain := plainJPEG(t)
	eoi := len(plain) - 2
	second := append(jpegSegment(0xFE, metaMarker), jpegSegment(0xDA, "\x01\x01\x00\x00\x3f\x00")...)
	second = append(second, 0x12, 0xFF, 0x00, 0x34, 0xFF, 0xD0, 0x56, 0xFF, 0xFF)
	in := append(append(append([]byte{}, plain[:eoi]...), second...), 0xFF, 0xD9)
	got, err := stripImageMetadata("image/jpeg", in)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(metaMarker)) || !bytes.HasSuffix(got, []byte{0x12, 0xFF, 0x00, 0x34, 0xFF, 0xD0, 0x56, 0xFF, 0xFF, 0xFF, 0xD9}) {
		t.Fatalf("second scan: % x", got[eoi:])
	}
}

func TestMalformedImagesAreRefused(t *testing.T) {
	jpg, _ := metadataJPEG(t)
	plain := plainJPEG(t)
	pngIn, _ := metadataPNG(t)
	webp, _ := metadataWebP()
	set32 := func(b []byte, at int, v uint32, big bool) []byte {
		b = append([]byte{}, b...)
		if big {
			binary.BigEndian.PutUint32(b[at:], v)
		} else {
			binary.LittleEndian.PutUint32(b[at:], v)
		}
		return b
	}
	for _, tc := range []struct {
		name, mimeType string
		raw            []byte
	}{
		{"jpeg truncated in a segment", "image/jpeg", jpg[:40]},
		{"jpeg truncated in the scan", "image/jpeg", plain[:len(plain)-40]},
		{"jpeg without EOI", "image/jpeg", plain[:len(plain)-2]},
		{"jpeg length past the end", "image/jpeg", append([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x7F, 0xFF}, metaMarker...)},
		{"jpeg EOI before any scan", "image/jpeg", []byte{0xFF, 0xD8, 0xFF, 0xFE, 0x00, 0x03, 'x', 0xFF, 0xD9}},
		{"jpeg junk between segments", "image/jpeg", append(append([]byte{0xFF, 0xD8, 0xFF}, 0xFE, 0x00, 0x03, 'x', 0x00), plain[2:]...)},
		{"png truncated", "image/png", pngIn[:len(pngIn)/2]},
		{"png without IEND", "image/png", plainPNG(t)[:len(plainPNG(t))-12]},
		{"png not starting with IHDR", "image/png", append([]byte(pngSignature), appendPNGChunk(nil, "tEXt", []byte(metaMarker))...)},
		{"png length past the end", "image/png", set32(plainPNG(t), 8+25, 1<<30, true)},
		{"webp RIFF size past the end", "image/webp", set32(webp, 4, uint32(len(webp)), false)},
		{"webp chunk past the end", "image/webp", set32(riff(webpChunk("VP8 ", make([]byte, 14))), 16, 1000, false)},
		{"webp short VP8X", "image/webp", riff(webpChunk("VP8X", []byte{webpEXIF}), webpChunk("VP8 ", make([]byte, 14)))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if http.DetectContentType(tc.raw) != tc.mimeType {
				t.Fatalf("fixture sniffs as %s", http.DetectContentType(tc.raw))
			}
			s, rt, led := newTestService(t, nil)
			data := base64.StdEncoding.EncodeToString(tc.raw)
			_, err := s.Run(context.Background(), imageCall(t, owner, imageMessage(imgPart(tc.mimeType, data))))
			if code(err) != CodeInvalidRequest || !strings.Contains(err.Error(), "not a well-formed "+tc.mimeType) {
				t.Fatalf("err = %v", err)
			}
			if strings.Contains(err.Error(), data[:min(16, len(data))]) || strings.Contains(err.Error(), metaMarker) {
				t.Fatal("the message quoted image data")
			}
			if rt.calls() != 0 || len(led.rows) != 0 {
				t.Fatal("a malformed image reached the provider or the ledger")
			}
		})
	}
}

// The provider receives the stripped bytes, whichever way the data arrived.
func TestTheProviderReceivesStrippedImages(t *testing.T) {
	jpg, jpgWant := metadataJPEG(t)
	pngIn, pngWant := metadataPNG(t)
	webp, webpWant := metadataWebP()
	s, rt, _ := newTestService(t, nil)
	call := imageCall(t, owner, imageMessage(
		textPart("What is this?"),
		imgPart("image/jpeg", base64.StdEncoding.EncodeToString(jpg)),
		map[string]any{"type": "image", "data": "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngIn)},
		imgPart("image/webp", base64.StdEncoding.EncodeToString(webp)),
	))
	if _, err := s.Run(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]string
	if err := json.Unmarshal(decodeContext(t, rt.jobs[0].Context).Messages[0].Content, &blocks); err != nil {
		t.Fatal(err)
	}
	for i, want := range [][]byte{jpgWant, pngWant, webpWant} {
		got, err := base64.StdEncoding.DecodeString(blocks[i+1]["data"])
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("image %d reached the provider unstripped (%v)", i, err)
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
