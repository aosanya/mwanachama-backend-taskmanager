package models

type OverdueFilter struct {
	AsOf  string `query:"as_of"`
	Limit int    `query:"limit"`
}

type OverdueTask struct {
	Task
	OverdueHours int `json:"overdue_hours"`
}
