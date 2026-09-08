// import.go implements [TaskManager.ImportProject], [TaskManager.StartImportProject],
// [TaskManager.GetImportProjectStatus], and [TaskManager.CancelImportProject].
//
// ImportProject is a synchronous convenience wrapper over the async
// StartImportProject → poll pattern, kept for callers that want a single
// blocking call. For large documents the async path is preferred.
//
// The async flow:
//  1. StartImportProject creates an ImportProjectJob row (status=pending)
//     and returns immediately.
//  2. A background goroutine parses the document, creates all entities, and
//     transitions the job through pending → running → completed | failed |
//     cancelled.
//  3. GetImportProjectStatus polls the stored row and augments the result
//     with in-memory progress steps while the goroutine is alive.
//  4. CancelImportProject signals the goroutine via context cancellation.
package mwanachamataskmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

const (
	importJobStatusPending   = "pending"
	importJobStatusRunning   = "running"
	importJobStatusCompleted = "completed"
	importJobStatusFailed    = "failed"
	importJobStatusCancelled = "cancelled"
)

// importJobEntry holds the cancel function and in-memory progress log for an
// in-flight import goroutine.
type importJobEntry struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	steps  []string
}

func (e *importJobEntry) appendStep(msg string) {
	e.mu.Lock()
	e.steps = append(e.steps, msg)
	e.mu.Unlock()
}

func (e *importJobEntry) getSteps() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.steps))
	copy(out, e.steps)
	return out
}

var (
	importJobsMu sync.Mutex
	importJobs   = make(map[string]*importJobEntry)
)

// importDoc is the JSON schema for a project import document.
type importDoc struct {
	Project    string       `json:"project"`
	TaskPrefix string       `json:"task_prefix"`
	Tasks      []importTask `json:"tasks"`
}

type importTask struct {
	Name           string   `json:"name"`
	Title          string   `json:"title"`
	Priority       string   `json:"priority"`
	DependsOn      []string `json:"depends_on"`
	Tags           []string `json:"tags"`
	Description    string   `json:"description"`
	SeparateBranch bool     `json:"separate_branch"`
	BranchName     string   `json:"branch_name"`
}

// ImportProject is a synchronous wrapper: it calls StartImportProject, then
// blocks until the background goroutine finishes, and returns the result.
func (m *taskManager) ImportProject(ctx context.Context, document string) (ImportResult, error) {
	job, err := m.StartImportProject(ctx, document)
	if err != nil {
		return ImportResult{}, err
	}

	for {
		select {
		case <-ctx.Done():
			_ = m.CancelImportProject(ctx, job.ID)
			return ImportResult{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}

		current, err := m.GetImportProjectStatus(ctx, job.ID)
		if err != nil {
			return ImportResult{}, fmt.Errorf("ImportProject: poll: %w", err)
		}
		switch current.Status {
		case importJobStatusCompleted:
			proj, projErr := m.GetProjectByName(ctx, current.ProjectName)
			if projErr != nil {
				return ImportResult{TasksCreated: current.TasksCreated, DepsCreated: current.DepsCreated}, nil
			}
			tasks, _ := m.ListTasksInProject(ctx, proj.ID)
			return ImportResult{
				Project:      proj,
				Tasks:        tasks,
				TasksCreated: current.TasksCreated,
				DepsCreated:  current.DepsCreated,
			}, nil
		case importJobStatusFailed:
			return ImportResult{}, fmt.Errorf("%w: %s", ErrInvalidImport, current.ErrorMessage)
		case importJobStatusCancelled:
			return ImportResult{}, fmt.Errorf("import cancelled")
		}
	}
}

// StartImportProject validates the document, creates an ImportProjectJob
// row, starts the background goroutine, and returns immediately.
func (m *taskManager) StartImportProject(ctx context.Context, document string) (ImportProjectJob, error) {
	if err := validateImportDoc(document); err != nil {
		return ImportProjectJob{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row := gormstore.ImportProjectJobToRow(ImportProjectJob{
		Status:    importJobStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err := m.db.WithContext(ctx).Table(m.tables.ImportProjectJobs).Create(&row).Error; err != nil {
		return ImportProjectJob{}, fmt.Errorf("StartImportProject: create job: %w", err)
	}
	job := gormstore.ImportProjectJobFromRow(row)

	jobCtx, cancel := context.WithCancel(context.Background())
	entry := &importJobEntry{cancel: cancel}
	importJobsMu.Lock()
	importJobs[job.ID] = entry
	importJobsMu.Unlock()

	go m.runImport(jobCtx, job.ID, document, entry)

	return job, nil
}

// GetImportProjectStatus returns the current state of an async import job.
func (m *taskManager) GetImportProjectStatus(ctx context.Context, jobID string) (ImportProjectJob, error) {
	var row gormstore.ImportProjectJobRow
	err := m.db.WithContext(ctx).Table(m.tables.ImportProjectJobs).Where("id = ?", jobID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ImportProjectJob{}, ErrImportJobNotFound
		}
		return ImportProjectJob{}, fmt.Errorf("GetImportProjectStatus: %w", err)
	}
	job := gormstore.ImportProjectJobFromRow(row)

	importJobsMu.Lock()
	entry, ok := importJobs[jobID]
	importJobsMu.Unlock()
	if ok {
		job.ProgressSteps = entry.getSteps()
	}
	return job, nil
}

// CancelImportProject signals the background goroutine to stop.
func (m *taskManager) CancelImportProject(ctx context.Context, jobID string) error {
	job, err := m.GetImportProjectStatus(ctx, jobID)
	if err != nil {
		return err
	}
	switch job.Status {
	case importJobStatusCompleted, importJobStatusFailed, importJobStatusCancelled:
		return ErrImportJobNotCancellable
	}

	importJobsMu.Lock()
	entry, ok := importJobs[jobID]
	importJobsMu.Unlock()
	if ok {
		entry.cancel()
	}
	return m.updateImportJobStatus(context.Background(), jobID, importJobStatusCancelled, "")
}

// ── Background goroutine ──────────────────────────────────────────────────────

func (m *taskManager) runImport(ctx context.Context, jobID, document string, entry *importJobEntry) {
	defer func() {
		importJobsMu.Lock()
		delete(importJobs, jobID)
		importJobsMu.Unlock()
	}()

	if err := m.updateImportJobStatus(ctx, jobID, importJobStatusRunning, ""); err != nil {
		return
	}
	entry.appendStep("Parsing import document…")

	var doc importDoc
	if err := json.Unmarshal([]byte(document), &doc); err != nil {
		m.failImportJob(ctx, jobID, err.Error())
		return
	}

	entry.appendStep(fmt.Sprintf("Creating project %q…", doc.Project))
	proj, err := m.CreateProject(ctx, Project{Name: doc.Project, TaskPrefix: doc.TaskPrefix})
	if err != nil {
		m.failImportJob(ctx, jobID, fmt.Sprintf("create project: %v", err))
		return
	}

	idMap := make(map[string]string, len(doc.Tasks))
	tasksCreated := 0

	for _, it := range doc.Tasks {
		select {
		case <-ctx.Done():
			_ = m.updateImportJobStatus(context.Background(), jobID, importJobStatusCancelled, "")
			return
		default:
		}
		entry.appendStep(fmt.Sprintf("Creating task %q…", it.Name))
		t, err := m.CreateTask(ctx, Task{
			Title:          it.Title,
			Priority:       parsePriority(it.Priority),
			Description:    it.Description,
			TaskName:       it.Name,
			ProjectName:    proj.ProjectName,
			Tags:           it.Tags,
			SeparateBranch: it.SeparateBranch,
			BranchName:     it.BranchName,
		})
		if err != nil {
			m.failImportJob(ctx, jobID, fmt.Sprintf("create task %s: %v", it.Name, err))
			return
		}
		shortKey := strings.TrimPrefix(it.Name, doc.TaskPrefix)
		idMap[shortKey] = t.ID
		tasksCreated++

		if err := m.AddTaskToProject(ctx, t.ID, proj.ID); err != nil {
			m.failImportJob(ctx, jobID, fmt.Sprintf("add task %s to project: %v", it.Name, err))
			return
		}
	}

	depsCreated := 0
	entry.appendStep("Writing dependency edges…")
	for _, it := range doc.Tasks {
		shortKey := strings.TrimPrefix(it.Name, doc.TaskPrefix)
		fromID := idMap[shortKey]
		for _, depShortID := range it.DependsOn {
			toID, ok := idMap[depShortID]
			if !ok {
				continue
			}
			_, err := m.CreateRelationship(ctx, Relationship{
				Label:  RelLabelDependsOn,
				FromID: fromID,
				ToID:   toID,
			})
			if err != nil {
				m.failImportJob(ctx, jobID, fmt.Sprintf("depends_on %s→%s: %v", it.Name, depShortID, err))
				return
			}
			depsCreated++
		}
	}

	// Tag entities + has_tag edges were already written by CreateTask above
	// (via setTaskTags) for each task's own Tags — nothing left to do here.

	entry.appendStep(fmt.Sprintf("Done: %d tasks, %d deps.", tasksCreated, depsCreated))
	now := time.Now().UTC().Format(time.RFC3339)
	_ = m.db.WithContext(context.Background()).Table(m.tables.ImportProjectJobs).Where("id = ?", jobID).
		Updates(map[string]any{
			"status":        importJobStatusCompleted,
			"tasks_created": tasksCreated,
			"deps_created":  depsCreated,
			"project_name":  proj.ProjectName,
			"updated_at":    now,
		}).Error
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *taskManager) updateImportJobStatus(ctx context.Context, jobID, status, errMsg string) error {
	return m.db.WithContext(ctx).Table(m.tables.ImportProjectJobs).Where("id = ?", jobID).
		Updates(map[string]any{
			"status":        status,
			"error_message": errMsg,
			"updated_at":    time.Now().UTC().Format(time.RFC3339),
		}).Error
}

func (m *taskManager) failImportJob(ctx context.Context, jobID, errMsg string) {
	_ = m.updateImportJobStatus(context.Background(), jobID, importJobStatusFailed, errMsg)
}

// validateImportDoc parses and validates the document structure without side effects.
func validateImportDoc(document string) error {
	var doc importDoc
	if err := json.Unmarshal([]byte(document), &doc); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidImport, err.Error())
	}
	if doc.Project == "" {
		return fmt.Errorf("%w: \"project\" field is required", ErrInvalidImport)
	}
	if len(doc.Tasks) == 0 {
		return fmt.Errorf("%w: \"tasks\" array must not be empty", ErrInvalidImport)
	}
	for i, t := range doc.Tasks {
		if t.Name == "" {
			return fmt.Errorf("%w: task[%d] missing \"name\"", ErrInvalidImport, i)
		}
	}
	return nil
}

func parsePriority(s string) TaskPriority {
	switch s {
	case "low":
		return TaskPriorityLow
	case "high":
		return TaskPriorityHigh
	case "critical":
		return TaskPriorityCritical
	default:
		return TaskPriorityMedium
	}
}
