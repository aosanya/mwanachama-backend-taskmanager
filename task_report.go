package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type UnloggedTodo struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Title  string `json:"title"`
	Number int    `json:"number"`
}

type TaskReport struct {
	TaskID         string         `json:"task_id"`
	Step           string         `json:"step"`
	Tags           []string       `json:"tags"`
	TodosDone      int            `json:"todos_done"`
	TodosTotal     int            `json:"todos_total"`
	Blocked        []HeldTodo     `json:"blocked"`
	Commits        []string       `json:"commits"`
	EntryCount     int            `json:"entry_count"`
	LastEntryID    *string        `json:"last_entry_id"`
	FirstEntryAt   *string        `json:"first_entry_at"`
	LastEntryAt    *string        `json:"last_entry_at"`
	MinutesBetween *int           `json:"minutes_between"`
	Missing        []UnloggedTodo `json:"missing"`
}

func (m *taskManager) TaskReportFor(ctx context.Context, requesterID, issueID string) (TaskReport, error) {
	task, err := m.GetTaskFor(ctx, requesterID, issueID)
	if err != nil {
		return TaskReport{}, err
	}
	todos, err := m.issueTodoList(ctx, issueID)
	if err != nil {
		return TaskReport{}, err
	}
	notes, err := m.allTaskNotes(ctx, issueID)
	if err != nil {
		return TaskReport{}, err
	}
	report := TaskReport{
		TaskID: issueID, Step: task.Step, Tags: append([]string{}, task.Tags...),
		TodosDone: todos.Done, TodosTotal: todos.Total,
		Blocked: heldTodosOf(todos), Commits: commitLinksOf(notes), EntryCount: len(notes),
		Missing: unloggedTodosOf(todos, notes),
	}
	report.setLogTimes(notes)
	return report, nil
}

func heldTodosOf(list IssueTodoList) []HeldTodo {
	held := []HeldTodo{}
	for _, view := range list.Items {
		if (view.Status == TodoStatusBlocked || view.Status == TodoStatusFailed) && view.BlockedReason != "" {
			held = append(held, HeldTodo{
				ID: view.ID, Code: view.Code, Title: view.Title,
				Ordinality: view.Ordinality, BlockedReason: view.BlockedReason,
			})
		}
	}
	return held
}

func commitLinksOf(notes []TaskNote) []string {
	commits := []string{}
	seen := map[string]bool{}
	for _, note := range notes {
		for _, link := range note.Links {
			if !strings.HasPrefix(link, "commit") {
				continue
			}
			key := commitKey(link)
			if !seen[key] {
				seen[key] = true
				commits = append(commits, link)
			}
		}
	}
	return commits
}

func commitKey(link string) string {
	if words := strings.Fields(link); len(words) > 1 {
		return words[1]
	}
	return link
}

func unloggedTodosOf(list IssueTodoList, notes []TaskNote) []UnloggedTodo {
	missing := []UnloggedTodo{}
	for i, view := range list.Items {
		if view.Status != TodoStatusCompleted {
			continue
		}
		prefix := fmt.Sprintf("Todo %d of %d done", i+1, list.Total)
		if !anyNoteStartsWith(notes, prefix) {
			missing = append(missing, UnloggedTodo{ID: view.ID, Code: view.Code, Title: view.Title, Number: i + 1})
		}
	}
	return missing
}

func anyNoteStartsWith(notes []TaskNote, prefix string) bool {
	for _, note := range notes {
		if strings.HasPrefix(strings.TrimSpace(note.Text), prefix) {
			return true
		}
	}
	return false
}

func (r *TaskReport) setLogTimes(notes []TaskNote) {
	if len(notes) == 0 {
		return
	}
	first, last := notes[0], notes[len(notes)-1]
	r.LastEntryID = &last.ID
	r.FirstEntryAt = &first.CreatedAt
	r.LastEntryAt = &last.CreatedAt
	began, beganErr := time.Parse(noteTimeForm, first.CreatedAt)
	ended, endedErr := time.Parse(noteTimeForm, last.CreatedAt)
	if beganErr == nil && endedErr == nil {
		minutes := int(ended.Sub(began) / time.Minute)
		r.MinutesBetween = &minutes
	}
}
