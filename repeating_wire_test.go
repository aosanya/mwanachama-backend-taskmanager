package mwanachamataskmanager

import (
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/recur"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestANewRepeatingTaskReadsItsScheduleThroughTheSharedWire(t *testing.T) {
	base := NewRepeatingTask{Title: "x", AssigneeKind: models.AssigneeRole, AssigneeID: "r"}
	ok := base
	ok.Schedule = recur.Wire{Start: "2026-01-05T08:00:00Z", Zone: "UTC", Rule: "FREQ=WEEKLY"}
	if _, err := RepeatingTaskFromNew(ok); err != nil {
		t.Fatalf("a valid wire schedule was refused: %v", err)
	}
	for name, w := range map[string]recur.Wire{
		"dates without end": {Start: "2026-01-05T00:00:00Z", Zone: "UTC", Dates: []string{"2026-01-05"}},
		"open days":         {Start: "2026-01-05T00:00:00Z", Zone: "UTC", Rule: "FREQ=DAILY", OpenDays: 2},
	} {
		n := base
		n.Schedule = w
		if _, err := RepeatingTaskFromNew(n); !errors.Is(err, ErrInvalidRepeatingTask) {
			t.Errorf("%s: got %v, want ErrInvalidRepeatingTask", name, err)
		}
	}
}
