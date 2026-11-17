package mwanachamataskmanager

import (
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/plaintext"
)

func checkTaskTitle(title string) error {
	if problem := plaintext.Problem("title", title); problem != "" {
		return fmt.Errorf("%w: %s", ErrInvalidTask, problem)
	}
	return nil
}
