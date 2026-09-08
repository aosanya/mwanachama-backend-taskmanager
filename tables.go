package mwanachamataskmanager

import (
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// TableNames configures which physical tables a [TaskManager] reads and
// writes. See [gormstore.TableNames].
type TableNames = gormstore.TableNames

// DefaultTableNames builds the conventional table set for one mounted
// instance of this package. See [gormstore.DefaultTableNames].
func DefaultTableNames(instance string) TableNames {
	return gormstore.DefaultTableNames(instance)
}

// Migrate creates or updates the tables named by t. See [gormstore.Migrate].
func Migrate(db *gorm.DB, t TableNames) error {
	return gormstore.Migrate(db, t)
}
