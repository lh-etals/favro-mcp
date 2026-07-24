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
		Description: "List boards in the organization. By default lists boards at the TOP LEVEL only (not inside collections/folders). To find boards inside a collection: first call list_collections, then pass that collection's name or ID as the collection argument on Favro.",
	}, s.listBoards)

	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "get_board",
		Description: "Get details of a specific board including its columns and lanes on Favro.",
	}, s.getBoard)

	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "get_current_board",
		Description: "Get details of the currently selected board, including its columns and lanes on Favro.",
	}, s.getCurrentBoard)

	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "set_board",
		Description: "Select a board (by ID or name) as the active board for card operations on Favro.",
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
			return jsonResult(nil, &notFoundError{entityType: "collection", identifier: *args.Collection})
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
	return jsonResult(s.boardDetail(client, args.BoardID))
}

func (s *Server) getCurrentBoard(_ context.Context, _ *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	if s.session.Board() == "" {
		return textResult("No board selected. Use the `set_board` tool first.")
	}
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(s.boardDetail(client, s.session.Board()))
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
	return textResult(mdMessage(fmt.Sprintf("Selected board **%s** (%s).", w.Name, w.Type), map[string]any{"widget_common_id": w.WidgetCommonID}))
}

// boardDetail builds the board detail as frontmatter + Markdown body.
func (s *Server) boardDetail(client *favro.Client, widgetCommonID string) (string, error) {
	board, err := client.GetWidget(widgetCommonID)
	if err != nil {
		return "", err
	}
	columns, err := client.GetColumns(widgetCommonID)
	if err != nil {
		return "", err
	}
	sort.SliceStable(columns, func(i, j int) bool { return columns[i].Position < columns[j].Position })

	var b strings.Builder
	fmt.Fprintf(&b, "## Columns\n\n")
	if len(columns) == 0 {
		b.WriteString("(none)\n")
	}
	for i, c := range columns {
		fmt.Fprintf(&b, "%d. **%s** · %s (%d cards)\n", i+1, c.Name, c.ColumnID, c.CardCount)
	}
	if len(board.Lanes) > 0 {
		b.WriteString("\n## Lanes\n\n")
		for _, l := range board.Lanes {
			fmt.Fprintf(&b, "- **%s** · %s\n", l.Name, l.LaneID)
		}
	}
	front := boardDetailFront{ID: board.WidgetCommonID, Name: board.Name, Type: board.Type, Archived: board.Archived}
	return rendered{front: front, body: b.String()}.String(), nil
}

type boardDetailFront struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	Type     string `yaml:"type,omitempty"`
	Archived bool   `yaml:"archived,omitempty"`
}
