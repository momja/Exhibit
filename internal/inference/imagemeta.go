package inference

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// Image metadata stripping (Exh-v81w). Every image complete() forwards is
// rewritten first, keeping only what is needed to draw its pixels: no
// location, camera, timestamps, comments, editing history, thumbnails or
// colour profiles (an ICC profile names the device that made it). The host
// page does the same before the bytes leave the device
// (web/gallery/image-metadata.js); this is the backstop for a client that
// bypasses it. The two implementations follow the same rules, stated once
// here and once there, and both are held to the shared corpus in
// testdata/imagemeta:
//
//   - JPEG: walk the segments from SOI. Keep APP14 (Adobe, which decides the
//     colour transform) and every non-APP segment (tables, frame and scan
//     headers, restart intervals). Drop every other APPn, APP0 included (JFIF
//     and JFXX can carry a thumbnail; APP1 is EXIF and XMP; APP2 is ICC and
//     MPF), and COM. Scan data is copied verbatim up to the next marker, and
//     walking resumes there, so a progressive file's later segments follow
//     the same rules. A scan needs a frame header (SOFn) before it, and EOI
//     needs a scan before it. Output ends at EOI: anything after it (a second
//     picture, a motion photo's video, a vendor trailer) is dropped.
//   - PNG: IHDR comes first and is 13 bytes. Keep IHDR, PLTE, IDAT, IEND,
//     tRNS, gAMA, cHRM, sRGB, sBIT, pHYs, bKGD, cICP, mDCV and cLLI; drop
//     every other chunk (eXIf, tEXt, zTXt, iTXt, tIME, iCCP and anything
//     unknown). Kept chunks are copied verbatim, CRC included. At least one
//     IDAT comes before IEND, IEND is empty, and output ends there. acTL,
//     fcTL or fdAT make it an APNG, which is refused.
//   - WebP: keep VP8, VP8L, VP8X and ALPH; drop every other chunk (EXIF, XMP,
//     ICCP and anything unknown). VP8X's ICC (0x20), EXIF (0x08) and XMP
//     (0x04) flags are cleared and the RIFF size is rewritten. A VP8 or VP8L
//     image chunk is required. ANIM, ANMF or VP8X's animation flag (0x02)
//     make it animated, which is refused. Output ends at the RIFF's end.
//
// Orientation goes with EXIF, so a phone photo that relied on it may reach
// the model rotated, and colours fall back to sRGB without the profile.
// Pixels are never decoded or re-encoded.
//
// A file that cannot be walked cleanly to its end (truncated, a length past
// the end, a required chunk or segment missing) is refused, never forwarded
// as it came. The walk stops at the first problem, so the two
// implementations refuse the same file for the same reason.

var (
	// errMalformedImage is a file of an accepted type whose structure could
	// not be walked.
	errMalformedImage = errors.New("malformed image")
	// errAnimatedImage is an APNG or an animated WebP.
	errAnimatedImage = errors.New("animated image")
)

func stripImageMetadata(mimeType string, raw []byte) ([]byte, error) {
	switch mimeType {
	case "image/jpeg":
		return stripJPEG(raw)
	case "image/png":
		return stripPNG(raw)
	case "image/webp":
		return stripWebP(raw)
	}
	return nil, errMalformedImage
}

// keepJPEGSegment says whether a length-bearing segment survives.
func keepJPEGSegment(marker byte) bool {
	switch {
	case marker == 0xEE: // APP14 Adobe
		return true
	case marker >= 0xE0 && marker <= 0xEF, marker == 0xFE: // every other APPn, COM
		return false
	}
	return true
}

// isJPEGFrame says whether a marker is a frame header, SOF0–SOF15. C4 (DHT),
// C8 (JPG) and CC (DAC) share the range and are not.
func isJPEGFrame(marker byte) bool {
	return marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC
}

func stripJPEG(raw []byte) ([]byte, error) {
	n := len(raw)
	if n < 4 || raw[0] != 0xFF || raw[1] != 0xD8 {
		return nil, errMalformedImage
	}
	out := make([]byte, 0, n)
	out = append(out, 0xFF, 0xD8)
	sawFrame, sawScan := false, false
	i := 2
	for {
		if i >= n || raw[i] != 0xFF {
			return nil, errMalformedImage
		}
		for i < n && raw[i] == 0xFF { // fill bytes before a marker
			i++
		}
		if i >= n {
			return nil, errMalformedImage
		}
		marker := raw[i]
		i++
		switch {
		case marker == 0xD9: // EOI
			if !sawScan {
				return nil, errMalformedImage
			}
			return append(out, 0xFF, 0xD9), nil
		case marker == 0x00, marker == 0xD8:
			return nil, errMalformedImage
		case marker == 0x01, marker >= 0xD0 && marker <= 0xD7: // TEM, RSTn
			out = append(out, 0xFF, marker)
			continue
		}
		if i+2 > n {
			return nil, errMalformedImage
		}
		length := int(binary.BigEndian.Uint16(raw[i:]))
		if length < 2 || i+length > n {
			return nil, errMalformedImage
		}
		if isJPEGFrame(marker) {
			sawFrame = true
		}
		if marker == 0xDA && !sawFrame { // a scan with no frame header
			return nil, errMalformedImage
		}
		if keepJPEGSegment(marker) {
			out = append(out, 0xFF, marker)
			out = append(out, raw[i:i+length]...)
		}
		i += length
		if marker != 0xDA { // not SOS
			continue
		}
		// Entropy-coded data runs to the next marker: FF 00 is a stuffed
		// byte, FF D0–D7 a restart, FF FF fill.
		sawScan = true
		start := i
		for {
			if i+1 >= n {
				return nil, errMalformedImage
			}
			if raw[i] != 0xFF {
				i++
				continue
			}
			next := raw[i+1]
			if next == 0x00 || (next >= 0xD0 && next <= 0xD7) {
				i += 2
				continue
			}
			if next == 0xFF {
				i++
				continue
			}
			break
		}
		out = append(out, raw[start:i]...)
	}
}

// pngKept are the chunks needed to draw the pixels.
var pngKept = map[string]bool{
	"IHDR": true, "PLTE": true, "IDAT": true, "IEND": true,
	"tRNS": true, "gAMA": true, "cHRM": true, "sRGB": true, "sBIT": true, "pHYs": true, "bKGD": true,
	"cICP": true, "mDCV": true, "cLLI": true,
}

// pngAnimation are the APNG chunks.
var pngAnimation = map[string]bool{"acTL": true, "fcTL": true, "fdAT": true}

const pngSignature = "\x89PNG\r\n\x1a\n"

func stripPNG(raw []byte) ([]byte, error) {
	if !bytes.HasPrefix(raw, []byte(pngSignature)) {
		return nil, errMalformedImage
	}
	out := make([]byte, 0, len(raw))
	out = append(out, pngSignature...)
	sawData := false
	for i, first := len(pngSignature), true; ; first = false {
		if i+8 > len(raw) {
			return nil, errMalformedImage
		}
		length := binary.BigEndian.Uint32(raw[i:])
		kind := string(raw[i+4 : i+8])
		if length > 1<<31-1 || uint64(len(raw)-i) < 12+uint64(length) {
			return nil, errMalformedImage
		}
		if first != (kind == "IHDR") || (kind == "IHDR" && length != 13) {
			return nil, errMalformedImage
		}
		if pngAnimation[kind] {
			return nil, errAnimatedImage
		}
		end := i + 12 + int(length)
		if pngKept[kind] {
			out = append(out, raw[i:end]...)
		}
		switch kind {
		case "IDAT":
			sawData = true
		case "IEND":
			if length != 0 || !sawData {
				return nil, errMalformedImage
			}
			return out, nil
		}
		i = end
	}
}

// webpKept are the chunks needed to draw the pixels.
var webpKept = map[string]bool{"VP8 ": true, "VP8L": true, "VP8X": true, "ALPH": true}

// The VP8X flags (byte 0 of its payload) that are cleared or refused.
const (
	webpFlagICC       = 0x20
	webpFlagEXIF      = 0x08
	webpFlagXMP       = 0x04
	webpFlagAnimation = 0x02
)

func stripWebP(raw []byte) ([]byte, error) {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
		return nil, errMalformedImage
	}
	riffSize := binary.LittleEndian.Uint32(raw[4:])
	if riffSize < 4 || uint64(riffSize) > uint64(len(raw)-8) {
		return nil, errMalformedImage
	}
	end := 8 + int(riffSize)
	out := make([]byte, 12, end)
	copy(out, raw[:12])
	sawImage := false
	for i := 12; i < end; {
		if i+8 > end {
			return nil, errMalformedImage
		}
		kind := string(raw[i : i+4])
		size := uint64(binary.LittleEndian.Uint32(raw[i+4:]))
		next := uint64(i) + 8 + size + size&1 // an odd-sized chunk is padded
		if next > uint64(end) {
			return nil, errMalformedImage
		}
		switch kind {
		case "ANIM", "ANMF":
			return nil, errAnimatedImage
		case "VP8X":
			if size < 10 {
				return nil, errMalformedImage
			}
			if raw[i+8]&webpFlagAnimation != 0 {
				return nil, errAnimatedImage
			}
		case "VP8 ", "VP8L":
			sawImage = true
		}
		if webpKept[kind] {
			start := len(out)
			out = append(out, raw[i:next]...)
			if kind == "VP8X" {
				out[start+8] &^= webpFlagICC | webpFlagEXIF | webpFlagXMP
			}
		}
		i = int(next)
	}
	if !sawImage {
		return nil, errMalformedImage
	}
	binary.LittleEndian.PutUint32(out[4:], uint32(len(out)-8))
	return out, nil
}
