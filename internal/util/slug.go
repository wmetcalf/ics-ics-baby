package util

import (
	"regexp"
	"strings"
	"unicode"
)

var nonWord = regexp.MustCompile(`[^\w\s-]`)
var multiDash = regexp.MustCompile(`[-\s]+`)

func Slugify(s string) string {
	var b strings.Builder
	s = strings.TrimSpace(s)
	s = nonWord.ReplaceAllString(s, "")
	for _, r := range s {
		if r > unicode.MaxASCII {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	out := b.String()
	out = multiDash.ReplaceAllString(out, "-")
	return strings.Trim(out, "-")
}
