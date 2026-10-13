package cliphist

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

// MaxClipBytes caps what's recorded; huge pastes are almost never worth keeping.
const MaxClipBytes = 64 * 1024

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), // JWT
	regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),                                  // AWS access key
	regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr|github_pat)_[A-Za-z0-9_]{20,}\b`),       // GitHub
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`),                               // Slack
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),                                      // API keys (OpenAI-style etc.)
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),                                   // GitLab
	// key=value / key: value where the key names a secret.
	regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|auth)\b\s*[:=]\s*\S+`),
}

// Sensitive reports whether text looks like a password, token or key and
// must not be recorded.
func Sensitive(text string) bool {
	for _, re := range secretPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	// A single "word" that looks random is most likely a generated password or token.
	t := strings.TrimSpace(text)
	if !strings.ContainsAny(t, " \t\n") && looksRandom(t) {
		return true
	}
	return false
}

// looksRandom flags 12+ char strings mixing at least three character classes
// with high per-character entropy. Paths, hostnames and plain words don't qualify.
func looksRandom(s string) bool {
	if len(s) < 12 || len(s) > 256 {
		return false
	}
	// Typical non-secret shapes: paths, URLs, dotted hostnames/IPs, emails.
	if strings.ContainsAny(s, "/\\") || strings.Count(s, ".") >= 2 || strings.Contains(s, "@") {
		return false
	}
	var lower, upper, digit, other bool
	for _, r := range s {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			other = true
		}
	}
	classes := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			classes++
		}
	}
	return classes >= 3 && entropy(s) >= 3.0
}

// entropy is the Shannon entropy of s in bits per character.
func entropy(s string) float64 {
	counts := map[rune]int{}
	n := 0
	for _, r := range s {
		counts[r]++
		n++
	}
	var h float64
	for _, c := range counts {
		p := float64(c) / float64(n)
		h -= p * math.Log2(p)
	}
	return h
}

// Recordable reports whether text should go into history at all.
func Recordable(text string) bool {
	return strings.TrimSpace(text) != "" && len(text) <= MaxClipBytes && !Sensitive(text)
}
