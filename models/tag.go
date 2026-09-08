package models

// Tag is a free-form label that can be attached to Tasks via the TaskTag
// join table. Tags are unique by name.
type Tag struct {
	ID string `json:"id"`

	// Name is the unique label text (e.g. "setup", "auth"). Required.
	Name string `json:"name"`

	// Color is an optional hex/CSS color hint for UI rendering.
	Color string `json:"color,omitempty"`

	Description string `json:"description,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
