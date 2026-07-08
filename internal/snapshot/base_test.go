package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInjectBaseHref(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		pageURL string
		want    string
	}{
		{
			"inserted first in head",
			`<html><head><title>t</title></head><body></body></html>`,
			"https://ex.com/a/page.html",
			`<html><head><base href="https://ex.com/a/page.html"><title>t</title></head><body></body></html>`,
		},
		{
			"case-insensitive head match",
			`<HTML><HEAD><TITLE>t</TITLE></HEAD></HTML>`,
			"https://ex.com/",
			`<HTML><HEAD><base href="https://ex.com/"><TITLE>t</TITLE></HEAD></HTML>`,
		},
		{
			"head with attributes falls back to closing tag",
			`<html><head lang="en"><title>t</title></head></html>`,
			"https://ex.com/",
			`<html><head lang="en"><title>t</title><base href="https://ex.com/"></head></html>`,
		},
		{
			"no head prepends",
			`<p>hi</p>`,
			"https://ex.com/",
			`<base href="https://ex.com/"><p>hi</p>`,
		},
		{
			"existing base is preserved",
			`<html><head><base href="https://author.example/"><title>t</title></head></html>`,
			"https://ex.com/",
			`<html><head><base href="https://author.example/"><title>t</title></head></html>`,
		},
		{
			"self-closing base counts as existing",
			`<html><head><base/></head></html>`,
			"https://ex.com/",
			`<html><head><base/></head></html>`,
		},
		{
			"basefont is not a base tag",
			`<html><head><basefont size="3"></head></html>`,
			"https://ex.com/",
			`<html><head><base href="https://ex.com/"><basefont size="3"></head></html>`,
		},
		{
			"empty page URL is a no-op",
			`<html><head></head></html>`,
			"",
			`<html><head></head></html>`,
		},
		{
			// A multi-byte rune before <head> whose Unicode-lowered form is a
			// different byte length (the 3-byte Kelvin sign 'K' → 1-byte 'k')
			// would shift a ToLower-computed index; matching over the original
			// bytes keeps the insertion point correct.
			"multi-byte rune before head does not shift insertion",
			"<html><!--K--><head><title>t</title></head></html>",
			"https://ex.com/",
			"<html><!--K--><head><base href=\"https://ex.com/\"><title>t</title></head></html>",
		},
		{
			"attribute value is escaped",
			`<html><head></head></html>`,
			`https://ex.com/?q="x"&r=1`,
			`<html><head><base href="https://ex.com/?q=&#34;x&#34;&amp;r=1"></head></html>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, InjectBaseHref(c.body, c.pageURL))
		})
	}
}
