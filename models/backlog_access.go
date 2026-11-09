package models

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type AccessLevel string

const (
	AccessSee  AccessLevel = "see"
	AccessMove AccessLevel = "move"
)

type BacklogGrant struct {
	BacklogID string      `json:"backlog_id"`
	GroupID   string      `json:"group_id"`
	Level     AccessLevel `json:"level"`
}

type GroupAccess struct {
	GroupID string      `json:"group_id"`
	Level   AccessLevel `json:"level"`
	Gone    bool        `json:"gone,omitempty"`
}

type BacklogAccess struct {
	BacklogID  string        `json:"backlog_id"`
	OwnerGroup string        `json:"owner_group,omitempty"`
	Groups     []GroupAccess `json:"groups"`
}

func (l AccessLevel) Valid() bool { return l == AccessSee || l == AccessMove }

func (l AccessLevel) rank() int {
	switch l {
	case AccessMove:
		return 2
	case AccessSee:
		return 1
	}
	return 0
}

func ParseGrants(value any) ([]BacklogGrant, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("backlog access: %w", err)
	}
	var grants []BacklogGrant
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&grants); err != nil {
		return nil, fmt.Errorf("backlog access: %w", err)
	}
	for _, g := range grants {
		if err := CheckGrant(g); err != nil {
			return nil, err
		}
	}
	return grants, nil
}

func CheckGrant(g BacklogGrant) error {
	if g.BacklogID == "" || g.GroupID == "" {
		return fmt.Errorf("backlog access: a grant names a backlog and a group")
	}
	if !g.Level.Valid() {
		return fmt.Errorf("backlog access: level %q is not see or move", g.Level)
	}
	return nil
}

func GrantsFor(grants []BacklogGrant, backlogID string) []GroupAccess {
	out := []GroupAccess{}
	for _, g := range grants {
		if g.BacklogID == backlogID {
			out = append(out, GroupAccess{GroupID: g.GroupID, Level: g.Level})
		}
	}
	return out
}

func WithGrant(grants []BacklogGrant, next BacklogGrant) []BacklogGrant {
	out := make([]BacklogGrant, 0, len(grants)+1)
	replaced := false
	for _, g := range grants {
		if g.BacklogID == next.BacklogID && g.GroupID == next.GroupID {
			if !replaced {
				out = append(out, next)
			}
			replaced = true
			continue
		}
		out = append(out, g)
	}
	if !replaced {
		out = append(out, next)
	}
	return out
}

func WithoutGrant(grants []BacklogGrant, backlogID, groupID string) []BacklogGrant {
	out := make([]BacklogGrant, 0, len(grants))
	for _, g := range grants {
		if g.BacklogID != backlogID || g.GroupID != groupID {
			out = append(out, g)
		}
	}
	return out
}

func GrantsKept(grants []BacklogGrant, backlogs []Backlog) []BacklogGrant {
	out := make([]BacklogGrant, 0, len(grants))
	for _, g := range grants {
		if _, ok := FindBacklog(backlogs, g.BacklogID); ok {
			out = append(out, g)
		}
	}
	return out
}

func AccessLevelOf(grants []BacklogGrant, ownerGroup, backlogID string, groups []string) AccessLevel {
	if ownerGroup == "" && len(grants) == 0 {
		return AccessMove
	}
	held := map[string]bool{}
	for _, id := range groups {
		held[id] = true
	}
	if ownerGroup != "" && held[ownerGroup] {
		return AccessMove
	}
	best := AccessLevel("")
	for _, g := range grants {
		if g.BacklogID == backlogID && held[g.GroupID] && g.Level.rank() > best.rank() {
			best = g.Level
		}
	}
	return best
}
