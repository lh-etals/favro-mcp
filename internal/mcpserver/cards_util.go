package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// contentHash is a small, stable stand-in for a text body - change detection,
// not security - so a diffing client can tell a description edited from one
// that didn't without paying for the full text on every poll.
func contentHash(s string) string {
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
