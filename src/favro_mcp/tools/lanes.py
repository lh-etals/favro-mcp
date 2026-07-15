"""Lane (swimlane) tools for Favro MCP."""

from typing import Any

from fastmcp import Context

from favro_mcp.context import get_favro_context
from favro_mcp.resolvers import BoardResolver
from favro_mcp.server import mcp


@mcp.tool
def list_lanes(ctx: Context, board: str | None = None) -> dict[str, Any]:
    """List the lanes (swimlanes) on a board.

    Lanes are read-only in the Favro API and cannot be created, renamed, or
    deleted. Use a returned lane_id as the `lane_id` argument to create_card or
    update_card to place a card in a specific lane. A board without lanes
    enabled returns an empty list.

    Args:
        board: The board's widget_common_id, name, or ID.
               Uses the current board if not specified.

    Returns:
        A list of lanes with their IDs and names.
    """
    favro_ctx = get_favro_context(ctx)
    favro_ctx.require_org()
    with favro_ctx.get_client() as client:
        board_id = board or favro_ctx.current_board_id
        if not board_id:
            raise ValueError("No board specified and no current board selected.")
        if board:
            board_id = BoardResolver(client).resolve(board).widget_common_id

        lanes = client.get_lanes(board_id)
        result = [
            {
                "lane_id": lane.lane_id,
                "name": lane.name,
            }
            for lane in lanes
        ]
        return {"lanes": result}
