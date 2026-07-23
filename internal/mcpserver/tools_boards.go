package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerBoards(srv *mcp.Server, s *Server) {
	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "list_boards",
		Description: "List boards in the organization. By default lists boards at the TOP LEVEL only (not inside collections/folders). To find boards inside a collection: first call list_collections, then pass that collection's name or ID as the collection argument.",
	}, s.listBoards)

	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "get_board",
		Description: "Get details of a specific board including its columns and lanes.",
	}, s.getBoard)

	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "get_current_board",
		Description: "Get details of the currently selected board, including its columns and lanes.",
	}, s.getCurrentBoard)

	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "set_board",
		Description: "Select a board (by ID or name) as the active board for card operations.",
	}, s.setBoard)
}

type listBoardsArgs struct {
	Collection *string `json:"collection,omitempty" jsonschema:"Collection (folder) name or ID. If omitted, only top-level boards are returned"`
}

func (s *Server) listBoards(_ context.Context, _ *mcp.CallToolRequest, args listBoardsArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	collectionID := ""
	if args.Collection != nil && *args.Collection != "" {
		collections, err := client.GetCollections(false)
		if err != nil {
			return jsonResult(nil, err)
		}
		for _, c := range collections {
			if c.CollectionID == *args.Collection || strings.EqualFold(c.Name, *args.Collection) {
				collectionID = c.CollectionID
				break
			}
		}
		if collectionID == "" {
			return jsonResult(map[string]any{
				"error": "Collection '" + *args.Collection + "' not found. Use list_collections to see available collections.",
			}, nil)
		}
	}
	boards, err := client.GetWidgets(collectionID, false)
	if err != nil {
		return jsonResult(nil, err)
	}
	rows := make([]boardRow, 0, len(boards))
	for _, b := range boards {
		rows = append(rows, boardRow{Name: b.Name, ID: b.WidgetCommonID, Type: b.Type, Archived: b.Archived})
	}
	front := listBoardsFront{Boards: rows}
	where := ""
	if args.Collection != nil && *args.Collection != "" {
		front.Collection = *args.Collection
		where = " in " + *args.Collection
	}
	body := fmt.Sprintf("%d board(s)%s.", len(rows), where)
	if where == "" {
		body += " Pass collection=<name> to list boards inside a folder."
	}
	return textResult(rendered{front: front, body: body}.String())
}

// listBoardsFront is the ordered frontmatter for list_boards.
type listBoardsFront struct {
	Collection string     `yaml:"collection,omitempty"`
	Boards     []boardRow `yaml:"boards"`
}

// boardRow is one row of list_boards output (name-first, stable id reference).
type boardRow struct {
	Name     string `yaml:"name"`
	ID       string `yaml:"id"`
	Type     string `yaml:"type,omitempty"`
	Archived bool   `yaml:"archived,omitempty"`
}

type getBoardArgs struct {
	BoardID string `json:"board_id" jsonschema:"The board's widget_common_id"`
}

func (s *Server) getBoard(_ context.Context, _ *mcp.CallToolRequest, args getBoardArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	detail, err := s.boardDetail(client, args.BoardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(detail, nil)
}

func (s *Server) getCurrentBoard(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	if s.session.Board() == "" {
		return jsonResult(map[string]any{"message": "No board selected. Use set_board tool first."}, nil)
	}
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	detail, err := s.boardDetail(client, s.session.Board())
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(detail, nil)
}

type setBoardArgs struct {
	Board string `json:"board" jsonschema:"Board ID or name"`
}

func (s *Server) setBoard(_ context.Context, _ *mcp.CallToolRequest, args setBoardArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	w, err := NewResolver(client).Board(args.Board)
	if err != nil {
		return jsonResult(nil, err)
	}
	s.session.SetBoard(w.WidgetCommonID)
	return jsonResult(map[string]any{
		"message":          "Selected board: " + w.Name,
		"widget_common_id": w.WidgetCommonID,
		"name":             w.Name,
		"type":             w.Type,
	}, nil)
}

// boardDetail builds the common board+columns+lanes payload.
func (s *Server) boardDetail(client *favro.Client, widgetCommonID string) (map[string]any, error) {
	board, err := client.GetWidget(widgetCommonID)
	if err != nil {
		return nil, err
	}
	columns, err := client.GetColumns(widgetCommonID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(columns, func(i, j int) bool { return columns[i].Position < columns[j].Position })
	cols := make([]map[string]any, 0, len(columns))
	for _, c := range columns {
		cols = append(cols, map[string]any{
			"column_id":  c.ColumnID,
			"name":       c.Name,
			"position":   c.Position,
			"card_count": c.CardCount,
		})
	}
	lanes := make([]map[string]any, 0, len(board.Lanes))
	for _, l := range board.Lanes {
		lanes = append(lanes, map[string]any{"lane_id": l.LaneID, "name": l.Name})
	}
	color := ""
	if board.Color != nil {
		color = *board.Color
	}
	return map[string]any{
		"widget_common_id": board.WidgetCommonID,
		"name":             board.Name,
		"type":             board.Type,
		"archived":         board.Archived,
		"color":            color,
		"columns":          cols,
		"lanes":            lanes,
	}, nil
}
