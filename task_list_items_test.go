package mwanachamataskmanager_test

import (
	"encoding/json"
	"net/http"
	"sort"
	"testing"
)

func listItemsReply(t *testing.T, b *board, query string) []map[string]any {
	t.Helper()
	code, raw := b.call(http.MethodGet, "/tasks"+query, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /tasks%s = %d %s", query, code, raw)
	}
	var reply map[string]json.RawMessage
	if err := json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("GET /tasks%s: reply is not a JSON object: %s", query, raw)
	}
	rawItems, ok := reply["items"]
	if !ok {
		t.Fatalf("GET /tasks%s: reply has no items key: %s", query, raw)
	}
	var items []map[string]any
	if err := json.Unmarshal(rawItems, &items); err != nil {
		t.Fatalf("GET /tasks%s: items is not a list of objects: %s", query, rawItems)
	}
	return items
}
func sortedKeys(item map[string]any) []string {
	keys := make([]string, 0, len(item))
	for key := range item {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func seedThreeIssues(b *board) {
	for _, title := range []string{"first issue", "second issue", "third issue"} {
		b.create(map[string]any{"title": title})
	}
}
func TestListTasksAnswersAnItemsObject(t *testing.T) {
	b := newBoard(t)
	seedThreeIssues(b)
	items := listItemsReply(t, b, "")
	if len(items) != 3 {
		t.Fatalf("items has %d entries, want 3", len(items))
	}
	for _, item := range items {
		for _, key := range []string{"id", "code", "title"} {
			if _, ok := item[key]; !ok {
				t.Errorf("item lacks %s: %v", key, item)
			}
		}
	}
}
func TestListTasksWithFieldsKeepsOnlyTheNamedKeysInsideItems(t *testing.T) {
	b := newBoard(t)
	seedThreeIssues(b)
	items := listItemsReply(t, b, "?fields=id,code")
	if len(items) != 3 {
		t.Fatalf("items has %d entries, want 3", len(items))
	}
	for _, item := range items {
		got := sortedKeys(item)
		if len(got) != 2 || got[0] != "code" || got[1] != "id" {
			t.Errorf("item keys %v, want [code id]", got)
		}
	}
}
