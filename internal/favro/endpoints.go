package favro

import (
	"net/url"
	"strconv"
)

// Users ---------------------------------------------------------------------

func (c *Client) GetUser(userID string) (*User, error) {
	data, err := c.get("/users/"+userID, nil, false)
	if err != nil {
		return nil, err
	}
	return decodeOne[User](data)
}

func (c *Client) GetUsers() ([]User, error) {
	items, err := c.paginateAll("/users", nil)
	if err != nil {
		return nil, err
	}
	return decodeMany[User](items)
}

// Organizations -------------------------------------------------------------

func (c *Client) GetOrganizations() ([]Organization, error) {
	items, err := c.paginateAll("/organizations", url.Values{})
	if err != nil {
		return nil, err
	}
	return decodeMany[Organization](items)
}

func (c *Client) GetOrganization(orgID string) (*Organization, error) {
	data, err := c.get("/organizations/"+orgID, nil, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Organization](data)
}

// Collections ---------------------------------------------------------------

func (c *Client) GetCollections(archived bool) ([]Collection, error) {
	params := url.Values{"archived": {strconv.FormatBool(archived)}}
	items, err := c.paginateAll("/collections", params)
	if err != nil {
		return nil, err
	}
	return decodeMany[Collection](items)
}

func (c *Client) GetCollection(collectionID string) (*Collection, error) {
	data, err := c.get("/collections/"+collectionID, nil, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Collection](data)
}

// Widgets (boards) ----------------------------------------------------------

func (c *Client) GetWidgets(collectionID string, archived bool) ([]Widget, error) {
	params := url.Values{"archived": {strconv.FormatBool(archived)}}
	if collectionID != "" {
		params.Set("collectionId", collectionID)
	}
	items, err := c.paginateAll("/widgets", params)
	if err != nil {
		return nil, err
	}
	return decodeMany[Widget](items)
}

func (c *Client) GetWidget(widgetCommonID string) (*Widget, error) {
	data, err := c.get("/widgets/"+widgetCommonID, nil, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Widget](data)
}

func (c *Client) GetLanes(widgetCommonID string) ([]Lane, error) {
	w, err := c.GetWidget(widgetCommonID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, nil
	}
	return w.Lanes, nil
}

// Columns -------------------------------------------------------------------

func (c *Client) GetColumns(widgetCommonID string) ([]Column, error) {
	params := url.Values{"widgetCommonId": {widgetCommonID}}
	items, err := c.paginateAll("/columns", params)
	if err != nil {
		return nil, err
	}
	return decodeMany[Column](items)
}

func (c *Client) GetColumn(columnID string) (*Column, error) {
	data, err := c.get("/columns/"+columnID, nil, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Column](data)
}

func (c *Client) CreateColumn(widgetCommonID, name string, position *int) (*Column, error) {
	body := map[string]any{"widgetCommonId": widgetCommonID, "name": name}
	if position != nil {
		body["position"] = *position
	}
	data, err := c.post("/columns", body, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Column](data)
}

func (c *Client) UpdateColumn(columnID string, name *string, position *int) (*Column, error) {
	body := map[string]any{}
	if name != nil {
		body["name"] = *name
	}
	if position != nil {
		body["position"] = *position
	}
	data, err := c.put("/columns/"+columnID, body, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Column](data)
}

func (c *Client) DeleteColumn(columnID string) error {
	_, err := c.del("/columns/"+columnID, nil, true)
	return err
}

// Cards ---------------------------------------------------------------------

type CardFilter struct {
	WidgetCommonID    string
	CollectionID      string
	ColumnID          string
	CardSequentialID  *int
	TodoList          bool
	Unique            bool
	Archived          *bool
}

func (f CardFilter) params() url.Values {
	p := url.Values{"unique": {strconv.FormatBool(f.Unique)}}
	if f.WidgetCommonID != "" {
		p.Set("widgetCommonId", f.WidgetCommonID)
	}
	if f.CollectionID != "" {
		p.Set("collectionId", f.CollectionID)
	}
	if f.ColumnID != "" {
		p.Set("columnId", f.ColumnID)
	}
	if f.CardSequentialID != nil {
		p.Set("cardSequentialId", strconv.Itoa(*f.CardSequentialID))
	}
	if f.TodoList {
		p.Set("todoList", "true")
	}
	if f.Archived != nil {
		p.Set("archived", strconv.FormatBool(*f.Archived))
	}
	return p
}

// retryWithoutMarkdown re-runs fn after dropping the descriptionFormat param,
// matching the Python client's 500-with-markdown fallback.
func retryWithoutMarkdown(get func(useMarkdown bool) ([]map[string]any, error)) ([]map[string]any, error) {
	items, err := get(true)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Status == 500 {
			return get(false)
		}
		return nil, err
	}
	return items, nil
}

func (c *Client) GetCards(f CardFilter) ([]Card, error) {
	items, err := retryWithoutMarkdown(func(useMarkdown bool) ([]map[string]any, error) {
		p := f.params()
		if useMarkdown {
			p.Set("descriptionFormat", "markdown")
		}
		return c.paginateAll("/cards", p)
	})
	if err != nil {
		return nil, err
	}
	return decodeMany[Card](items)
}

func (c *Client) GetCardsPage(f CardFilter, page int) ([]Card, int, error) {
	var items []map[string]any
	var total int
	var err error
	items, total, err = func(useMarkdown bool) ([]map[string]any, int, error) {
		p := f.params()
		if useMarkdown {
			p.Set("descriptionFormat", "markdown")
		}
		return c.paginateSingle("/cards", p, page)
	}(true)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Status == 500 {
			p := f.params()
			items, total, err = c.paginateSingle("/cards", p, page)
		}
	}
	if err != nil {
		return nil, 0, err
	}
	cards, err := decodeMany[Card](items)
	if err != nil {
		return nil, 0, err
	}
	return cards, total, nil
}

func (c *Client) GetCard(cardID string) (*Card, error) {
	get := func(useMarkdown bool) (map[string]any, error) {
		p := url.Values{}
		if useMarkdown {
			p.Set("descriptionFormat", "markdown")
		}
		return c.get("/cards/"+cardID, p, true)
	}
	data, err := get(true)
	if err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Status == 500 {
			data, err = get(false)
		}
	}
	if err != nil {
		return nil, err
	}
	return decodeOne[Card](data)
}

type CreateCardOpts struct {
	Name                string
	WidgetCommonID      string
	ColumnID            string
	LaneID              string
	DetailedDescription *string
	Tags                []string
	StartDate           *string
	DueDate             *string
	Assignments         []string
}

func (c *Client) CreateCard(o CreateCardOpts) (*Card, error) {
	body := map[string]any{"name": o.Name}
	if o.WidgetCommonID != "" {
		body["widgetCommonId"] = o.WidgetCommonID
	}
	if o.ColumnID != "" {
		body["columnId"] = o.ColumnID
	}
	if o.LaneID != "" {
		body["laneId"] = o.LaneID
	}
	if o.DetailedDescription != nil {
		body["detailedDescription"] = *o.DetailedDescription
	}
	if len(o.Tags) > 0 {
		body["addTags"] = o.Tags
	}
	if o.StartDate != nil {
		body["startDate"] = *o.StartDate
	}
	if o.DueDate != nil {
		body["dueDate"] = *o.DueDate
	}
	if len(o.Assignments) > 0 {
		body["addAssignmentIds"] = o.Assignments
	}
	data, err := c.post("/cards", body, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Card](data)
}

type UpdateCardOpts struct {
	CardID            string
	Name              *string
	DetailedDescription *string
	WidgetCommonID    *string
	ColumnID          *string
	LaneID            *string
	DragMode          *string
	AddTags           []string
	RemoveTags        []string
	StartDate         *string
	DueDate           *string
	AddAssignments    []string
	RemoveAssignments []string
	Archived          *bool
	ListPosition      *float64
	CustomFields      []map[string]any
}

func (c *Client) UpdateCard(o UpdateCardOpts) (*Card, error) {
	body := map[string]any{}
	if o.Name != nil {
		body["name"] = *o.Name
	}
	if o.DetailedDescription != nil {
		body["detailedDescription"] = *o.DetailedDescription
	}
	if o.WidgetCommonID != nil {
		body["widgetCommonId"] = *o.WidgetCommonID
	}
	if o.ColumnID != nil {
		body["columnId"] = *o.ColumnID
	}
	if o.LaneID != nil {
		body["laneId"] = *o.LaneID
	}
	if o.DragMode != nil {
		body["dragMode"] = *o.DragMode
	}
	if len(o.AddTags) > 0 {
		body["addTagIds"] = o.AddTags
	}
	if len(o.RemoveTags) > 0 {
		body["removeTagIds"] = o.RemoveTags
	}
	if o.StartDate != nil {
		body["startDate"] = *o.StartDate
	}
	if o.DueDate != nil {
		body["dueDate"] = *o.DueDate
	}
	if len(o.AddAssignments) > 0 {
		body["addAssignmentIds"] = o.AddAssignments
	}
	if len(o.RemoveAssignments) > 0 {
		body["removeAssignmentIds"] = o.RemoveAssignments
	}
	if o.Archived != nil {
		body["archive"] = *o.Archived
	}
	if o.ListPosition != nil {
		body["listPosition"] = *o.ListPosition
	}
	if len(o.CustomFields) > 0 {
		body["customFields"] = o.CustomFields
	}
	data, err := c.put("/cards/"+o.CardID, body, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Card](data)
}

func (c *Client) DeleteCard(cardID string, everywhere bool) error {
	params := url.Values{}
	if everywhere {
		params.Set("everywhere", "true")
	}
	_, err := c.del("/cards/"+cardID, params, true)
	return err
}

// Attachments ---------------------------------------------------------------

func (c *Client) UploadAttachment(cardID, filename string, content []byte) (*Attachment, error) {
	if len(content) > 10*1024*1024 {
		return nil, &APIError{Status: 0, Message: "file size exceeds 10 MB limit"}
	}
	params := url.Values{"filename": {filename}}
	data, err := c.postBinary("/cards/"+cardID+"/attachment", content, params, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Attachment](data)
}

// Tags ----------------------------------------------------------------------

func (c *Client) GetTags() ([]Tag, error) {
	items, err := c.paginateAll("/tags", nil)
	if err != nil {
		return nil, err
	}
	return decodeMany[Tag](items)
}

func (c *Client) GetTag(tagID string) (*Tag, error) {
	data, err := c.get("/tags/"+tagID, nil, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Tag](data)
}

// Comments ------------------------------------------------------------------

func (c *Client) GetComments(cardCommonID string) ([]Comment, error) {
	params := url.Values{"cardCommonId": {cardCommonID}}
	items, err := c.paginateAll("/comments", params)
	if err != nil {
		return nil, err
	}
	return decodeMany[Comment](items)
}

func (c *Client) CreateComment(cardCommonID, comment string) (*Comment, error) {
	body := map[string]any{"cardCommonId": cardCommonID, "comment": comment}
	data, err := c.post("/comments", body, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Comment](data)
}

// Custom fields -------------------------------------------------------------

func (c *Client) GetCustomFields() ([]map[string]any, error) {
	return c.paginateAll("/customfields", nil)
}

// Task lists ----------------------------------------------------------------

func (c *Client) GetTasklists(cardCommonID string) ([]TaskList, error) {
	params := url.Values{"cardCommonId": {cardCommonID}}
	items, err := c.paginateAll("/tasklists", params)
	if err != nil {
		return nil, err
	}
	return decodeMany[TaskList](items)
}

func (c *Client) CreateTasklist(cardCommonID, name string, position *int) (*TaskList, error) {
	body := map[string]any{"cardCommonId": cardCommonID, "name": name}
	if position != nil {
		body["position"] = *position
	}
	data, err := c.post("/tasklists", body, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[TaskList](data)
}

// Tasks ---------------------------------------------------------------------

func (c *Client) GetTasks(cardCommonID, tasklistID string) ([]Task, error) {
	params := url.Values{"cardCommonId": {cardCommonID}}
	if tasklistID != "" {
		params.Set("taskListId", tasklistID)
	}
	items, err := c.paginateAll("/tasks", params)
	if err != nil {
		return nil, err
	}
	return decodeMany[Task](items)
}

func (c *Client) CreateTask(tasklistID, name string, position *int) (*Task, error) {
	body := map[string]any{"taskListId": tasklistID, "name": name}
	if position != nil {
		body["position"] = *position
	}
	data, err := c.post("/tasks", body, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Task](data)
}

func (c *Client) UpdateTask(taskID string, name *string, completed *bool, position *int) (*Task, error) {
	body := map[string]any{}
	if name != nil {
		body["name"] = *name
	}
	if completed != nil {
		body["completed"] = *completed
	}
	if position != nil {
		body["position"] = *position
	}
	data, err := c.put("/tasks/"+taskID, body, true)
	if err != nil {
		return nil, err
	}
	return decodeOne[Task](data)
}
