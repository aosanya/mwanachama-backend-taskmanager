package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	capInputKey   = "input_tokens"
	capOutputKey  = "output_tokens"
	capRuntimeKey = "runtime_seconds"

	toDoStepID      = "to-do"
	defaultBacklog  = "backlog"
	noFilesReported = "not reported by the runner"
)

var markTags = []string{tagReady, tagWaiting, tagBlocked, tagSinbin}

var capLabels = []struct{ key, label string }{
	{capInputKey, "Input tokens"},
	{capOutputKey, "Output tokens"},
	{capRuntimeKey, "Runtime (working time)"},
}

func passedCaps(caps WorkerCaps, used TaskUsageWrite) []string {
	var passed []string
	if used.InputTokens >= caps.InputTokens {
		passed = append(passed, capInputKey)
	}
	if used.OutputTokens >= caps.OutputTokens {
		passed = append(passed, capOutputKey)
	}
	if !used.ClockStopped && used.SecondsUsed >= caps.RuntimeSeconds {
		passed = append(passed, capRuntimeKey)
	}
	return passed
}

func withThousands(n int64) string {
	digits := strconv.FormatInt(n, 10)
	var out []string
	for len(digits) > 3 {
		out = append([]string{digits[len(digits)-3:]}, out...)
		digits = digits[:len(digits)-3]
	}
	return strings.Join(append([]string{digits}, out...), ",")
}

func capLabel(key string) string {
	for _, c := range capLabels {
		if c.key == key {
			return c.label
		}
	}
	return key
}

func capFigures(caps WorkerCaps, used TaskUsageWrite) string {
	figures := map[string][2]int64{
		capInputKey:   {used.InputTokens, caps.InputTokens},
		capOutputKey:  {used.OutputTokens, caps.OutputTokens},
		capRuntimeKey: {used.SecondsUsed, caps.RuntimeSeconds},
	}
	parts := make([]string, 0, len(capLabels))
	for _, c := range capLabels {
		parts = append(parts, fmt.Sprintf("%s %s of %s", c.label, withThousands(figures[c.key][0]), withThousands(figures[c.key][1])))
	}
	return strings.Join(parts, "; ")
}

func (m *taskManager) lastWorkerStep(ctx context.Context, requesterID, taskID string) string {
	notes, err := m.ListTaskNotes(ctx, requesterID, taskID)
	if err != nil {
		return "none logged yet"
	}
	for i := len(notes) - 1; i >= 0; i-- {
		if notes[i].Kind == noteByWorker {
			return notes[i].Text
		}
	}
	return "none logged yet"
}

func filesLine(files *[]string) string {
	switch {
	case files == nil:
		return noFilesReported
	case len(*files) == 0:
		return "none"
	}
	return strings.Join(*files, ", ")
}

func sinbinTags(tags []string) []string {
	kept := make([]string, 0, len(tags)+1)
	for _, tag := range tags {
		if !hasTagFold(markTags, tag) {
			kept = append(kept, tag)
		}
	}
	return append(kept, tagSinbin)
}

func (m *taskManager) placeInSinbin(ctx context.Context, requesterID string, task Task) []string {
	var failed []string
	tags := sinbinTags(task.Tags)
	if _, err := m.PatchTaskAs(ctx, requesterID, TaskPatch{ID: task.ID, Tags: &tags}); err != nil {
		failed = append(failed, "change the tags")
	}
	backlog := task.Backlog
	if backlog == "" {
		backlog = defaultBacklog
	}
	if _, err := m.MoveTaskToStep(ctx, task.ID, backlog, toDoStepID); err != nil {
		failed = append(failed, "move the issue to To do")
	}
	return failed
}

func (m *taskManager) stopEntry(ctx context.Context, requesterID, taskID string, caps WorkerCaps, used TaskUsageWrite, passed []string, failed []string) string {
	names := make([]string, 0, len(passed))
	for _, key := range passed {
		names = append(names, capLabel(key))
	}
	where := "Tagged Sinbin without Ready and moved to To do."
	if len(failed) > 0 {
		where = "The hub could not " + strings.Join(failed, " or ") + ", so the tags and the step are as they were."
	}
	return fmt.Sprintf(
		"Stopped at a cap. Cap passed: %s. Used: %s. Last step finished: %s. Left: everything after that entry; the next worker reads the Log and continues from it. Files changed, not committed: %s; the stop neither commits nor discards them. %s",
		strings.Join(names, ", "), capFigures(caps, used), m.lastWorkerStep(ctx, requesterID, taskID), filesLine(used.FilesUncommitted), where,
	)
}

func (m *taskManager) stopWorker(ctx context.Context, requesterID string, caps WorkerCaps, used TaskUsageWrite, passed []string, task Task) {
	failed := m.placeInSinbin(ctx, requesterID, task)
	text := m.stopEntry(ctx, requesterID, task.ID, caps, used, passed, failed)
	m.AddTaskNote(ctx, requesterID, task.ID, noteByWorker, text, nil)
}

func (m *taskManager) applyCaps(ctx context.Context, requesterID string, task Task, used TaskUsageWrite, reading TaskCaps) TaskCaps {
	passed := passedCaps(reading.InForce, used)
	if len(passed) == 0 {
		return reading
	}
	reading.Stop, reading.Passed = true, passed
	if task.Status == TaskStatusInProgress {
		m.stopWorker(ctx, requesterID, reading.InForce, used, passed, task)
	}
	return reading
}
