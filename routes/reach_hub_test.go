package routes_test

import (
	"context"
	"net/http"
	"strings"
	"sync"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const tagSetter = "tag-setter"

var settable sync.Map

func reachTagOf(group string) string { return "t-" + group }

func isOwnerGroup(group string) bool { return group == "owners" || group == "g-owner" }

func wireReach(mgr mwanachamataskmanager.TaskManager, groupsOf map[string][]string) {
	held := func(id string) []string {
		if id == tagSetter {
			return []string{"owners", "g-owner"}
		}
		return groupsOf[id]
	}
	mgr.UseGroups(func(_ context.Context, id string) ([]string, error) { return held(id), nil })
	mgr.UseGroupTags(func(_ context.Context, id string) ([]string, error) {
		var tags []string
		if id == tagSetter {
			settable.Range(func(k, _ any) bool { tags = append(tags, k.(string)); return true })
		}
		for _, g := range held(id) {
			tags = append(tags, reachTagOf(g))
			if isOwnerGroup(g) {
				tags = append(tags, "all")
			}
		}
		return tags, nil
	})
}

func (h *accessHub) tagForSee(path string, body any) bool {
	payload, ok := body.(map[string]string)
	if !ok {
		return false
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[0] != "backlogs" || parts[2] != "access" {
		return false
	}
	settable.Store(reachTagOf(parts[3]), true)
	_, err := h.mgr.AddBacklogTagAs(context.Background(), tagSetter, parts[1], reachTagOf(parts[3]))
	if payload["level"] != "see" {
		return false
	}
	if err != nil {
		h.t.Fatalf("tag %s on %s: %v", parts[3], parts[1], err)
	}
	return true
}

var _ = http.StatusOK
