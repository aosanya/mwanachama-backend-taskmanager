package mwanachamataskmanager

import "fmt"

var efforts = []TaskEffort{
	TaskEffortUnknown, TaskEffortVeryLow, TaskEffortLow, TaskEffortMedium, TaskEffortHigh, TaskEffortVeryHigh,
}

func checkEffort(e TaskEffort) error {
	for _, known := range efforts {
		if known == e {
			return nil
		}
	}
	return fmt.Errorf("%w, got %q", ErrInvalidEffort, string(e))
}
