package mcpserver

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// rendered is the output-format contract: optional YAML frontmatter (strictly
// structured data) followed by an optional Markdown body. Code-like values that
// must appear in the body go inside fenced (~~~) blocks.
type rendered struct {
	front any    // marshalled to YAML between --- fences; nil = no frontmatter
	body  string // Markdown body (may be empty)
}

func (r rendered) String() string {
	var b strings.Builder
	if r.front != nil {
		if y, err := yaml.Marshal(r.front); err == nil {
			b.WriteString("---\n")
			b.Write(y)
			b.WriteString("---\n")
		}
		if r.body != "" {
			b.WriteString("\n")
		}
	}
	b.WriteString(r.body)
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
