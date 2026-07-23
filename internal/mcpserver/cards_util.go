package mcpserver

import (
	"strings"

	"github.com/lh-etals/favro-mcp/internal/favro"
)

// cardToMap renders a Card as the JSON object returned by card-detail tools.
func cardToMap(c *favro.Card) map[string]any {
	assignments := make([]map[string]any, 0, len(c.Assignments))
	for _, a := range c.Assignments {
		assignments = append(assignments, map[string]any{"user_id": a.UserID, "completed": a.Completed})
	}
	customFields := make([]map[string]any, 0, len(c.CustomFields))
	for _, cf := range c.CustomFields {
		cfMap := map[string]any{
			"custom_field_id": cf.CustomFieldID,
			"value":           cf.Value,
		}
		if cf.Total != nil {
			cfMap["total"] = *cf.Total
		}
		if cf.Link != nil {
			cfMap["link"] = cf.Link
		}
		if cf.Members != nil {
			cfMap["members"] = cf.Members
		}
		if cf.Color != nil {
			cfMap["color"] = *cf.Color
		}
		customFields = append(customFields, cfMap)
	}
	var timeOnBoard any
	if c.TimeOnBoard != nil {
		timeOnBoard = map[string]any{"time": c.TimeOnBoard.Time, "is_stopped": c.TimeOnBoard.IsStopped}
	}
	return map[string]any{
		"card_id":              c.CardID,
		"card_common_id":       c.CardCommonID,
		"sequential_id":        c.SequentialID,
		"name":                 c.Name,
		"detailed_description": strOr(c.DetailedDescription),
		"widget_common_id":     strOr(c.WidgetCommonID),
		"column_id":            strOr(c.ColumnID),
		"lane_id":              strOr(c.LaneID),
		"tags":                 c.Tags,
		"assignments":          assignments,
		"start_date":           strOr(c.StartDate),
		"due_date":             strOr(c.DueDate),
		"archived":             c.Archived,
		"tasks_done":           c.TasksDone,
		"tasks_total":          c.TasksTotal,
		"time_on_board":        timeOnBoard,
		"time_on_columns":      c.TimeOnColumns,
		"custom_fields":        customFields,
	}
}

// stripTasklistFromDescription removes the trailing tasklist checkbox lines that
// Favro auto-appends to a card's detailedDescription.
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
