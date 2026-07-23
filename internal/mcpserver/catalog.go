package mcpserver

// ToolInfo describes one tool for the installer's toolset selection UI.
type ToolInfo struct {
	Name        string
	Tier        string // TierRead | TierWrite | TierDelete
	Description string
}

// ToolCatalog returns the full list of tools (name, tier, description) by
// running the register* functions in capture mode. This is the single source of
// truth - the same addTool calls that register tools also populate this list.
func ToolCatalog() []ToolInfo {
	catalogCapture = []ToolInfo{}
	s := &Server{session: NewSession(), toolset: TierDelete}
	s.registerTools(nil) // srv is nil; addTool returns during capture before use
	out := catalogCapture
	catalogCapture = nil
	return out
}
