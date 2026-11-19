package mwanachamataskmanager_test

import (
	"context"
	"github.com/aosanya/mwanachama-backend-shared/recur"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"testing"
	"time"
)

const budapest = "Europe/Budapest"

func scheduleRecord(rule, start, end, zone string) mwanachamataskmanager.RepeatingTaskRecord {
	return mwanachamataskmanager.RepeatingTaskRecord{ID: "rt-1", Title: "Inspect the playground", ScheduleRule: rule, ScheduleStart: start, ScheduleEnd: end, ScheduleZone: zone, AssigneeKind: models.AssigneeActor, AssigneeID: "actor-1"}
}
func TestNextDueAtNamesTheDayOfTheNextPeriod(t *testing.T) {
	wednesdayNoon := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	tuesdayNoon := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	stopped := scheduleRecord("FREQ=WEEKLY;BYDAY=MO", "2026-11-02T07:00:00Z", "", budapest)
	stopped.StoppedAt = "2026-11-10T09:00:00Z"
	cases := []struct {
		name string
		rec  mwanachamataskmanager.RepeatingTaskRecord
		now  time.Time
		want string
	}{{name: "a weekly Monday rule seen on a Wednesday names the coming Monday at its start time", rec: scheduleRecord("FREQ=WEEKLY;BYDAY=MO", "2026-11-02T07:00:00Z", "", budapest), now: wednesdayNoon, want: "2026-11-23T08:00:00+01:00"}, {name: "a monthly day-1 rule names the first of next month at the same wall time", rec: scheduleRecord("FREQ=MONTHLY;BYMONTHDAY=1", "2026-11-01T08:00:00Z", "", budapest), now: tuesdayNoon, want: "2026-12-01T09:00:00+01:00"}, {name: "a fortnightly Wednesday rule on a Wednesday that is itself an occurrence names the one two weeks on", rec: scheduleRecord("FREQ=WEEKLY;INTERVAL=2;BYDAY=WE", "2026-11-04T07:00:00Z", "", budapest), now: wednesdayNoon, want: "2026-12-02T08:00:00+01:00"}, {name: "a stopped schedule names no day", rec: stopped, now: wednesdayNoon, want: ""}, {name: "a schedule whose end has passed names no day", rec: scheduleRecord("FREQ=WEEKLY;BYDAY=MO", "2026-11-02T07:00:00Z", "2026-11-10T00:00:00Z", budapest), now: wednesdayNoon, want: ""}, {name: "a list of dates that are all behind names no day", rec: func() mwanachamataskmanager.RepeatingTaskRecord {
		rec := scheduleRecord("", "2026-11-01T07:00:00Z", "2026-11-12T00:00:00Z", budapest)
		rec.ScheduleDates = []string{"2026-11-03", "2026-11-10"}
		return rec
	}(), now: wednesdayNoon, want: ""}, {name: "a schedule with an unknown zone names no day", rec: scheduleRecord("FREQ=WEEKLY;BYDAY=MO", "2026-11-02T07:00:00Z", "", "Nowhere/Land"), now: wednesdayNoon, want: ""}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mwanachamataskmanager.NextDueAt(c.rec, c.now)
			if got != c.want {
				t.Fatalf("NextDueAt = %q, want %q", got, c.want)
			}
		})
	}
}
func TestNextDueAtIsWrittenInTheSchedulesOwnZone(t *testing.T) {
	rec := scheduleRecord("FREQ=DAILY", "2026-10-31T23:30:00Z", "", budapest)
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	got := mwanachamataskmanager.NextDueAt(rec, now)
	if len(got) < 10 || got[:10] != "2026-11-18" {
		t.Fatalf("NextDueAt = %q, want the Budapest day 2026-11-18 first", got)
	}
	if got != "2026-11-18T00:30:00+01:00" {
		t.Fatalf("NextDueAt = %q, want 2026-11-18T00:30:00+01:00", got)
	}
}
func TestNextDueAtOnTheRepeatingTasksOfAManager(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	started, err := mgr.StartRepeatingTask(ctx, mwanachamataskmanager.NewRepeatingTask{Title: "Inspect the playground", Schedule: recur.Wire{Start: time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339), Zone: budapest, Rule: "FREQ=DAILY"}, AssigneeKind: models.AssigneeActor, AssigneeID: "actor-1"})
	if err != nil {
		t.Fatalf("StartRepeatingTask: %v", err)
	}
	assertComingDay := func(label, got string) {
		t.Helper()
		next, err := time.Parse(time.RFC3339, got)
		if err != nil {
			t.Fatalf("%s NextDueAt %q is not RFC 3339: %v", label, got, err)
		}
		if !next.After(time.Now()) {
			t.Fatalf("%s NextDueAt %q is not after now", label, got)
		}
	}
	read, err := mgr.GetRepeatingTask(ctx, started.ID)
	if err != nil {
		t.Fatalf("GetRepeatingTask: %v", err)
	}
	assertComingDay("get", read.NextDueAt)
	listed, err := mgr.ListRepeatingTasks(ctx, mwanachamataskmanager.RepeatingTaskFilter{})
	if err != nil {
		t.Fatalf("ListRepeatingTasks: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListRepeatingTasks returned %d schedules, want 1", len(listed))
	}
	assertComingDay("list", listed[0].NextDueAt)
	if _, err := mgr.StopRepeatingTask(ctx, started.ID); err != nil {
		t.Fatalf("StopRepeatingTask: %v", err)
	}
	after, err := mgr.GetRepeatingTask(ctx, started.ID)
	if err != nil {
		t.Fatalf("GetRepeatingTask after stop: %v", err)
	}
	if after.NextDueAt != "" {
		t.Fatalf("a stopped schedule has NextDueAt %q, want empty", after.NextDueAt)
	}
}
