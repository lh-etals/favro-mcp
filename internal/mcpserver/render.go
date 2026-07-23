package mcpserver

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// rendered is the output-format contract: optional YAML frontmatter (strictly
// structured data, field order = struct declaration order) followed by an
// optional Markdown body. Code-like values that must appear in the body go
// inside fenced (~~~) blocks.
type rendered struct {
	front any // struct marshalled to YAML between --- fences; nil = no frontmatter
	body  string
}

func (r rendered) String() string {
	var b strings.Builder
	if r.front != nil {
		b.WriteString("---\n")
		if y, err := yaml.Marshal(r.front); err == nil {
			b.Write(y)
		} else {
			fmt.Fprintf(&b, "render_error: %q\n", err.Error())
		}
		b.WriteString("---\n")
		if r.body != "" {
			b.WriteString("\n")
		}
	}
	b.WriteString(r.body)
	if r.body != "" && !strings.HasSuffix(r.body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// textResult wraps a pre-rendered string as the MCP TextContent result.
func textResult(s string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}, nil, nil
}

// mdMessage renders a prose sentence followed by a fenced (~~~) block of the
// given identifiers (code-like values live inside the fence, per the contract).
func mdMessage(prose string, ids map[string]any) string {
	var b strings.Builder
	b.WriteString(prose)
	if len(ids) > 0 {
		b.WriteString("\n~~~\n")
		if y, err := yaml.Marshal(ids); err == nil {
			b.Write(y)
		}
		b.WriteString("~~~\n")
	} else {
		b.WriteString("\n")
	}
	return b.String()
}

// parseFrontmatter splits a rendered doc into its frontmatter (as a map, in
// document order) and the Markdown body, validating that the YAML is well
// formed and the fences are balanced. Used by tests to validate output.
func parseFrontmatter(s string) (front *yaml.Node, body string, err error) {
	s = strings.TrimLeft(s, "\n\r")
	if !strings.HasPrefix(s, "---") {
		return nil, s, fmt.Errorf("no frontmatter opening fence")
	}
	lines := strings.Split(s, "\n")
	if strings.TrimRight(lines[0], "\r") != "---" {
		return nil, s, fmt.Errorf("bad opening fence")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, "", fmt.Errorf("no frontmatter closing fence")
	}
	yamlBlock := strings.Join(lines[1:end], "\n")
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(yamlBlock), &node); err != nil {
		return nil, "", fmt.Errorf("invalid frontmatter YAML: %w", err)
	}
	return &node, strings.Join(lines[end+1:], "\n"), nil
}
