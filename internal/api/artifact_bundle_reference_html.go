package api

import (
	"html"
	"strings"

	htmlparser "golang.org/x/net/html"
)

func artifactBundleHTMLSpans(content string, depth int, budget *int64) artifactBundleReferenceSpans {
	tokenizer := htmlparser.NewTokenizer(strings.NewReader(content))
	var nested artifactBundleReferenceSpans
	var attributes artifactBundleHTMLAttributes
	var tag string
	var scriptTypeSeen bool
	rawLanguage := artifactBundleText
	offset := 0
	return func() (artifactBundleReferenceSpan, bool) {
		for {
			if nested != nil {
				if span, ok := nested(); ok {
					return span, true
				}
				nested = nil
			}
			if name, start, end, ok := attributes.next(); ok {
				raw := content[start:end]
				if strings.EqualFold(name, "type") && tag == "script" && !scriptTypeSeen {
					scriptTypeSeen = true
					value := strings.ToLower(strings.TrimSpace(artifactBundleHTMLAttributeValue(raw)))
					if value != "" && value != "module" && value != "importmap" {
						rawLanguage = artifactBundleLanguage(value)
					}
				}
				span := artifactBundleReferenceSpan{text: raw, offset: start, depth: depth, budget: budget}
				if start > 0 && (content[start-1] == '\'' || content[start-1] == '"') {
					span.quote = content[start-1]
				}
				language := artifactBundleText
				switch {
				case strings.EqualFold(name, "style"):
					language = artifactBundleCSS
				case len(name) > 2 && strings.EqualFold(name[:2], "on"):
					language = artifactBundleJavaScript
				case strings.EqualFold(name, "srcdoc"):
					language = artifactBundleHTML
				case strings.EqualFold(name, "srcset"), strings.EqualFold(name, "imagesrcset"):
					span.srcset = true
				}
				// Encoded quotes in an ordinary attribute are URL bytes. CSS and
				// JS attribute values instead have their own string delimiters.
				span.literalAttribute = language == artifactBundleText && artifactBundleURLAttribute(name)
				// Browsers decode attribute character references before running CSS
				// or JS. Map monotonically back to raw bytes when yielding a URL.
				if decoded := artifactBundleHTMLAttributeValue(raw); decoded != raw {
					mapping := &artifactBundleHTMLOffsets{raw: raw, base: start}
					span.text, span.offset, span.sourceOffset = decoded, 0, mapping.sourceOffset
				}
				nested = artifactBundleCodeSpans(span, language)
				continue
			}
			token := tokenizer.Next()
			start := offset
			offset += len(tokenizer.Raw())
			switch token {
			case htmlparser.ErrorToken:
				return artifactBundleReferenceSpan{}, false
			case htmlparser.StartTagToken, htmlparser.SelfClosingTagToken:
				name, _ := tokenizer.TagName()
				tag = string(name)
				scriptTypeSeen = false
				rawLanguage = artifactBundleText
				if tag == "style" {
					rawLanguage = artifactBundleCSS
				} else if tag == "script" {
					rawLanguage = artifactBundleJavaScript
				}
				attributes = artifactBundleHTMLAttributes{raw: content[start:offset], base: start, at: 1 + len(name)}
			case htmlparser.TextToken:
				nested = artifactBundleCodeSpans(artifactBundleReferenceSpan{text: content[start:offset], offset: start, depth: depth, budget: budget}, rawLanguage)
			case htmlparser.EndTagToken:
				rawLanguage = artifactBundleText
			}
		}
	}
}

// URL-valued attributes have HTML delimiters, whereas structured values such
// as data-config JSON and srcdoc markup contain their own quote boundaries.
func artifactBundleURLAttribute(name string) bool {
	switch strings.ToLower(name) {
	case "src", "href", "srcset", "imagesrcset", "action", "formaction", "poster", "data", "cite",
		"background", "longdesc", "profile", "codebase", "classid", "archive", "icon", "usemap", "manifest", "xlink:href",
		"ping", "attributionsrc", "itemid", "itemtype":
		return true
	}
	return false
}

// The HTML tokenizer establishes tag boundaries and raw-text context. This
// cursor only locates raw attribute values: TagAttr decodes them and loses the
// source offsets needed for a byte-preserving rewrite.
type artifactBundleHTMLAttributes struct {
	raw      string
	base, at int
}

func (attributes *artifactBundleHTMLAttributes) next() (string, int, int, bool) {
	raw := attributes.raw
	for attributes.at < len(raw) {
		for attributes.at < len(raw) && (artifactBundleSpace(raw[attributes.at]) || raw[attributes.at] == '/') {
			attributes.at++
		}
		if attributes.at >= len(raw) || raw[attributes.at] == '>' {
			break
		}
		nameStart := attributes.at
		// An initial '=' is part of an attribute name (HTML's error recovery).
		attributes.at++
		for attributes.at < len(raw) && !artifactBundleSpace(raw[attributes.at]) && !strings.ContainsRune("/=>", rune(raw[attributes.at])) {
			attributes.at++
		}
		name := raw[nameStart:attributes.at]
		for attributes.at < len(raw) && artifactBundleSpace(raw[attributes.at]) {
			attributes.at++
		}
		if attributes.at >= len(raw) || raw[attributes.at] != '=' {
			return name, attributes.base + attributes.at, attributes.base + attributes.at, true
		}
		attributes.at++
		for attributes.at < len(raw) && artifactBundleSpace(raw[attributes.at]) {
			attributes.at++
		}
		start := attributes.at
		if start < len(raw) && (raw[start] == '\'' || raw[start] == '"') {
			quote := raw[start]
			start++
			attributes.at++
			for attributes.at < len(raw) && raw[attributes.at] != quote {
				attributes.at++
			}
			end := attributes.at
			if attributes.at < len(raw) {
				attributes.at++
			}
			return name, attributes.base + start, attributes.base + end, true
		}
		for attributes.at < len(raw) && !artifactBundleSpace(raw[attributes.at]) && raw[attributes.at] != '>' {
			attributes.at++
		}
		return name, attributes.base + start, attributes.base + attributes.at, true
	}
	return "", 0, 0, false
}

// Offset mapping retains two cursors, not an integer per decoded byte or a span
// per entity. Both cursors move forward over the attribute at most once.
type artifactBundleHTMLOffsets struct {
	raw                    string
	base, rawAt, decodedAt int
}

func (mapping *artifactBundleHTMLOffsets) sourceOffset(decodedOffset int) int {
	for mapping.decodedAt < decodedOffset && mapping.rawAt < len(mapping.raw) {
		rawSize, decodedSize := 1, 1
		if mapping.raw[mapping.rawAt] == '&' {
			var decoded string
			rawSize, decoded = artifactBundleHTMLAttributeEntity(mapping.raw[mapping.rawAt:])
			decodedSize = len(decoded)
		}
		mapping.rawAt += rawSize
		mapping.decodedAt += decodedSize
	}
	return mapping.base + mapping.rawAt
}

func artifactBundleHTMLAttributeValue(raw string) string {
	ampersand := strings.IndexByte(raw, '&')
	if ampersand < 0 {
		return raw
	}
	var decoded strings.Builder
	for ampersand >= 0 {
		decoded.WriteString(raw[:ampersand])
		raw = raw[ampersand:]
		size, value := artifactBundleHTMLAttributeEntity(raw)
		decoded.WriteString(value)
		raw = raw[size:]
		ampersand = strings.IndexByte(raw, '&')
	}
	decoded.WriteString(raw)
	return decoded.String()
}

func artifactBundleHTMLAttributeEntity(raw string) (int, string) {
	end := 1
	for end < len(raw) {
		character := raw[end]
		if character != '#' && !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
			break
		}
		end++
	}
	if end < len(raw) && raw[end] == ';' {
		end++
	}
	encoded := raw[:end]
	decoded := html.UnescapeString(encoded)
	if decoded == encoded {
		return 1, "&"
	}
	// UnescapeString may decode a semicolon-less prefix, leaving a suffix.
	for len(encoded) > 0 && len(decoded) > 0 && encoded[len(encoded)-1] == decoded[len(decoded)-1] {
		encoded = encoded[:len(encoded)-1]
		decoded = decoded[:len(decoded)-1]
	}
	// In attributes a named reference without ';' is literal when followed by
	// an ASCII letter, digit or '=' (unlike UnescapeString's text context).
	if raw[1] != '#' && encoded[len(encoded)-1] != ';' && len(encoded) < len(raw) {
		next := raw[len(encoded)]
		if next == '=' || next >= 'a' && next <= 'z' || next >= 'A' && next <= 'Z' || next >= '0' && next <= '9' {
			return 1, "&"
		}
	}
	return len(encoded), decoded
}
