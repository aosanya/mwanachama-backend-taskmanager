package mwanachamataskmanager

import (
	"sync"

	"gorm.io/gorm"
)

var backlogSaves sync.Mutex

func (m *taskManager) lockBacklogs(tx *gorm.DB) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("select pg_advisory_xact_lock(hashtext(?))", m.store.Table(roleTask)+":"+backlogsSetting).Error
}
