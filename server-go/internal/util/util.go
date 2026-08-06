// Package util holds tiny shared helpers used across the Go control plane.
package util

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"unicode"
)

// UID returns a short random hex id (suitable for run-/wf-/audit ids).
func UID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Slug lowercases and collapses non-alphanumerics into underscores.
func Slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// TitleCase capitalizes the first letter of each word.
func TitleCase(s string) string {
	prev := true
	return strings.Map(func(r rune) rune {
		if prev && unicode.IsLetter(r) {
			prev = false
			return unicode.ToUpper(r)
		}
		if r == ' ' || r == '-' || r == '_' {
			prev = true
		} else {
			prev = false
		}
		return r
	}, s)
}
