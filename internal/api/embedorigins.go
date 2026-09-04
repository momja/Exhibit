package api

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/momja/Exhibit/internal/origin"
)

// Third-party embedding configuration (av-6nbo).
//
// Every render document carries `frame-ancestors <APP_ORIGIN>`, so nothing but
// the app's own pages may put one in an iframe. EMBED_ORIGINS names the other
// sites allowed to embed a *share* — a landing page that wants to show a real,
// running artifact rather than a screenshot of one. Unset, which is every
// instance that has not asked for this, the emitted policy is byte-identical to
// what it has always been.
//
// Why this is configuration and not a per-artifact decision: it is a property
// of the deployment (which of my own sites may frame my library's shares),
// not of any one artifact, and it is the operator's to state once. The
// per-artifact allowlist governs the opposite direction — what an artifact
// reaches out to — and nothing about framing belongs in it.
const envEmbedOrigins = "EMBED_ORIGINS"

// EmbedOriginsFromEnv reads EMBED_ORIGINS: origins separated by commas or
// whitespace, empty by default. Parsing lives here, beside the other
// configuration this package reads from the environment, because it is the
// whole of this knob's behaviour and the part worth testing.
//
// Entries are origins in exactly the sense internal/origin defines — the same
// rule the per-artifact allowlist is held to (av-i7hd), and for the same
// reason: these values are pasted into a CSP directive, where a path-bearing
// entry is path-matched and an oddly-spelled host is a second name for a
// decision already made. There is one definition of "an origin" in this
// codebase and this reuses it rather than inventing a second.
//
// An unusable entry is an error the caller makes fatal, not an entry silently
// dropped. Dropping fails closed — the site simply cannot frame the share, which
// is the behaviour of every instance that never set this — but it fails
// *invisibly*: the operator sees a broken frame on a page they configured
// correctly as far as they know, with nothing anywhere naming the typo. This is
// read once at startup by someone who set it deliberately, and that is the only
// cheap moment to say so.
func EmbedOriginsFromEnv() ([]string, error) {
	// Comma or whitespace, accepted interchangeably: neither can appear inside
	// an origin, so there is no ambiguity to resolve, and an operator who
	// copies the space-separated form out of a CSP header gets the same result
	// as one who writes the comma-separated form other list-valued environment
	// variables use.
	fields := strings.FieldsFunc(os.Getenv(envEmbedOrigins), func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	if len(fields) == 0 {
		return nil, nil
	}
	// Deliberately not origin.NormalizeOrigins, whose error reads "invalid
	// allowlist entry" — right where it is surfaced (the per-artifact allowlist
	// editor) and misleading here, where the operator is reading a startup
	// failure about an environment variable and would go looking at an
	// artifact. The rule — NormalizeOrigin — is shared; only the sentence
	// around it differs.
	origins := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		o, err := origin.NormalizeOrigin(field)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not an origin: %w", envEmbedOrigins, field, err)
		}
		if _, dup := seen[o]; dup {
			continue
		}
		seen[o] = struct{}{}
		origins = append(origins, o)
	}
	return origins, nil
}
