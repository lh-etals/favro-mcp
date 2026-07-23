package favro

// Models for Favro API responses. JSON tags match the Favro REST API's
// camelCase field names. Datetime-like fields are kept as strings (ISO 8601)
// and passed through unchanged.

type OrganizationMember struct {
	UserID   string  `json:"userId"`
	Role     string  `json:"role"`
	JoinDate *string `json:"joinDate,omitempty"`
}

type Organization struct {
	OrganizationID string                `json:"organizationId"`
	Name           string                `json:"name"`
	Thumbnail      string                `json:"thumbnail,omitempty"`
	SharedToUsers  []OrganizationMember `json:"sharedToUsers,omitempty"`
}

type User struct {
	UserID            string  `json:"userId"`
	Name              string  `json:"name"`
	Email             string  `json:"email"`
	OrganizationRole  *string `json:"organizationRole,omitempty"`
}

type Collection struct {
	CollectionID   string  `json:"collectionId"`
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Archived       bool    `json:"archived"`
	Background     *string `json:"background,omitempty"`
	PublicSharing  *string `json:"publicSharing,omitempty"`
}

type Lane struct {
	LaneID string `json:"laneId"`
	Name   string `json:"name"`
}

type Widget struct {
	WidgetCommonID       string  `json:"widgetCommonId"`
	OrganizationID       string  `json:"organizationId"`
	CollectionIDs        []string `json:"collectionIds,omitempty"`
	Name                 string  `json:"name"`
	Type                 string  `json:"type"`
	Color                *string `json:"color,omitempty"`
	OwnerRole            *string `json:"ownerRole,omitempty"`
	EditRole             *string `json:"editRole,omitempty"`
	Archived             bool    `json:"archived"`
	BreakdownCardCommonID *string `json:"breakdownCardCommonId,omitempty"`
	Lanes                []Lane  `json:"lanes,omitempty"`
}

type Column struct {
	ColumnID       string   `json:"columnId"`
	OrganizationID string   `json:"organizationId"`
	WidgetCommonID string   `json:"widgetCommonId"`
	Name           string   `json:"name"`
	Position       float64  `json:"position"`
	CardCount      int      `json:"cardCount"`
	TimeSum        *int     `json:"timeSum,omitempty"`
	EstimationSum  *float64 `json:"estimationSum,omitempty"`
}

type CardAssignment struct {
	UserID    string `json:"userId"`
	Completed bool   `json:"completed"`
}

type CardCustomField struct {
	CustomFieldID string `json:"customFieldId"`
	Value         any    `json:"value,omitempty"`
	Total         *float64 `json:"total,omitempty"`
	Link          map[string]string `json:"link,omitempty"`
	Members       []string `json:"members,omitempty"`
	Status        *string `json:"status,omitempty"`
	Color         *string `json:"color,omitempty"`
}

type CardTimeOnBoard struct {
	Time     int  `json:"time"`
	IsStopped bool `json:"isStopped"`
}

type Card struct {
	CardID              string             `json:"cardId"`
	OrganizationID      string             `json:"organizationId"`
	CardCommonID        string             `json:"cardCommonId"`
	Name                string             `json:"name"`
	SequentialID        int                `json:"sequentialId"`
	WidgetCommonID      *string            `json:"widgetCommonId,omitempty"`
	ColumnID            *string            `json:"columnId,omitempty"`
	LaneID              *string            `json:"laneId,omitempty"`
	ParentCardID        *string            `json:"parentCardId,omitempty"`
	IsLane              bool               `json:"isLane"`
	Archived            bool               `json:"archived"`
	DetailedDescription *string            `json:"detailedDescription,omitempty"`
	Tags                []string           `json:"tags,omitempty"`
	StartDate           *string            `json:"startDate,omitempty"`
	DueDate             *string            `json:"dueDate,omitempty"`
	Assignments         []CardAssignment   `json:"assignments,omitempty"`
	NumComments         int                `json:"numComments"`
	TasksTotal          int                `json:"tasksTotal"`
	TasksDone           int                `json:"tasksDone"`
	CustomFields        []CardCustomField  `json:"customFields,omitempty"`
	TimeOnBoard         *CardTimeOnBoard   `json:"timeOnBoard,omitempty"`
	TimeOnColumns       map[string]int     `json:"timeOnColumns,omitempty"`
	TodoListUserID      *string            `json:"todoListUserId,omitempty"`
	TodoListCompleted   *bool              `json:"todoListCompleted,omitempty"`
	ListPosition        *float64           `json:"listPosition,omitempty"`
	SheetPosition       *float64           `json:"sheetPosition,omitempty"`
}

type Tag struct {
	TagID          string  `json:"tagId"`
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Color          *string `json:"color,omitempty"`
}

type Comment struct {
	CommentID    string  `json:"commentId"`
	CardCommonID string  `json:"cardCommonId"`
	OrganizationID string `json:"organizationId"`
	UserID       string  `json:"userId"`
	Comment      string  `json:"comment"`
	Created      string  `json:"created"`
	LastUpdated  *string `json:"lastUpdated,omitempty"`
}

type TaskList struct {
	TaskListID    string  `json:"taskListId"`
	OrganizationID string `json:"organizationId"`
	CardCommonID  string  `json:"cardCommonId"`
	Name          string  `json:"name"`
	Position      float64 `json:"position"`
}

type Task struct {
	TaskID        string  `json:"taskId"`
	TaskListID    string  `json:"taskListId"`
	OrganizationID string `json:"organizationId"`
	CardCommonID  string  `json:"cardCommonId"`
	Name          string  `json:"name"`
	Completed     bool    `json:"completed"`
	Position      float64 `json:"position"`
}

type Attachment struct {
	Name    string `json:"name"`
	FileURL string `json:"fileURL"`
}

// CustomField is the raw shape returned by GET /customfields. Fields are left
// loose because the endpoint returns heterogeneous definitions.
type CustomField struct {
	CustomFieldID any    `json:"customFieldId"`
	Name          string `json:"name"`
	Type          string `json:"type"`
}
