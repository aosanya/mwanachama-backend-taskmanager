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

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// TestPinsW20_UpdateProjectCanWriteIntoAnAlreadyDeletedRow pins board row
// W20: project.go's UpdateProject reads the current row through GetProject
// (`WHERE id = ? AND deleted = false`) but writes through a bare
// `Where("id = ?", p.ID).Save(&row)` with no `deleted` guard — the exact
// same shape as W19 (UpdateTask) and mwanachama-backend-assetmanager's A14
// (UpdateAsset/UpdateLocation), found while widening from those two. A
// DeleteProject that lands in the gap between UpdateProject's own read and
// its own write is invisible to the write: the update still applies its new
// Name to the now-deleted row, and UpdateProject returns 200 with no error,
// even though the row is (and stays) soft deleted.
//
// Driven through the real `routes.Routes(tm)` mux behind `httptest.
// NewServer`, with two real *http.Client requests fired concurrently — PUT
// {projectID} racing DELETE {projectID} — repeated across 200 fresh
// projects so the test does not depend on hitting one particular
// interleaving.
//
// Once W20 is fixed (the same CAS shape as W19/A14 — an `UPDATE ... WHERE
// id = ? AND deleted = false`, checking RowsAffected and returning
// ErrProjectNotFound on 0 rows affected), the written-into-deleted count
// here should drop to 0 and this test should be rewritten to assert exactly
// that.
func TestPinsW20_UpdateProjectCanWriteIntoAnAlreadyDeletedRow(t *testing.T) {
	const iterations = 200
	bothSucceeded := 0
	writtenIntoDeletedRow := 0

	for i := 0; i < iterations; i++ {
		tm, db, tables := w19RaceManagerAndDB(t, fmt.Sprintf("w20race%d", i))
		srv := httptest.NewServer(w19RaceMux(tm))
		client := srv.Client()

		p, err := tm.CreateProject(context.Background(), mwanachamataskmanager.Project{Name: "original"})
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}

		var wg sync.WaitGroup
		var updateStatus, deleteStatus int
		var updateErr, deleteErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			_ = json.NewEncoder(&buf).Encode(map[string]any{"id": p.ID, "name": "raced-update"})
			req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/projects/%s", srv.URL, p.ID), &buf)
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
			req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/projects/%s", srv.URL, p.ID), nil)
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
				Name    string
				Deleted bool
			}
			if err := db.Table(tables.Projects).Select("name, deleted").Where("id = ?", p.ID).Scan(&row).Error; err != nil {
				t.Fatalf("raw select: %v", err)
			}
			if row.Deleted && row.Name == "raced-update" {
				writtenIntoDeletedRow++
			}
		}

		srv.Close()
	}

	t.Logf("UpdateProject returned 200 concurrently with DeleteProject succeeding: %d/%d iterations", bothSucceeded, iterations)
	t.Logf("of those, the persisted row ended up deleted=true with the racing update's Name silently applied anyway: %d/%d", writtenIntoDeletedRow, bothSucceeded)

	if writtenIntoDeletedRow == 0 {
		t.Fatalf("expected at least one iteration where a soft-deleted Project row still absorbed a racing update (race not reproduced across %d iterations — current broken behavior no longer confirmed)", iterations)
	}
}
