package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
)

const rankGap = 1 << 16

const taskOrder = `"rank", created_at, id`

type rankedCard struct {
	ID   string
	Rank int
}

func (m *taskManager) nextRank(ctx context.Context) (int, error) {
	var top int
	err := m.store.Query(ctx, roleTask).Select(`COALESCE(MAX("rank"), 0)`).Scan(&top).Error
	if err != nil {
		return 0, err
	}
	return top + rankGap, nil
}

func sortByRank(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		if a.Rank != b.Rank {
			return a.Rank < b.Rank
		}
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		return a.ID < b.ID
	})
}

func (m *taskManager) PlaceTask(ctx context.Context, taskID, afterID, beforeID string) (Task, error) {
	if afterID == "" && beforeID == "" {
		return Task{}, fmt.Errorf("%w: name the card it goes after or before", ErrInvalidPlacement)
	}
	if taskID == afterID || taskID == beforeID {
		return Task{}, fmt.Errorf("%w: a card cannot be placed beside itself", ErrInvalidPlacement)
	}
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return m.place(tx, taskID, afterID, beforeID)
	})
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) || errors.Is(err, ErrInvalidPlacement) {
			return Task{}, err
		}
		return Task{}, fmt.Errorf("PlaceTask: %w", err)
	}
	return m.GetTask(ctx, taskID)
}

func (m *taskManager) place(tx *gorm.DB, taskID, afterID, beforeID string) error {
	table := m.store.Table(roleTask)
	var cards []rankedCard
	if err := tx.Table(table).Select(`id, "rank"`).Where("deleted = ?", false).Order(taskOrder).Scan(&cards).Error; err != nil {
		return err
	}
	rest := make([]rankedCard, 0, len(cards))
	moved := false
	for _, c := range cards {
		if c.ID == taskID {
			moved = true
			continue
		}
		rest = append(rest, c)
	}
	if !moved {
		return ErrTaskNotFound
	}
	at, err := insertionIndex(rest, afterID, beforeID)
	if err != nil {
		return err
	}
	rank, room := rankAt(rest, at)
	if room {
		return tx.Table(table).Where("id = ?", taskID).UpdateColumn("rank", rank).Error
	}
	return renumber(tx, table, rest, at, taskID)
}

func insertionIndex(rest []rankedCard, afterID, beforeID string) (int, error) {
	indexOf := func(id string) (int, bool) {
		for i, c := range rest {
			if c.ID == id {
				return i, true
			}
		}
		return 0, false
	}
	after, before := -1, len(rest)
	if afterID != "" {
		i, ok := indexOf(afterID)
		if !ok {
			return 0, ErrTaskNotFound
		}
		after = i
	}
	if beforeID != "" {
		i, ok := indexOf(beforeID)
		if !ok {
			return 0, ErrTaskNotFound
		}
		before = i
	}
	if afterID != "" && beforeID != "" && after >= before {
		return 0, fmt.Errorf("%w: the card it goes after sits below the card it goes before", ErrInvalidPlacement)
	}
	if afterID != "" {
		return after + 1, nil
	}
	return before, nil
}

func rankAt(rest []rankedCard, at int) (int, bool) {
	switch {
	case len(rest) == 0:
		return rankGap, true
	case at == 0:
		return rest[0].Rank - rankGap, true
	case at == len(rest):
		return rest[len(rest)-1].Rank + rankGap, true
	}
	lo, hi := rest[at-1].Rank, rest[at].Rank
	if hi-lo < 2 {
		return 0, false
	}
	return lo + (hi-lo)/2, true
}

func renumber(tx *gorm.DB, table string, rest []rankedCard, at int, taskID string) error {
	ordered := make([]rankedCard, 0, len(rest)+1)
	ordered = append(ordered, rest[:at]...)
	ordered = append(ordered, rankedCard{ID: taskID, Rank: -1})
	ordered = append(ordered, rest[at:]...)
	for i, c := range ordered {
		want := (i + 1) * rankGap
		if c.Rank == want {
			continue
		}
		if err := tx.Table(table).Where("id = ?", c.ID).UpdateColumn("rank", want).Error; err != nil {
			return err
		}
	}
	return nil
}
