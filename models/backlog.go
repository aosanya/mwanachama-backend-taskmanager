package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aosanya/mwanachama-backend-shared/plaintext"
)

type BacklogStep struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Statuses []TaskStatus `json:"statuses,omitempty"`
}

const BacklogKindSmart = "smart"

type BacklogRule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"`
}

type Backlog struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Steps    []BacklogStep `json:"steps"`
	Kind     string        `json:"kind,omitempty"`
	Rules    *BacklogRules `json:"rules,omitempty"`
	Archived bool          `json:"archived,omitempty"`
}

type BacklogRules struct {
	Match string        `json:"match"`
	Rules []BacklogRule `json:"rules"`
}

func (b Backlog) Smart() bool { return b.Kind == BacklogKindSmart }

type StepSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Issues int    `json:"issues"`
}

type BacklogSummary struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Issues   int           `json:"issues"`
	Steps    []StepSummary `json:"steps"`
	Level    AccessLevel   `json:"level,omitempty"`
	Kind     string        `json:"kind,omitempty"`
	Rules    *BacklogRules `json:"rules,omitempty"`
	Archived bool          `json:"archived,omitempty"`
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
		if b.Smart() {
			if err := checkSmart(b); err != nil {
				return err
			}
			continue
		}
		if err := checkSteps(b); err != nil {
			return err
		}
	}
	if backlogs[0].Smart() {
		return fmt.Errorf("backlogs: the first backlog keeps steps, a smart backlog cannot be first")
	}
	if backlogs[0].Archived {
		return fmt.Errorf("backlogs: the first backlog takes issues that have no place, it cannot be archived")
	}
	return nil
}

func CheckChangedNames(current, next []Backlog) error {
	for _, b := range next {
		before, existed := backlogNamed(current, b.ID)
		if !existed || before.Name != b.Name {
			if problem := plaintext.Problem("backlog name", b.Name); problem != "" {
				return fmt.Errorf("backlogs: %s", problem)
			}
			if err := checkBacklogNameFree(next, b); err != nil {
				return err
			}
		}
		for _, s := range b.Steps {
			if i, ok := FindStep(before, s.ID); existed && ok && before.Steps[i].Name == s.Name {
				continue
			}
			if problem := plaintext.Problem("step name", s.Name); problem != "" {
				return fmt.Errorf("backlogs: %s", problem)
			}
			if err := checkStepNameFree(b, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkBacklogNameFree(backlogs []Backlog, changed Backlog) error {
	for _, other := range backlogs {
		if other.ID != changed.ID && plaintext.SameName(other.Name, changed.Name) {
			return fmt.Errorf("backlogs: %q is declared twice", changed.Name)
		}
	}
	return nil
}

func checkStepNameFree(b Backlog, changed BacklogStep) error {
	for _, other := range b.Steps {
		if other.ID != changed.ID && plaintext.SameName(other.Name, changed.Name) {
			return fmt.Errorf("backlogs: step %q of %q is declared twice", changed.Name, b.Name)
		}
	}
	return nil
}

func backlogNamed(backlogs []Backlog, id string) (Backlog, bool) {
	if i, ok := FindBacklog(backlogs, id); ok {
		return backlogs[i], true
	}
	return Backlog{}, false
}

func checkSmart(b Backlog) error {
	if len(b.Steps) > 0 {
		return fmt.Errorf("backlogs: smart backlog %q has no steps", b.Name)
	}
	if b.Rules == nil || len(b.Rules.Rules) == 0 {
		return fmt.Errorf("backlogs: smart backlog %q needs at least one rule", b.Name)
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
	if bi, ok := FindBacklog(backlogs, t.Backlog); ok {
		if _, known := FindStep(backlogs[bi], t.Step); known {
			return t.Backlog, t.Step
		}
		return t.Backlog, ""
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

func StepAfterStatusChange(backlogs []Backlog, t Task) (string, string) {
	bi, ok := FindBacklog(backlogs, t.Backlog)
	if !ok {
		return "", ""
	}
	b := backlogs[bi]
	if si, held := FindStep(b, t.Step); held && (len(b.Steps[si].Statuses) == 0 || StepNamesStatus(b.Steps[si], t.Status)) {
		return b.ID, t.Step
	}
	for _, s := range b.Steps {
		if StepNamesStatus(s, t.Status) {
			return b.ID, s.ID
		}
	}
	return b.ID, ""
}

func StepNamesStatus(step BacklogStep, status TaskStatus) bool {
	for _, s := range step.Statuses {
		if s == status {
			return true
		}
	}
	return false
}
