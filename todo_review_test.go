package mwanachamataskmanager_test

import (
	"context"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const everyTodoDoneEntry = "Every todo is Done: moved to Review"

type reviewFixture struct {
	todoFixture
	backlog string
	issue   mwanachamataskmanager.Task
	pieces  []mwanachamataskmanager.TaskTodo
}

func newReviewFixture(t *testing.T, steps []string, startStep string, pieceCount int) reviewFixture {
	t.Helper()
	ctx := context.Background()
	f := newTodoFixture(t)
	made, err := f.mgr.CreateBacklog(ctx, "Issue todo lists", steps)
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	issue, err := f.mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "an issue with todos"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := f.mgr.MoveTaskToStep(ctx, issue.ID, made.ID, startStep); err != nil {
		t.Fatalf("MoveTaskToStep %s: %v", startStep, err)
	}
	fixture := reviewFixture{todoFixture: f, backlog: made.ID, issue: issue}
	for at := 1; at <= pieceCount; at++ {
		fixture.pieces = append(fixture.pieces, f.piece(t, issue.ID, "piece "+string(rune('a'+at-1)), at))
	}
	return fixture
}

func (f reviewFixture) setState(t *testing.T, piece int, state string) {
	t.Helper()
	if _, err := f.mgr.SetTodoStateFor(context.Background(), "", f.pieces[piece].ID, state, ""); err != nil {
		t.Fatalf("SetTodoStateFor %s: %v", state, err)
	}
}

func (f reviewFixture) stepNow(t *testing.T) string {
	t.Helper()
	issue, err := f.mgr.GetTask(context.Background(), f.issue.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return issue.Step
}

func (f reviewFixture) reviewEntries(t *testing.T) int {
	t.Helper()
	notes, err := f.mgr.ListTaskNotes(context.Background(), "", f.issue.ID)
	if err != nil {
		t.Fatalf("ListTaskNotes: %v", err)
	}
	count := 0
	for _, note := range notes {
		if note.Kind == "worker" && strings.Contains(note.Text, everyTodoDoneEntry) {
			count++
		}
	}
	return count
}

var withReview = []string{"To do", "In progress", "Review", "Done"}

func TestLastTodoDoneMovesAnInProgressIssueToReviewWithALogEntry(t *testing.T) {
	f := newReviewFixture(t, withReview, "in-progress", 2)
	f.setState(t, 0, "Done")
	if got := f.stepNow(t); got != "in-progress" {
		t.Fatalf("with a piece still open the issue is in %q, want in-progress", got)
	}
	f.setState(t, 1, "Done")
	if got := f.stepNow(t); got != "review" {
		t.Fatalf("after the last piece is Done the issue is in %q, want review", got)
	}
	if got := f.reviewEntries(t); got != 1 {
		t.Fatalf("the Log holds %d worker entries saying why, want 1", got)
	}
}

func TestLastTodoDoneInABacklogWithoutReviewMovesNothing(t *testing.T) {
	f := newReviewFixture(t, []string{"To do", "In progress", "Done"}, "in-progress", 1)
	f.setState(t, 0, "Done")
	if got := f.stepNow(t); got != "in-progress" {
		t.Fatalf("a backlog without Review moved the issue to %q", got)
	}
	if got := f.reviewEntries(t); got != 0 {
		t.Fatalf("the Log holds %d entries about a move that did not happen", got)
	}
}

func TestLastTodoDoneLeavesAnIssueThatIsNotInProgressWhereItIs(t *testing.T) {
	for _, step := range []string{"to-do", "done"} {
		f := newReviewFixture(t, withReview, step, 1)
		f.setState(t, 0, "Done")
		if got := f.stepNow(t); got != step {
			t.Fatalf("an issue in %q moved to %q", step, got)
		}
		if got := f.reviewEntries(t); got != 0 {
			t.Fatalf("an issue in %q got %d entries about a move", step, got)
		}
	}
}

func TestADoneTodoWithAnOpenPieceBesideItMovesNothing(t *testing.T) {
	for _, open := range []string{"To do", "In progress"} {
		f := newReviewFixture(t, withReview, "in-progress", 2)
		f.setState(t, 1, open)
		f.setState(t, 0, "Done")
		if got := f.stepNow(t); got != "in-progress" {
			t.Fatalf("a piece left %q yet the issue moved to %q", open, got)
		}
	}
}

func TestABlockedPieceKeepsTheIssueInProgress(t *testing.T) {
	f := newReviewFixture(t, withReview, "in-progress", 2)
	if _, err := f.mgr.SetTodoStateFor(context.Background(), "", f.pieces[1].ID, "Blocked", "waiting for the owner"); err != nil {
		t.Fatalf("block: %v", err)
	}
	f.setState(t, 0, "Done")
	if got := f.stepNow(t); got != "in-progress" {
		t.Fatalf("a Blocked piece left and the issue moved to %q", got)
	}
}

func TestAPieceReopenedAfterwardsDoesNotMoveTheIssueBack(t *testing.T) {
	f := newReviewFixture(t, withReview, "in-progress", 1)
	f.setState(t, 0, "Done")
	if got := f.stepNow(t); got != "review" {
		t.Fatalf("the issue is in %q, want review", got)
	}
	f.setState(t, 0, "To do")
	if got := f.stepNow(t); got != "review" {
		t.Fatalf("reopening a piece moved the issue to %q", got)
	}
	f.setState(t, 0, "Done")
	if got := f.stepNow(t); got != "review" {
		t.Fatalf("the issue is in %q after the piece is Done again, want review", got)
	}
	if got := f.reviewEntries(t); got != 1 {
		t.Fatalf("the Log holds %d entries about the move, want 1 (the issue was not In progress the second time)", got)
	}
}

func TestAPieceAddedAfterwardsDoesNotMoveTheIssueBack(t *testing.T) {
	f := newReviewFixture(t, withReview, "in-progress", 1)
	f.setState(t, 0, "Done")
	f.piece(t, f.issue.ID, "late piece", 2)
	if got := f.stepNow(t); got != "review" {
		t.Fatalf("adding a piece moved the issue to %q", got)
	}
}

func TestSettingADoneTodoToDoneAgainIsNotATransitionAndMovesNothing(t *testing.T) {
	f := newReviewFixture(t, withReview, "in-progress", 1)
	f.setState(t, 0, "Done")
	if _, err := f.mgr.MoveTaskToStep(context.Background(), f.issue.ID, f.backlog, "in-progress"); err != nil {
		t.Fatalf("send back: %v", err)
	}
	f.setState(t, 0, "Done")
	if got := f.stepNow(t); got != "in-progress" {
		t.Fatalf("Done again moved the issue to %q", got)
	}
}

func TestReopeningAndFinishingThePieceAfterASendBackMovesTheIssueAgain(t *testing.T) {
	f := newReviewFixture(t, withReview, "in-progress", 1)
	f.setState(t, 0, "Done")
	if _, err := f.mgr.MoveTaskToStep(context.Background(), f.issue.ID, f.backlog, "in-progress"); err != nil {
		t.Fatalf("send back: %v", err)
	}
	f.setState(t, 0, "To do")
	f.setState(t, 0, "Done")
	if got := f.stepNow(t); got != "review" {
		t.Fatalf("the transition to the last Done left the issue in %q", got)
	}
	if got := f.reviewEntries(t); got != 2 {
		t.Fatalf("the Log holds %d entries about the move, want 2", got)
	}
}
