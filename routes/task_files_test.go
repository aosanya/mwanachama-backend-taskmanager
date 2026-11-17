package routes_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func filesOf(t *testing.T, issue map[string]any) []string {
	t.Helper()
	raw, present := issue["files"]
	if !present {
		t.Fatalf("no files field in %v", issue)
	}
	list, ok := raw.([]any)
	if !ok {
		t.Fatalf("files is %T %v, want a list", raw, raw)
	}
	out := []string{}
	for _, item := range list {
		out = append(out, item.(string))
	}
	return out
}

func TestCreateWithFilesThenReadReturnsThem(t *testing.T) {
	f := newMoreWritesFixture(t)
	want := []string{"hub-steward:code/app/src/app/menu/MenuEditForm.tsx", "code/wakala-api/modules/taskmanager/", "documentation/readme.md"}
	made := f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{
		"title": "with files", "files": want, "backlog": f.docs, "step": "write",
	})
	if got := filesOf(t, made); !reflect.DeepEqual(got, want) {
		t.Fatalf("created files %v, want %v", got, want)
	}
	read := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+made["id"].(string), nil)
	if got := filesOf(t, read); !reflect.DeepEqual(got, want) {
		t.Fatalf("read files %v, want %v", got, want)
	}
}

func TestAnIssueMadeWithoutFilesReadsAnEmptyList(t *testing.T) {
	f := newMoreWritesFixture(t)
	read := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue, nil)
	if got := filesOf(t, read); len(got) != 0 {
		t.Fatalf("files %v, want none", got)
	}
}

func TestPatchReplacesTheFilesAndAnEmptyListClearsThem(t *testing.T) {
	f := newMoreWritesFixture(t)
	path := "/tasks/" + f.issue
	f.h.expect(http.StatusOK, "boss", "PATCH", path, map[string]any{"files": []string{"a/one.go", "a/two.go"}})

	replaced := f.h.expect(http.StatusOK, "boss", "PATCH", path, map[string]any{"files": []string{"b/three.go"}})
	if got := filesOf(t, replaced); !reflect.DeepEqual(got, []string{"b/three.go"}) {
		t.Fatalf("replaced files %v", got)
	}

	f.h.expect(http.StatusOK, "boss", "PATCH", path, map[string]any{"title": "renamed"})
	kept := f.h.expect(http.StatusOK, "boss", "GET", path, nil)
	if got := filesOf(t, kept); !reflect.DeepEqual(got, []string{"b/three.go"}) {
		t.Fatalf("a patch without files changed them: %v", got)
	}

	cleared := f.h.expect(http.StatusOK, "boss", "PATCH", path, map[string]any{"files": []string{}})
	if got := filesOf(t, cleared); len(got) != 0 {
		t.Fatalf("cleared files %v", got)
	}
	read := f.h.expect(http.StatusOK, "boss", "GET", path, nil)
	if got := filesOf(t, read); len(got) != 0 {
		t.Fatalf("read after clear %v", got)
	}
}

func TestAWholeUpdateWithoutFilesKeepsThem(t *testing.T) {
	f := newMoreWritesFixture(t)
	path := "/tasks/" + f.issue
	f.h.expect(http.StatusOK, "boss", "PATCH", path, map[string]any{"files": []string{"keep/me.go"}})

	f.h.expect(http.StatusOK, "boss", "PUT", path, map[string]any{"title": "write the guide", "status": "pending", "priority": "high"})
	read := f.h.expect(http.StatusOK, "boss", "GET", path, nil)
	if got := filesOf(t, read); !reflect.DeepEqual(got, []string{"keep/me.go"}) {
		t.Fatalf("files after a PUT without files: %v", got)
	}
}

func TestDuplicateAndSurroundingSpaceAreDroppedFromFiles(t *testing.T) {
	f := newMoreWritesFixture(t)
	patched := f.h.expect(http.StatusOK, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{
		"files": []string{"a/one.go", " a/one.go ", "b/two.go", "a/one.go"},
	})
	if got := filesOf(t, patched); !reflect.DeepEqual(got, []string{"a/one.go", "b/two.go"}) {
		t.Fatalf("files %v", got)
	}
}

func TestEveryInvalidFileEntryIsRefusedWithItsReason(t *testing.T) {
	f := newMoreWritesFixture(t)
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("dir/file%d.go", i)
	}
	cases := []struct {
		name  string
		files []string
		said  string
	}{
		{"blank", []string{"ok.go", "   "}, "blank"},
		{"empty", []string{""}, "blank"},
		{"too long", []string{strings.Repeat("a", 301)}, "300"},
		{"nul", []string{"bad\x00name.go"}, "control"},
		{"newline", []string{"bad\nname.go"}, "control"},
		{"tab", []string{"bad\tname.go"}, "control"},
		{"leading slash", []string{"/etc/passwd"}, "/etc/passwd"},
		{"repo then leading slash", []string{"hub-steward:/code/x.go"}, "hub-steward:/code/x.go"},
		{"parent segment", []string{"code/../secret.go"}, "code/../secret.go"},
		{"leading parent", []string{"../secret.go"}, "../secret.go"},
		{"only a repo", []string{"hub-steward:"}, "hub-steward:"},
		{"more than 100", tooMany, "100"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			created := f.h.expect(http.StatusBadRequest, "boss", "POST", "/tasks", map[string]any{
				"title": "bad files", "files": c.files, "backlog": f.docs, "step": "write",
			})
			patched := f.h.expect(http.StatusBadRequest, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{"files": c.files})
			for _, body := range []map[string]any{created, patched} {
				if !strings.Contains(fmt.Sprint(body), c.said) {
					t.Fatalf("refusal %v does not say %q", body, c.said)
				}
			}
		})
	}
	read := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue, nil)
	if got := filesOf(t, read); len(got) != 0 {
		t.Fatalf("a refused patch changed the files: %v", got)
	}
}

func TestExactlyOneHundredFilesAreAccepted(t *testing.T) {
	f := newMoreWritesFixture(t)
	files := make([]string, 100)
	for i := range files {
		files[i] = fmt.Sprintf("dir/file%d.go", i)
	}
	patched := f.h.expect(http.StatusOK, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{"files": files})
	if got := filesOf(t, patched); len(got) != 100 {
		t.Fatalf("got %d files", len(got))
	}
}

func TestTheCreateAndPatchToolsTakeAFilesList(t *testing.T) {
	found := map[string]bool{}
	for _, tool := range routes.ToolShape() {
		if tool.Name != "taskmanager_task_create" && tool.Name != "taskmanager_task_patch" {
			continue
		}
		var schema struct {
			Properties map[string]struct {
				Type  string `json:"type"`
				Items struct {
					Type string `json:"type"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		files := schema.Properties["files"]
		if files.Type != "array" || files.Items.Type != "string" {
			t.Fatalf("%s files argument is %+v, want an array of strings", tool.Name, files)
		}
		found[tool.Name] = true
	}
	if len(found) != 2 {
		t.Fatalf("found %v", found)
	}
}
