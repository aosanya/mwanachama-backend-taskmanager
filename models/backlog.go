package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type BacklogStep struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Statuses []TaskStatus `json:"statuses,omitempty"`
}

type Backlog struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Steps []BacklogStep `json:"steps"`
}

type StepSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Issues int    `json:"issues"`
}

type BacklogSummary struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Issues int           `json:"issues"`
	Steps  []StepSummary `json:"steps"`
}

type BacklogList struct {
	Items []BacklogSummary `json:"items"`
}

func DefaultBacklogs() []Backlog {
	states := DefaultTaskStates()
	steps := make([]BacklogStep, len(states))
	for i, s := range states {
		steps[i] = BacklogStep{ID: Slug(s.Name), Name: s.Name, Statuses: s.Statuses}
	}
	return []Backlog{{ID: "backlog", Name: "Backlog", Steps: steps}}
}

func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func ParseBacklogs(value any) ([]Backlog, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("backlogs: %w", err)
	}
	var backlogs []Backlog
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&backlogs); err != nil {
		return nil, fmt.Errorf("backlogs: %w", err)
	}
	return backlogs, CheckBacklogs(backlogs)
}

func CheckBacklogs(backlogs []Backlog) error {
	if len(backlogs) == 0 {
		return fmt.Errorf("backlogs: a hub keeps at least one backlog")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, b := range backlogs {
		if strings.TrimSpace(b.Name) == "" || b.ID == "" {
			return fmt.Errorf("backlogs: a backlog has no name or id")
		}
		if ids[b.ID] || names[b.Name] {
			return fmt.Errorf("backlogs: %q is declared twice", b.Name)
		}
		ids[b.ID], names[b.Name] = true, true
		if err := checkSteps(b); err != nil {
			return err
		}
	}
	return nil
}

func checkSteps(b Backlog) error {
	if len(b.Steps) == 0 {
		return fmt.Errorf("backlogs: %q needs at least one step", b.Name)
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	held := map[TaskStatus]string{}
	for _, s := range b.Steps {
		if strings.TrimSpace(s.Name) == "" || s.ID == "" {
			return fmt.Errorf("backlogs: a step of %q has no name or id", b.Name)
		}
		if ids[s.ID] || names[s.Name] {
			return fmt.Errorf("backlogs: step %q of %q is declared twice", s.Name, b.Name)
		}
		ids[s.ID], names[s.Name] = true, true
		for _, status := range s.Statuses {
			if !knownTaskStatuses[status] {
				return fmt.Errorf("backlogs: step %q maps %q, which is not a task status", s.Name, status)
			}
			if other, ok := held[status]; ok {
				return fmt.Errorf("backlogs: %q is mapped by both %q and %q in %q", status, other, s.Name, b.Name)
			}
			held[status] = s.Name
		}
	}
	return nil
}

func FindBacklog(backlogs []Backlog, id string) (int, bool) {
	for i, b := range backlogs {
		if b.ID == id {
			return i, true
		}
	}
	return -1, false
}

func FindStep(b Backlog, id string) (int, bool) {
	for i, s := range b.Steps {
		if s.ID == id {
			return i, true
		}
	}
	return -1, false
}

func PlaceOf(backlogs []Backlog, t Task) (string, string) {
	if t.Backlog != "" {
		return t.Backlog, t.Step
	}
	first := backlogs[0]
	for _, s := range first.Steps {
		for _, status := range s.Statuses {
			if status == t.Status {
				return first.ID, s.ID
			}
		}
	}
	return first.ID, ""
}
