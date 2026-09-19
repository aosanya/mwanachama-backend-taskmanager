package routes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

// w19RaceManagerAndDB mirrors this package's other race-test managers (see
// w15's own wk15Manager) but also hands back the underlying *gorm.DB and
// table names, so the test can peek at persisted row state (deleted plus a
// content field together) that no TaskManager method exposes — GetTask/
// ListTasks both filter deleted=false, so a deleted row's current content
// is otherwise unobservable through the public API.
func w19RaceManagerAndDB(t *testing.T, prefix string) (mwanachamataskmanager.TaskManager, *gorm.DB, mwanachamataskmanager.TableNames) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	tables := mwanachamataskmanager.DefaultTableNames(prefix)
	if err := mwanachamataskmanager.Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, tables, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr, db, tables
}

func w19RaceMux(tm mwanachamataskmanager.TaskManager) *http.ServeMux {
	m := http.NewServeMux()
	for _, rt := range routes.Routes(tm) {
		m.Handle(rt.Pattern(""), rt.Handler)
	}
	return m
}

// TestPinsW19_UpdateTaskCanWriteIntoAnAlreadyDeletedRow pins board row W19:
// task_impl_task.go's UpdateTask reads the current row through GetTask
// (`WHERE id = ? AND deleted = false`) but writes through a bare
// `Save(&row).Where("id = ?", task.ID)` with no `deleted` guard at all — a
// DeleteTask that lands in the gap between UpdateTask's own read and its
// own write is invisible to the write: the update still applies its new
// Title/Status/etc to the now-deleted row, and UpdateTask returns 200 with
// no error, even though the row is (and stays) soft deleted.
//
// Driven through the real `routes.Routes(tm)` mux behind `httptest.
// NewServer`, with two real *http.Client requests fired concurrently — PUT
// {taskID} racing DELETE {taskID} — repeated across 200 fresh tasks so the
// test does not depend on hitting one particular interleaving. This is the
// identical shape as `mwanachama-backend-assetmanager`'s board row A14
// (`UpdateAsset`/`UpdateLocation` racing their own Delete counterparts,
// found the same session) — the same loophole class (a soft-delete flag
// one write path checks and another ignores) reproduced in a second repo.
//
// Once W19 is fixed (e.g. `Where("id = ? AND deleted = ?", task.ID,
// false).Save(&row)` or an explicit `Updates` with that same WHERE,
// checking RowsAffected and returning ErrTaskNotFound on 0 rows affected —
// the same compare-and-swap shape A14 and mwanachama-backend-git's
// `advanceBranchHead` already use), the deleted count here should drop to
// 0 and this test should be rewritten to assert exactly that.
func TestPinsW19_UpdateTaskCanWriteIntoAnAlreadyDeletedRow(t *testing.T) {
	const iterations = 200
	bothSucceeded := 0
	writtenIntoDeletedRow := 0

	for i := 0; i < iterations; i++ {
		tm, db, tables := w19RaceManagerAndDB(t, fmt.Sprintf("w19race%d", i))
		srv := httptest.NewServer(w19RaceMux(tm))
		client := srv.Client()

		task, err := tm.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "original"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		var wg sync.WaitGroup
		var updateStatus, deleteStatus int
		var updateErr, deleteErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			_ = json.NewEncoder(&buf).Encode(map[string]any{"id": task.ID, "title": "raced-update", "status": "pending"})
			req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/tasks/%s", srv.URL, task.ID), &buf)
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				updateErr = err
				return
			}
			defer resp.Body.Close()
			updateStatus = resp.StatusCode
		}()
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/tasks/%s", srv.URL, task.ID), nil)
			resp, err := client.Do(req)
			if err != nil {
				deleteErr = err
				return
			}
			defer resp.Body.Close()
			deleteStatus = resp.StatusCode
		}()
		wg.Wait()
		if updateErr != nil {
			t.Fatalf("update Do: %v", updateErr)
		}
		if deleteErr != nil {
			t.Fatalf("delete Do: %v", deleteErr)
		}

		if updateStatus == http.StatusOK && (deleteStatus == http.StatusNoContent || deleteStatus == http.StatusOK) {
			bothSucceeded++
			var row struct {
				Title   string
				Deleted bool
			}
			if err := db.Table(tables.Tasks).Select("title, deleted").Where("id = ?", task.ID).Scan(&row).Error; err != nil {
				t.Fatalf("raw select: %v", err)
			}
			if row.Deleted && row.Title == "raced-update" {
				writtenIntoDeletedRow++
			}
		}

		srv.Close()
	}

	t.Logf("UpdateTask returned 200 concurrently with DeleteTask succeeding: %d/%d iterations", bothSucceeded, iterations)
	t.Logf("of those, the persisted row ended up deleted=true with the racing update's Title silently applied anyway: %d/%d", writtenIntoDeletedRow, bothSucceeded)

	if writtenIntoDeletedRow == 0 {
		t.Fatalf("expected at least one iteration where a soft-deleted Task row still absorbed a racing update (race not reproduced across %d iterations — current broken behavior no longer confirmed)", iterations)
	}
}
