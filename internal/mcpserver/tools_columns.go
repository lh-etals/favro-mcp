package mcpserver

import (
	"context"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerColumns(srv *mcp.Server, s *Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_columns",
		Description: "List all columns on a specific board, sorted by position.",
	}, s.listColumns)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_column",
		Description: "Create a new column on a board. Appends to the end unless a position is given.",
	}, s.createColumn)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "rename_column",
		Description: "Rename a column (by column ID or name within a board).",
	}, s.renameColumn)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "move_column",
		Description: "Move a column to a new 0-based position.",
	}, s.moveColumn)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_column",
		Description: "Delete a column from a board. WARNING: this also deletes all cards in the column.",
	}, s.deleteColumn)
}

type listColumnsArgs struct {
	Board string `json:"board" jsonschema:"The board's widget_common_id, name, or ID"`
}

func (s *Server) listColumns(_ context.Context, _ *mcp.CallToolRequest, args listColumnsArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	boardID, err := NewResolver(client).Board(args.Board)
	if err != nil {
		return jsonResult(nil, err)
	}
	columns, err := client.GetColumns(boardID.WidgetCommonID)
	if err != nil {
		return jsonResult(nil, err)
	}
	sort.SliceStable(columns, func(i, j int) bool { return columns[i].Position < columns[j].Position })
	out := make([]map[string]any, 0, len(columns))
	for _, c := range columns {
		out = append(out, map[string]any{
			"column_id":  c.ColumnID,
			"name":       c.Name,
			"position":   c.Position,
			"card_count": c.CardCount,
		})
	}
	return jsonResult(map[string]any{"columns": out}, nil)
}

type createColumnArgs struct {
	Name     string  `json:"name" jsonschema:"Column name"`
	Board    *string `json:"board,omitempty" jsonschema:"Board ID or name (uses current board if not specified)"`
	Position *int    `json:"position,omitempty" jsonschema:"Position index (0-based); appends to end if not specified"`
}

func (s *Server) createColumn(_ context.Context, _ *mcp.CallToolRequest, args createColumnArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	boardID, err := s.resolveBoardArg(client, strOr(args.Board))
	if err != nil {
		return jsonResult(nil, err)
	}
	if boardID == "" {
		return jsonResult(nil, errNoBoard)
	}
	col, err := client.CreateColumn(boardID, args.Name, args.Position)
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":   "Created column: " + col.Name,
		"column_id": col.ColumnID,
		"name":      col.Name,
		"position":  col.Position,
	}, nil)
}

type renameColumnArgs struct {
	Column string  `json:"column" jsonschema:"Column ID or name"`
	Name   string  `json:"name" jsonschema:"New column name"`
	Board  *string `json:"board,omitempty" jsonschema:"Board ID or name (required for name lookup)"`
}

func (s *Server) renameColumn(_ context.Context, _ *mcp.CallToolRequest, args renameColumnArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	boardID, err := s.resolveBoardArg(client, strOr(args.Board))
	if err != nil {
		return jsonResult(nil, err)
	}
	if boardID == "" {
		return jsonResult(nil, errNoBoard)
	}
	col, err := NewResolver(client).Column(args.Column, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	updated, err := client.UpdateColumn(col.ColumnID, &args.Name, nil)
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":   "Renamed column to: " + updated.Name,
		"column_id": updated.ColumnID,
		"name":      updated.Name,
	}, nil)
}

type moveColumnArgs struct {
	Column   string  `json:"column" jsonschema:"Column ID or name"`
	Position int     `json:"position" jsonschema:"New position index (0-based)"`
	Board    *string `json:"board,omitempty" jsonschema:"Board ID or name (required for name lookup)"`
}

func (s *Server) moveColumn(_ context.Context, _ *mcp.CallToolRequest, args moveColumnArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	boardID, err := s.resolveBoardArg(client, strOr(args.Board))
	if err != nil {
		return jsonResult(nil, err)
	}
	if boardID == "" {
		return jsonResult(nil, errNoBoard)
	}
	col, err := NewResolver(client).Column(args.Column, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	pos := args.Position
	updated, err := client.UpdateColumn(col.ColumnID, nil, &pos)
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":   "Moved column '" + updated.Name + "' to position " + itoa(args.Position),
		"column_id": updated.ColumnID,
		"position":  updated.Position,
	}, nil)
}

type deleteColumnArgs struct {
	Column string  `json:"column" jsonschema:"Column ID or name"`
	Board  *string `json:"board,omitempty" jsonschema:"Board ID or name (required for name lookup)"`
}

func (s *Server) deleteColumn(_ context.Context, _ *mcp.CallToolRequest, args deleteColumnArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	boardID, err := s.resolveBoardArg(client, strOr(args.Board))
	if err != nil {
		return jsonResult(nil, err)
	}
	if boardID == "" {
		return jsonResult(nil, errNoBoard)
	}
	col, err := NewResolver(client).Column(args.Column, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	if err := client.DeleteColumn(col.ColumnID); err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":   "Deleted column: " + col.Name,
		"column_id": col.ColumnID,
	}, nil)
}
