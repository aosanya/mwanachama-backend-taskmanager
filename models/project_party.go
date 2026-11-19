package models

type ProjectParty struct {
	ProjectID   string `json:"project_id"`
	Role        string `json:"role"`
	RecordRowID string `json:"record_row_id"`
	PartyName   string `json:"party_name,omitempty"`
	AddedAt     string `json:"added_at"`
}
