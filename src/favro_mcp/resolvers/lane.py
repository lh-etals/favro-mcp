"""Lane resolver (requires board context; lanes have no by-id endpoint)."""

from favro_mcp.api.models import Lane

from .base import AmbiguousMatchError, BaseResolver, NotFoundError


class LaneResolver(BaseResolver[Lane]):
    """Resolver for lanes (swimlanes).

    Lanes are only exposed nested in the widget object, so resolution always
    requires board_id context. There is no GET /lanes/:id endpoint, so both ID
    and name are matched against the board's lanes rather than fetched directly.
    """

    entity_type = "lane"

    def _fetch_all(
        self, board_id: str | None = None, **context: str | None
    ) -> list[Lane]:
        if board_id is None:
            raise ValueError("board_id is required to resolve lanes")
        return self.client.get_lanes(board_id)

    def _fetch_by_id(self, entity_id: str) -> Lane | None:
        # Lanes have no dedicated GET endpoint; resolution happens via resolve().
        return None

    def _get_id(self, entity: Lane) -> str:
        return entity.lane_id

    def _get_name(self, entity: Lane) -> str:
        return entity.name

    def resolve(self, identifier: str, **context: str | None) -> Lane:
        """Resolve a lane by ID or name within a board.

        Args:
            identifier: Lane ID or name
            **context: Must include board_id

        Returns:
            The resolved lane

        Raises:
            NotFoundError: No lane matches
            AmbiguousMatchError: Multiple lanes share the given name
        """
        lanes = self._fetch_all(**context)

        # Exact ID match first (no by-id endpoint, so search the board's lanes)
        for lane in lanes:
            if lane.lane_id == identifier:
                return lane

        # Then case-insensitive name match
        matches = [
            lane for lane in lanes if lane.name.lower() == identifier.lower()
        ]
        if len(matches) == 1:
            return matches[0]
        if len(matches) == 0:
            raise NotFoundError(self.entity_type, identifier)
        match_info = [(lane.lane_id, lane.name) for lane in matches]
        raise AmbiguousMatchError(self.entity_type, identifier, match_info)
