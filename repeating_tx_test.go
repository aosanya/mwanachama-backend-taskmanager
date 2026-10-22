package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"gorm.io/gorm"
)

func TestATickWhosePeriodRowFailsLeavesNoTaskBehind(t *testing.T) {
	m, db, _ := newTestManagerWithDB(t, nil)
	ctx := context.Background()
	rec, err := m.CreateRepeatingTask(ctx, storedMonthlyReview(t))
	if err != nil {
		t.Fatal(err)
	}
	refuse := errors.New("period row refused")
	if err := db.Callback().Create().Before("gorm:create").Register("refuse_period_row", func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(map[string]any); ok {
			if _, isPeriod := row["period_key"]; isPeriod {
				_ = tx.AddError(refuse)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	if _, made, err := m.MakeRepeatingTaskDue(ctx, rec.ID, at); !errors.Is(err, refuse) || made {
		t.Fatalf("made=%v err=%v", made, err)
	}
	held, err := m.ListHeldTasks(ctx, models.HolderTasksQuery{ActorID: "actor-7"})
	if err != nil || len(held) != 0 {
		t.Fatalf("held %v err %v", held, err)
	}
	if err := db.Callback().Create().Remove("refuse_period_row"); err != nil {
		t.Fatal(err)
	}
	if _, made, err := m.MakeRepeatingTaskDue(ctx, rec.ID, at); err != nil || !made {
		t.Fatalf("retry made=%v err=%v", made, err)
	}
	held, err = m.ListHeldTasks(ctx, models.HolderTasksQuery{ActorID: "actor-7"})
	if err != nil || len(held) != 1 {
		t.Fatalf("held after retry %v err %v", held, err)
	}
}
