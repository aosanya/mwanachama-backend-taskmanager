package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestCreateTask_ARankTheCallerSendsIsKept(t *testing.T) {
	mgr := newTestManager(t)
	for _, rank := range []int{1, 7, 500} {
		made, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "ranked", Rank: rank})
		if err != nil {
			t.Fatal(err)
		}
		if made.Rank != rank {
			t.Fatalf("sent rank %d, stored %d", rank, made.Rank)
		}
		read, err := mgr.GetTask(context.Background(), made.ID)
		if err != nil || read.Rank != rank {
			t.Fatalf("read back rank %d (%v), want %d", read.Rank, err, rank)
		}
	}
}

func TestCreateTask_NoRankMintsOneBelowTheBottom(t *testing.T) {
	mgr := newTestManager(t)
	first, _ := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "first", Rank: 3})
	second, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Rank <= first.Rank {
		t.Fatalf("minted rank %d is not below the bottom rank %d", second.Rank, first.Rank)
	}
}

func TestListTasks_ReturnsTheLoadersOrderNotTheCreationOrder(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "third", Rank: 3})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "first", Rank: 1})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "second", Rank: 2})

	tasks, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	got := ""
	for _, task := range tasks {
		got += task.Title + ","
	}
	if got != "first,second,third," {
		t.Fatalf("listed %s, want first,second,third,", got)
	}
}
