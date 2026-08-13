package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// awsSigV4Param matches one AWS SigV4 query parameter (?X-Amz-Signature=...,
// &X-Amz-Date=..., etc.), case-insensitively and regardless of position.
//
// An inline image pasted into a description embeds a presigned S3 URL to the
// file, and Favro reissues that URL - a new date and signature, same file -
// on every fetch. Confirmed live: three back-to-back GETs of one untouched
// card each carried a different signature and so a different raw hash, which
// would have reported every attachment-bearing card as edited on every poll.
// The object path (and so the hash) stays stable once these are stripped.
var awsSigV4Param = regexp.MustCompile(`(?i)[?&]x-amz-[a-z0-9-]+=[^&\s)\]]*`)

// contentHash is a small, stable stand-in for a text body - change detection,
// not security - so a diffing client can tell a description edited from one
// that didn't without paying for the full text on every poll.
func contentHash(s string) string {
	s = awsSigV4Param.ReplaceAllString(s, "")
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

// stripTasklistFromDescription removes the trailing tasklist checkbox lines that
// Favro auto-appends to a card's detailedDescription. tasklists is a list of
// maps each with "name" and "tasks" ([]map[string]any with "name").
func stripTasklistFromDescription(description string, tasklists []map[string]any) string {
	if description == "" || len(tasklists) == 0 {
		return description
	}
	lines := strings.Split(strings.TrimRight(description, "\n"), "\n")

	checkboxPatterns := map[string]struct{}{}
	tasklistNames := map[string]struct{}{}
	for _, tl := range tasklists {
		if n, ok := tl["name"].(string); ok {
			tasklistNames[n] = struct{}{}
		}
		if tasks, ok := tl["tasks"].([]map[string]any); ok {
			for _, t := range tasks {
				if name, ok := t["name"].(string); ok {
					checkboxPatterns["☐ "+name] = struct{}{}
					checkboxPatterns["☑ "+name] = struct{}{}
				}
			}
		}
	}

	for len(lines) > 0 {
		line := strings.TrimSpace(lines[len(lines)-1])
		if line == "" {
			lines = lines[:len(lines)-1]
		} else if _, ok := checkboxPatterns[line]; ok {
			lines = lines[:len(lines)-1]
		} else if _, ok := tasklistNames[line]; ok {
			lines = lines[:len(lines)-1]
		} else {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}
