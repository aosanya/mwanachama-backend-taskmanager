package mwanachamataskmanager

import (
	"fmt"
	"time"
)

func checkDueAt(dueAt string) error {
	if dueAt == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, dueAt); err != nil {
		return fmt.Errorf("%w: due_at %q is not an RFC 3339 time", ErrInvalidTask, dueAt)
	}
	return nil
}
