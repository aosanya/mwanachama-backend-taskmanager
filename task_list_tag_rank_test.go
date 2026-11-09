package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestTaskFilter_TagAcceptsOneNameOrAList(t *testing.T) {
	for raw, want := range map[string]int{`{"tag":"Docs"}`: 1, `{"tag":["Docs","Blocked"]}`: 2, `{}`: 0} {
		var filter mwanachamataskmanager.TaskFilter
		if err := json.Unmarshal([]byte(raw), &filter); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if len(filter.Tag) != want {
			t.Fatalf("%s gave %v, want %d tags", raw, filter.Tag, want)
		}
	}
	var bad mwanachamataskmanager.TaskFilter
	if err := json.Unmarshal([]byte(`{"tag":5}`), &bad); err == nil {
		t.Fatal("a number is not a tag")
	}
}

func TestListTasks_EmptyTagIsIgnored(t *testing.T) {
	mgr := newTestManager(t)
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "any"})

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{""}}), "any")
}

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
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "third", Rank: 3})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "first", Rank: 1})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "second", Rank: 2})

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
