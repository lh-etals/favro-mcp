package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lh-etals/favro-mcp/internal/favro"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxAttachmentBytes = 10 << 20 // 10 MB Favro attachment limit

func registerCards(srv *mcp.Server, s *Server) {
	// read
	addTool(s, srv, TierRead, &mcp.Tool{Name: "list_cards", Description: "List cards on a specific board with pagination. Each page returns up to 100 cards."}, s.listCards)
	addTool(s, srv, TierRead, &mcp.Tool{Name: "list_custom_fields", Description: "List custom field definitions in the organization. Use the customFieldId when updating card custom fields."}, s.listCustomFields)
	addTool(s, srv, TierRead, &mcp.Tool{Name: "get_card_details", Description: "Get detailed information about a card: description, assignments, dates, custom fields, task lists with tasks, and comments."}, s.getCardDetails)
	// write
	addTool(s, srv, TierWrite, &mcp.Tool{Name: "add_comment", Description: "Add a comment to a card."}, s.addComment)
	addTool(s, srv, TierWrite, &mcp.Tool{Name: "create_card", Description: "Create a new card on a board. The board defaults to the current board. Markdown description, tags, and assignees are optional."}, s.createCard)
	addTool(s, srv, TierWrite, &mcp.Tool{Name: "update_card", Description: "Update a card's properties: name, markdown description, lane, archive state, custom fields, and task/checklist items."}, s.updateCard)
	addTool(s, srv, TierWrite, &mcp.Tool{Name: "move_card", Description: "Move a card to a different column and/or lane, optionally to another board. Uses drag mode 'move' so a cross-board move relocates the card rather than copying it."}, s.moveCard)
	addTool(s, srv, TierWrite, &mcp.Tool{Name: "assign_card", Description: "Assign or unassign a user (by ID, name, or email) from a card."}, s.assignCard)
	addTool(s, srv, TierWrite, &mcp.Tool{Name: "tag_card", Description: "Add or remove a tag (by ID or name) from a card."}, s.tagCard)
	addTool(s, srv, TierWrite, &mcp.Tool{Name: "upload_attachment", Description: "Upload a file attachment (max 10 MB) to a card."}, s.uploadAttachment)
	// delete
	addTool(s, srv, TierDelete, &mcp.Tool{Name: "delete_card", Description: "Delete a card. Set everywhere=true to delete it from all boards."}, s.deleteCard)
	addTool(s, srv, TierDelete, &mcp.Tool{Name: "delete_comment", Description: "Delete a comment from a card by comment ID (undo for add_comment)."}, s.deleteComment)
	addTool(s, srv, TierDelete, &mcp.Tool{Name: "delete_task", Description: "Delete a task (checklist item) by task ID (undo for update_card's add_task)."}, s.deleteTask)
	addTool(s, srv, TierDelete, &mcp.Tool{Name: "delete_tasklist", Description: "Delete a task list (checklist) by tasklist ID (undo for update_card's add_tasklist)."}, s.deleteTasklist)
	addTool(s, srv, TierDelete, &mcp.Tool{Name: "remove_attachment", Description: "Remove a file attachment from a card by its file URL (undo for upload_attachment)."}, s.removeAttachment)
}

// --- list_cards ------------------------------------------------------------

type listCardsArgs struct {
	Board    string  `json:"board" jsonschema:"The board's widget_common_id, name, or ID"`
	Column   *string `json:"column,omitempty" jsonschema:"Optional column ID or name to filter by"`
	Archived *bool   `json:"archived,omitempty" jsonschema:"Filter by archived status: true=only archived, false=only non-archived, omit=all"`
	Query    *string `json:"query,omitempty" jsonschema:"Case-insensitive substring filter on card name (client-side search)"`
	Page     int     `json:"page,omitempty" jsonschema:"Page number (0-indexed, default 0)"`
}

func (s *Server) listCards(_ context.Context, _ *mcp.CallToolRequest, args listCardsArgs) (*mcp.CallToolResult, any, error) {
	if args.Page < 0 {
		return jsonResult(nil, fmt.Errorf("page must be >= 0"))
	}
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
	f := favro.CardFilter{WidgetCommonID: boardID.WidgetCommonID, Unique: true}
	if args.Column != nil && *args.Column != "" {
		col, err := NewResolver(client).Column(*args.Column, boardID.WidgetCommonID)
		if err != nil {
			return jsonResult(nil, err)
		}
		f.ColumnID = col.ColumnID
	}
	f.Archived = args.Archived
	cards, totalPages, err := client.GetCardsPage(f, args.Page)
	if err != nil {
		return jsonResult(nil, err)
	}
	// Client-side name search (Favro has no server-side text search).
	query := ""
	if args.Query != nil {
		query = strings.ToLower(strings.TrimSpace(*args.Query))
	}
	rows := make([]cardRow, 0, len(cards))
	for _, c := range cards {
		if query != "" && !strings.Contains(strings.ToLower(c.Name), query) {
			continue
		}
		colID := strOr(c.ColumnID)
		rows = append(rows, cardRow{
			Seq: c.SequentialID, Name: c.Name, ID: c.CardID,
			Column: colID, Tags: c.Tags, Archived: c.Archived,
		})
	}
	front := map[string]any{
		"board": boardID.Name,
		"page":  args.Page,
		"pages": totalPages,
		"cards": rows,
	}
	if query != "" {
		front["query"] = *args.Query
	}
	body := fmt.Sprintf("%d card(s) (page %d/%d).", len(rows), args.Page, totalPages)
	if totalPages > 1 {
		body += fmt.Sprintf(" Next: page=%d.", args.Page+1)
	}
	return textResult(rendered{front: front, body: body}.String())
}

// cardRow is one row of list_cards output.
type cardRow struct {
	Seq     int      `yaml:"seq"`
	Name    string   `yaml:"name"`
	ID      string   `yaml:"id"`
	Column  string   `yaml:"column,omitempty"`
	Tags    []string `yaml:"tags,omitempty"`
	Archived bool    `yaml:"archived,omitempty"`
}

// --- list_custom_fields ----------------------------------------------------

type listCustomFieldsArgs struct {
	Name      *string `json:"name,omitempty" jsonschema:"Filter by name (case-insensitive substring match)"`
	FieldType *string `json:"field_type,omitempty" jsonschema:"Filter by type (e.g. Link, Text, Rating, Single select)"`
}

func (s *Server) listCustomFields(_ context.Context, _ *mcp.CallToolRequest, args listCustomFieldsArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	fields, err := client.GetCustomFields()
	if err != nil {
		return jsonResult(nil, err)
	}
	out := make([]map[string]any, 0, len(fields))
	for _, f := range fields {
		name, _ := f["name"].(string)
		ftype, _ := f["type"].(string)
		if args.Name != nil && *args.Name != "" {
			if !strings.Contains(strings.ToLower(name), strings.ToLower(*args.Name)) {
				continue
			}
		}
		if args.FieldType != nil && *args.FieldType != "" {
			if !strings.EqualFold(ftype, *args.FieldType) {
				continue
			}
		}
		out = append(out, map[string]any{
			"customFieldId": f["customFieldId"],
			"name":          name,
			"type":          ftype,
		})
	}
	return jsonResult(map[string]any{"custom_fields": out, "count": len(out)}, nil)
}

// --- get_card_details ------------------------------------------------------

type getCardDetailsArgs struct {
	Card  string  `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	Board *string `json:"board,omitempty" jsonschema:"Board ID or name (needed for name lookups)"`
}

func (s *Server) getCardDetails(_ context.Context, _ *mcp.CallToolRequest, args getCardDetailsArgs) (*mcp.CallToolResult, any, error) {
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
	c, err := NewResolver(client).Card(args.Card, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}

	// Task lists with their tasks.
	tasklists, err := client.GetTasklists(c.CardCommonID)
	if err != nil {
		return jsonResult(nil, err)
	}
	tasklistsData := make([]map[string]any, 0, len(tasklists))
	for _, tl := range tasklists {
		tasks, err := client.GetTasks(c.CardCommonID, tl.TaskListID)
		if err != nil {
			return jsonResult(nil, err)
		}
		taskRows := make([]map[string]any, 0, len(tasks))
		for _, t := range tasks {
			taskRows = append(taskRows, map[string]any{
				"task_id":    t.TaskID,
				"name":       t.Name,
				"completed":  t.Completed,
				"position":   t.Position,
			})
		}
		tasklistsData = append(tasklistsData, map[string]any{
			"tasklist_id": tl.TaskListID,
			"name":        tl.Name,
			"position":    tl.Position,
			"tasks":       taskRows,
		})
	}

	// Comments.
	comments, err := client.GetComments(c.CardCommonID)
	if err != nil {
		return jsonResult(nil, err)
	}
	commentsData := make([]map[string]any, 0, len(comments))
	for _, cm := range comments {
		lastUpd := ""
		if cm.LastUpdated != nil {
			lastUpd = *cm.LastUpdated
		}
		commentsData = append(commentsData, map[string]any{
			"comment_id":    cm.CommentID,
			"user_id":       cm.UserID,
			"comment":       cm.Comment,
			"created":       cm.Created,
			"last_updated":  lastUpd,
		})
	}

	result := cardToMap(c)
	result["tasklists"] = tasklistsData
	result["comments"] = commentsData
	result["detailed_description"] = stripTasklistFromDescription(strOr(c.DetailedDescription), tasklistsData)
	return jsonResult(result, nil)
}

// --- add_comment -----------------------------------------------------------

type addCommentArgs struct {
	Card    string  `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	Comment string  `json:"comment" jsonschema:"Comment text to post"`
	Board   *string `json:"board,omitempty" jsonschema:"Board ID or name (needed for name lookup; optional for sequential ID)"`
}

func (s *Server) addComment(_ context.Context, _ *mcp.CallToolRequest, args addCommentArgs) (*mcp.CallToolResult, any, error) {
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
	c, err := NewResolver(client).Card(args.Card, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	created, err := client.CreateComment(c.CardCommonID, args.Comment)
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":         "Comment added",
		"comment_id":      created.CommentID,
		"card_common_id":  created.CardCommonID,
		"user_id":         created.UserID,
		"created":         created.Created,
	}, nil)
}

// --- create_card -----------------------------------------------------------

type createCardArgs struct {
	Name        string    `json:"name" jsonschema:"Card name/title"`
	Board       *string   `json:"board,omitempty" jsonschema:"Board ID or name (uses current board if not specified)"`
	Column      *string   `json:"column,omitempty" jsonschema:"Column ID or name to place the card in"`
	Lane        *string   `json:"lane,omitempty" jsonschema:"Lane (swimlane) ID or name. Only applies to boards with lanes enabled; use list_lanes to see them"`
	Description *string   `json:"description,omitempty" jsonschema:"Detailed description (Favro supports a subset of Markdown)"`
	Tags        *[]string `json:"tags,omitempty" jsonschema:"List of tag IDs or names to add"`
	Assignees   *[]string `json:"assignees,omitempty" jsonschema:"List of user IDs, names, or emails to assign"`
}

func (s *Server) createCard(_ context.Context, _ *mcp.CallToolRequest, args createCardArgs) (*mcp.CallToolResult, any, error) {
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

	var columnID string
	if args.Column != nil && *args.Column != "" {
		col, err := NewResolver(client).Column(*args.Column, boardID)
		if err != nil {
			return jsonResult(nil, err)
		}
		columnID = col.ColumnID
	}
	var laneID string
	if args.Lane != nil && *args.Lane != "" {
		ln, err := NewResolver(client).Lane(*args.Lane, boardID)
		if err != nil {
			return jsonResult(nil, err)
		}
		laneID = ln.LaneID
	}
	var tagIDs []string
	if args.Tags != nil && len(*args.Tags) > 0 {
		tagIDs = make([]string, 0, len(*args.Tags))
		tr := NewResolver(client)
		for _, t := range *args.Tags {
			tag, err := tr.Tag(t)
			if err != nil {
				return jsonResult(nil, err)
			}
			tagIDs = append(tagIDs, tag.TagID)
		}
	}
	var userIDs []string
	if args.Assignees != nil && len(*args.Assignees) > 0 {
		userIDs = make([]string, 0, len(*args.Assignees))
		ur := NewResolver(client)
		for _, u := range *args.Assignees {
			user, err := ur.User(u)
			if err != nil {
				return jsonResult(nil, err)
			}
			userIDs = append(userIDs, user.UserID)
		}
	}

	// Prime the description field with a space when content is provided: Favro
	// only parses markdown on update if the field already has content, and a
	// board template may overwrite the description sent at creation.
	var primed *string
	if args.Description != nil {
		space := " "
		primed = &space
	}
	card, err := client.CreateCard(favro.CreateCardOpts{
		Name: args.Name, WidgetCommonID: boardID, ColumnID: columnID, LaneID: laneID,
		DetailedDescription: primed, Tags: tagIDs, Assignments: userIDs,
	})
	if err != nil {
		return jsonResult(nil, err)
	}
	if args.Description != nil {
		card, err = client.UpdateCard(favro.UpdateCardOpts{CardID: card.CardID, DetailedDescription: args.Description})
		if err != nil {
			return jsonResult(nil, err)
		}
	}
	prose := fmt.Sprintf("Created card **#%d %s**.", card.SequentialID, card.Name)
	return textResult(mdMessage(prose, map[string]any{
		"card_id":        card.CardID,
		"card_common_id": card.CardCommonID,
		"sequential_id":  card.SequentialID,
		"board":          boardID,
	}))
}

// --- update_card -----------------------------------------------------------

type taskUpdateSpec struct {
	TaskID    string  `json:"task_id"`
	Name      *string `json:"name,omitempty"`
	Completed *bool   `json:"completed,omitempty"`
}

type addTaskSpec struct {
	TaskListID string `json:"tasklist_id"`
	Name       string `json:"name"`
}

type updateCardArgs struct {
	Card         string              `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	Board        *string             `json:"board,omitempty" jsonschema:"Board ID or name (needed for sequential ID or name lookup)"`
	Name         *string             `json:"name,omitempty" jsonschema:"New card name"`
	Description  *string             `json:"description,omitempty" jsonschema:"New detailed description (Favro supports a subset of Markdown)"`
	Lane         *string             `json:"lane,omitempty" jsonschema:"Lane (swimlane) ID or name to move the card into"`
	Archived     *bool               `json:"archived,omitempty" jsonschema:"Archive or unarchive the card"`
	CustomFields *[]map[string]any   `json:"custom_fields,omitempty" jsonschema:"List of custom field updates, each with customFieldId and a value field"`
	Tasks        *[]taskUpdateSpec   `json:"tasks,omitempty" jsonschema:"List of task updates, each with task_id and optionally name/completed"`
	AddTasklist  *string             `json:"add_tasklist,omitempty" jsonschema:"Create a new checklist (task list) on this card with this name"`
	AddTask      *addTaskSpec        `json:"add_task,omitempty" jsonschema:"Add a task to an existing tasklist: {tasklist_id, name}"`
}

func (s *Server) updateCard(_ context.Context, _ *mcp.CallToolRequest, args updateCardArgs) (*mcp.CallToolResult, any, error) {
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
	c, err := NewResolver(client).Card(args.Card, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}

	var laneID *string
	if args.Lane != nil && *args.Lane != "" {
		laneBoard := boardID
		if laneBoard == "" && c.WidgetCommonID != nil {
			laneBoard = *c.WidgetCommonID
		}
		if laneBoard == "" {
			return jsonResult(nil, fmt.Errorf("board required to resolve lane"))
		}
		ln, err := NewResolver(client).Lane(*args.Lane, laneBoard)
		if err != nil {
			return jsonResult(nil, err)
		}
		id := ln.LaneID
		laneID = &id
	}

	// Prime an empty description field so Favro parses markdown on update.
	if args.Description != nil {
		existing, err := client.GetCard(c.CardID)
		if err != nil {
			return jsonResult(nil, err)
		}
		if existing.DetailedDescription == nil || strings.TrimSpace(*existing.DetailedDescription) == "" {
			space := " "
			if _, err := client.UpdateCard(favro.UpdateCardOpts{CardID: c.CardID, DetailedDescription: &space}); err != nil {
				return jsonResult(nil, err)
			}
		}
	}

	var customFields []map[string]any
	if args.CustomFields != nil {
		customFields = *args.CustomFields
	}
	updated, err := client.UpdateCard(favro.UpdateCardOpts{
		CardID: c.CardID, Name: args.Name, DetailedDescription: args.Description,
		LaneID: laneID, Archived: args.Archived, CustomFields: customFields,
	})
	if err != nil {
		return jsonResult(nil, err)
	}

	messages := []string{"Updated card: " + updated.Name}

	if args.Tasks != nil {
		for _, t := range *args.Tasks {
			if t.TaskID == "" {
				continue
			}
			if _, err := client.UpdateTask(t.TaskID, t.Name, t.Completed, nil); err != nil {
				return jsonResult(nil, err)
			}
		}
		messages = append(messages, fmt.Sprintf("Updated %d task(s)", len(*args.Tasks)))
	}
	if args.AddTasklist != nil && *args.AddTasklist != "" {
		newTL, err := client.CreateTasklist(c.CardCommonID, *args.AddTasklist, nil)
		if err != nil {
			return jsonResult(nil, err)
		}
		messages = append(messages, "Created task list: "+newTL.Name)
	}
	if args.AddTask != nil && args.AddTask.TaskListID != "" && args.AddTask.Name != "" {
		newTask, err := client.CreateTask(args.AddTask.TaskListID, args.AddTask.Name, nil)
		if err != nil {
			return jsonResult(nil, err)
		}
		messages = append(messages, "Created task: "+newTask.Name)
	}

	return jsonResult(map[string]any{
		"message":      strings.Join(messages, "; "),
		"card_id":      updated.CardID,
		"sequential_id": updated.SequentialID,
		"name":         updated.Name,
	}, nil)
}

// --- move_card -------------------------------------------------------------

type moveCardArgs struct {
	Card    string  `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	Column  *string `json:"column,omitempty" jsonschema:"Target column ID or name (on to_board if given, else on the card's current board)"`
	Lane    *string `json:"lane,omitempty" jsonschema:"Target lane (swimlane) ID or name"`
	Board   *string `json:"board,omitempty" jsonschema:"Source board ID or name - where the card currently lives"`
	ToBoard *string `json:"to_board,omitempty" jsonschema:"Destination board ID or name. Omit to move within the same board"`
}

func (s *Server) moveCard(_ context.Context, _ *mcp.CallToolRequest, args moveCardArgs) (*mcp.CallToolResult, any, error) {
	if (args.Column == nil || *args.Column == "") && (args.Lane == nil || *args.Lane == "") {
		return jsonResult(nil, fmt.Errorf("specify a column and/or a lane to move the card"))
	}
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	sourceBoard, err := s.resolveBoardArg(client, strOr(args.Board))
	if err != nil {
		return jsonResult(nil, err)
	}
	c, err := NewResolver(client).Card(args.Card, sourceBoard)
	if err != nil {
		return jsonResult(nil, err)
	}
	// Ensure we resolved the instance on the intended source board.
	if sourceBoard != "" && c.WidgetCommonID != nil && *c.WidgetCommonID != sourceBoard {
		return jsonResult(nil, fmt.Errorf(
			"card '%s' resolved to board %s, not the source board %s. Pass 'board' as the board the card currently lives on.",
			args.Card, *c.WidgetCommonID, sourceBoard))
	}

	targetBoard := sourceBoard
	if targetBoard == "" && c.WidgetCommonID != nil {
		targetBoard = *c.WidgetCommonID
	}
	if args.ToBoard != nil && *args.ToBoard != "" {
		tb, err := NewResolver(client).Board(*args.ToBoard)
		if err != nil {
			return jsonResult(nil, err)
		}
		targetBoard = tb.WidgetCommonID
	}
	if targetBoard == "" {
		return jsonResult(nil, fmt.Errorf("board ID required to resolve the target column/lane"))
	}

	var col *favro.Column
	if args.Column != nil && *args.Column != "" {
		col, err = NewResolver(client).Column(*args.Column, targetBoard)
		if err != nil {
			return jsonResult(nil, err)
		}
	}
	var ln *favro.Lane
	if args.Lane != nil && *args.Lane != "" {
		ln, err = NewResolver(client).Lane(*args.Lane, targetBoard)
		if err != nil {
			return jsonResult(nil, err)
		}
	}

	cardBoard := ""
	if c.WidgetCommonID != nil {
		cardBoard = *c.WidgetCommonID
	}
	// Only treat this as a cross-board move when we actually know the card's
	// board and it differs from the target. A nil WidgetCommonID must not imply
	// a cross-board move (it would spuriously set dragMode="move").
	crossBoard := cardBoard != "" && targetBoard != cardBoard
	var dragMode *string
	if crossBoard {
		m := "move"
		dragMode = &m
	}
	var colID *string
	if col != nil {
		colID = &col.ColumnID
	}
	var laneID *string
	if ln != nil {
		laneID = &ln.LaneID
	}
	tb := targetBoard
	updated, err := client.UpdateCard(favro.UpdateCardOpts{
		CardID: c.CardID, ColumnID: colID, LaneID: laneID,
		WidgetCommonID: &tb, DragMode: dragMode,
	})
	if err != nil {
		return jsonResult(nil, err)
	}

	var dests []string
	if col != nil {
		dests = append(dests, fmt.Sprintf("column '%s'", col.Name))
	}
	if ln != nil {
		dests = append(dests, fmt.Sprintf("lane '%s'", ln.Name))
	}
	location := strings.Join(dests, " and ")
	if crossBoard {
		location = fmt.Sprintf("board %s %s", targetBoard, location)
	}

	colIDVal, colNameVal := any(nil), any(nil)
	if col != nil {
		colIDVal = col.ColumnID
		colNameVal = col.Name
	}
	laneIDVal, laneNameVal := any(nil), any(nil)
	if ln != nil {
		laneIDVal = ln.LaneID
		laneNameVal = ln.Name
	}
	result := map[string]any{
		"message":          fmt.Sprintf("Moved card '%s' to %s", updated.Name, location),
		"card_id":          updated.CardID,
		"widget_common_id": targetBoard,
		"column_id":        colIDVal,
		"column_name":      colNameVal,
		"lane_id":          laneIDVal,
		"lane_name":        laneNameVal,
	}
	return jsonResult(result, nil)
}

// --- assign_card -----------------------------------------------------------

type assignCardArgs struct {
	Card   string  `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	User   string  `json:"user" jsonschema:"User ID, name, or email"`
	Board  *string `json:"board,omitempty" jsonschema:"Board ID or name (needed for name lookups)"`
	Remove bool    `json:"remove,omitempty" jsonschema:"If true, remove the assignment instead of adding"`
}

func (s *Server) assignCard(_ context.Context, _ *mcp.CallToolRequest, args assignCardArgs) (*mcp.CallToolResult, any, error) {
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
	c, err := NewResolver(client).Card(args.Card, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	u, err := NewResolver(client).User(args.User)
	if err != nil {
		return jsonResult(nil, err)
	}

	var updated *favro.Card
	var action, prep string
	if args.Remove {
		updated, err = client.UpdateCard(favro.UpdateCardOpts{CardID: c.CardID, RemoveAssignments: []string{u.UserID}})
		action, prep = "Unassigned", "from"
	} else {
		updated, err = client.UpdateCard(favro.UpdateCardOpts{CardID: c.CardID, AddAssignments: []string{u.UserID}})
		action, prep = "Assigned", "to"
	}
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":   fmt.Sprintf("%s %s %s card '%s'", action, u.Name, prep, updated.Name),
		"card_id":   updated.CardID,
		"user_id":   u.UserID,
		"user_name": u.Name,
	}, nil)
}

// --- tag_card --------------------------------------------------------------

type tagCardArgs struct {
	Card   string  `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	Tag    string  `json:"tag" jsonschema:"Tag ID or name"`
	Board  *string `json:"board,omitempty" jsonschema:"Board ID or name (needed for name lookups)"`
	Remove bool    `json:"remove,omitempty" jsonschema:"If true, remove the tag instead of adding"`
}

func (s *Server) tagCard(_ context.Context, _ *mcp.CallToolRequest, args tagCardArgs) (*mcp.CallToolResult, any, error) {
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
	c, err := NewResolver(client).Card(args.Card, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	t, err := NewResolver(client).Tag(args.Tag)
	if err != nil {
		return jsonResult(nil, err)
	}

	var updated *favro.Card
	var action, prep string
	if args.Remove {
		updated, err = client.UpdateCard(favro.UpdateCardOpts{CardID: c.CardID, RemoveTags: []string{t.TagID}})
		action, prep = "Removed", "from"
	} else {
		updated, err = client.UpdateCard(favro.UpdateCardOpts{CardID: c.CardID, AddTags: []string{t.TagID}})
		action, prep = "Added", "to"
	}
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":   fmt.Sprintf("%s tag '%s' %s card '%s'", action, t.Name, prep, updated.Name),
		"card_id":   updated.CardID,
		"tag_id":    t.TagID,
		"tag_name":  t.Name,
	}, nil)
}

// --- delete_card -----------------------------------------------------------

type deleteCardArgs struct {
	Card      string  `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	Board     *string `json:"board,omitempty" jsonschema:"Board ID or name (needed for name lookups)"`
	Everywhere bool   `json:"everywhere,omitempty" jsonschema:"If true, delete from all boards (not just current)"`
}

func (s *Server) deleteCard(_ context.Context, _ *mcp.CallToolRequest, args deleteCardArgs) (*mcp.CallToolResult, any, error) {
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
	c, err := NewResolver(client).Card(args.Card, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	name := c.Name
	cardID := c.CardID
	if err := client.DeleteCard(cardID, args.Everywhere); err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message": "Deleted card: " + name,
		"card_id": cardID,
	}, nil)
}

// --- upload_attachment -----------------------------------------------------

type uploadAttachmentArgs struct {
	Card     string  `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	FilePath string  `json:"file_path" jsonschema:"Absolute path to the file to upload (max 10 MB)"`
	Board    *string `json:"board,omitempty" jsonschema:"Board ID or name (needed for name lookups)"`
}

func (s *Server) uploadAttachment(_ context.Context, _ *mcp.CallToolRequest, args uploadAttachmentArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	info, err := os.Stat(args.FilePath)
	if err != nil || info.IsDir() {
		return jsonResult(nil, fmt.Errorf("file not found: %s", args.FilePath))
	}
	if info.Size() > maxAttachmentBytes {
		return jsonResult(nil, fmt.Errorf("file exceeds 10 MB limit (%d bytes)", info.Size()))
	}
	content, err := os.ReadFile(args.FilePath)
	if err != nil {
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
	c, err := NewResolver(client).Card(args.Card, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	att, err := client.UploadAttachment(c.CardID, filepath.Base(args.FilePath), content)
	if err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":  fmt.Sprintf("Uploaded '%s' to card '%s'", att.Name, c.Name),
		"name":     att.Name,
		"file_url": att.FileURL,
		"card_id":  c.CardID,
	}, nil)
}

// --- delete_comment --------------------------------------------------------

type deleteCommentArgs struct {
	CommentID string `json:"comment_id" jsonschema:"The comment ID (from get_card_details)"`
}

func (s *Server) deleteComment(_ context.Context, _ *mcp.CallToolRequest, args deleteCommentArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	if err := client.DeleteComment(args.CommentID); err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{"message": "Deleted comment: " + args.CommentID, "comment_id": args.CommentID}, nil)
}

// --- delete_task -----------------------------------------------------------

type deleteTaskArgs struct {
	TaskID string `json:"task_id" jsonschema:"The task (checklist item) ID"`
}

func (s *Server) deleteTask(_ context.Context, _ *mcp.CallToolRequest, args deleteTaskArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	if err := client.DeleteTask(args.TaskID); err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{"message": "Deleted task: " + args.TaskID, "task_id": args.TaskID}, nil)
}

// --- delete_tasklist -------------------------------------------------------

type deleteTasklistArgs struct {
	TasklistID string `json:"tasklist_id" jsonschema:"The task list (checklist) ID"`
}

func (s *Server) deleteTasklist(_ context.Context, _ *mcp.CallToolRequest, args deleteTasklistArgs) (*mcp.CallToolResult, any, error) {
	if _, err := s.requireOrg(); err != nil {
		return jsonResult(nil, err)
	}
	client, err := s.client()
	if err != nil {
		return jsonResult(nil, err)
	}
	if err := client.DeleteTasklist(args.TasklistID); err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{"message": "Deleted task list: " + args.TasklistID, "tasklist_id": args.TasklistID}, nil)
}

// --- remove_attachment -----------------------------------------------------

type removeAttachmentArgs struct {
	Card    string  `json:"card" jsonschema:"Card ID, sequential ID (#123), or name"`
	FileURL string  `json:"file_url" jsonschema:"The attachment file URL (from upload_attachment / get_card_details)"`
	Board   *string `json:"board,omitempty" jsonschema:"Board ID or name (needed for name lookups)"`
}

func (s *Server) removeAttachment(_ context.Context, _ *mcp.CallToolRequest, args removeAttachmentArgs) (*mcp.CallToolResult, any, error) {
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
	c, err := NewResolver(client).Card(args.Card, boardID)
	if err != nil {
		return jsonResult(nil, err)
	}
	if _, err := client.UpdateCard(favro.UpdateCardOpts{CardID: c.CardID, RemoveAttachments: []string{args.FileURL}}); err != nil {
		return jsonResult(nil, err)
	}
	return jsonResult(map[string]any{
		"message":  fmt.Sprintf("Removed attachment from card '%s'", c.Name),
		"card_id":  c.CardID,
		"file_url": args.FileURL,
	}, nil)
}
