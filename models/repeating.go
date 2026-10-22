package models

type RepeatingTaskRecord struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Description   string       `json:"description,omitempty"`
	ScheduleRule  string       `json:"schedule_rule,omitempty"`
	ScheduleDates []string     `json:"schedule_dates,omitempty"`
	ScheduleStart string       `json:"schedule_start"`
	ScheduleEnd   string       `json:"schedule_end,omitempty"`
	ScheduleZone  string       `json:"schedule_zone"`
	AssigneeKind  AssigneeKind `json:"assignee_kind"`
	AssigneeID    string       `json:"assignee_id"`
	StoppedAt     string       `json:"stopped_at,omitempty"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
}

type RepeatingTaskPeriod struct {
	PeriodKey       string `json:"period_key"`
	RepeatingTaskID string `json:"repeating_task_id"`
	TaskID          string `json:"task_id"`
	PeriodStart     string `json:"period_start"`
	DueAt           string `json:"due_at"`
	MadeAt          string `json:"made_at"`
}

type RepeatingTaskFilter struct {
	AssigneeKind AssigneeKind `json:"assignee_kind,omitempty"`
	AssigneeID   string       `json:"assignee_id,omitempty"`
}

type Schedule struct {
	Start string   `json:"start"`
	End   string   `json:"end,omitempty"`
	Zone  string   `json:"zone"`
	Rule  string   `json:"rule,omitempty"`
	Dates []string `json:"dates,omitempty"`
}

type NewRepeatingTask struct {
	Title        string       `json:"title"`
	Description  string       `json:"description,omitempty"`
	Schedule     Schedule     `json:"schedule"`
	AssigneeKind AssigneeKind `json:"assignee_kind"`
	AssigneeID   string       `json:"assignee_id"`
}
