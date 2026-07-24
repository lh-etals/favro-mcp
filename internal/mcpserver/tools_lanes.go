package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerLanes(srv *mcp.Server, s *Server) {
	addTool(s, srv, TierRead, &mcp.Tool{
		Name:        "list_lanes",
		Description: "List the lanes (swimlanes) on a board. Lanes are read-only in the Favro API and cannot be created, renamed, or deleted. Use a returned lane_id as the lane argument to create_card/update_card to place a card in a specific lane. A board without lanes enabled returns an empty list on Favro.",
	}, s.listLanes)
}

type listLanesArgs struct {
	Board *string `json:"board,omitempty" jsonschema:"The board's widget_common_id, name, or ID. Uses the current board if not specified"`
}

func (s *Server) listLanes(_ context.Context, _ *mcp.CallToolRequest, args listLanesArgs) (*mcp.CallToolResult, any, error) {
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
	lanes, err := client.GetLanes(boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	out := make([]laneRow, 0, len(lanes))
	for _, l := range lanes {
		out = append(out, laneRow{Name: l.Name, ID: l.LaneID})
	}
	body := fmt.Sprintf("%d lane(s).", len(out))
	if len(out) == 0 {
		body = "This board has no lanes (swimlanes disabled)."
	}
	return textResult(rendered{front: listLanesFront{Lanes: out}, body: body}.String())
}

type listLanesFront struct {
	Lanes []laneRow `yaml:"lanes"`
}

type laneRow struct {
	Name string `yaml:"name"`
	ID   string `yaml:"id"`
}
