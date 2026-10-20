package api

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var artifactBundleURLStart = regexp.MustCompile(`(?:[A-Za-z][A-Za-z0-9+.-]*:)?//|(?i:data:|blob:|file:|javascript:|mailto:)|/`)

type artifactBundleReference struct {
	start, end int
	url        string
	separator  string
}

// Contexts yield one active source range at a time. Neither comments nor URL
// candidates are collected into a document-sized table.
type artifactBundleReferenceScanner struct {
	content string
	spans   artifactBundleReferenceSpans
	span    artifactBundleReferenceSpan
	at      int
	err     error
}

func newArtifactBundleReferenceScanner(content, contentType string) artifactBundleReferenceScanner {
	return artifactBundleReferenceScanner{content: content, spans: artifactBundleSpans(content, artifactBundleLanguage(contentType))}
}

func (scanner *artifactBundleReferenceScanner) next() (artifactBundleReference, bool) {
	if scanner.err != nil {
		return artifactBundleReference{}, false
	}
	for {
		if reference, ok := scanner.nextURL(); ok {
			return reference, true
		}
		var ok bool
		scanner.span, ok = scanner.spans()
		if !ok {
			return artifactBundleReference{}, false
		}
		if scanner.span.err != nil {
			scanner.err = scanner.span.err
			return artifactBundleReference{}, false
		}
		scanner.at = 0
	}
}

// Consume whole URL tokens, including foreign URLs. A nested local-looking path
// must never be reconsidered after rejecting its enclosing URL.
func (scanner *artifactBundleReferenceScanner) nextURL() (artifactBundleReference, bool) {
	span := scanner.span
	content := span.text
	for scanner.at < len(content) {
		match := artifactBundleURLStart.FindStringIndex(content[scanner.at:])
		if match == nil {
			scanner.at = len(content)
			break
		}
		start := scanner.at + match[0]
		scanner.at += match[1]
		if start > 0 && !strings.ContainsRune(" \t\r\n\f\"'`=([{,;>}", rune(content[start-1])) {
			continue
		}
		quote := span.quote
		before := strings.TrimRight(content[:start], " \t\r\n\f")
		cssURL := !span.literalAttribute && len(before) >= 4 && strings.EqualFold(before[len(before)-4:], "url(")
		escapedQuote := false
		if !span.literalAttribute && start > 0 && strings.ContainsRune("\"'`", rune(content[start-1])) {
			quote = content[start-1]
			if span.encodedString {
				// Encoded JSON inside a JS string has its own escaped delimiters.
				for at := start - 2; at >= 0 && content[at] == '\\'; at-- {
					escapedQuote = !escapedQuote
				}
			}
		}
		end := scanner.at
		for end < len(content) {
			character := content[end]
			if character == '\\' && end+1 < len(content) {
				if escapedQuote && content[end+1] == quote {
					break
				}
				end += 2
				continue
			}
			if strings.ContainsRune(" \t\r\n\f<>", rune(character)) ||
				(!span.literalAttribute && (character == quote && !escapedQuote || quote == 0 && strings.ContainsRune("\"'`", rune(character)))) ||
				(cssURL && character == ')') {
				break
			}
			end++
		}
		scanner.at = end
		urlEnd := end
		if span.srcset {
			urlEnd = start + len(strings.TrimRight(content[start:end], ","))
		}
		reference := artifactBundleReference{start: span.offset + start, end: span.offset + end,
			url: content[start:urlEnd], separator: content[urlEnd:end]}
		if span.sourceOffset != nil {
			reference.start = span.sourceOffset(reference.start)
			rawURLEnd := span.sourceOffset(span.offset + urlEnd)
			reference.end = span.sourceOffset(reference.end)
			// Decode for lexical context only. Returning original URL bytes also
			// preserves foreign URLs in import's unconditional rewrite loop.
			reference.url = scanner.content[reference.start:rawURLEnd]
			reference.separator = scanner.content[rawURLEnd:reference.end]
		}
		return reference, true
	}
	return artifactBundleReference{}, false
}

// Browser URL parsing compares numeric ports, including leading-zero forms.
// Keep this bundle-local: canonicalOrigin also serves unrelated legacy paths.
func artifactBundleOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 0 || number > 65535 {
			return ""
		}
		u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(number))
	}
	return canonicalOrigin(u.String())
}
