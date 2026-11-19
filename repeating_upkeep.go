package mwanachamataskmanager

import "time"

func NextDueAt(rec RepeatingTaskRecord, after time.Time) string {
	if rec.StoppedAt != "" {
		return ""
	}
	r, err := RepeatingTaskFromRecord(rec)
	if err != nil {
		return ""
	}
	next, ok := r.Schedule.Next(after)
	if !ok {
		return ""
	}
	return next.In(r.Schedule.Zone).Format(time.RFC3339)
}
