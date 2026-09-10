package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// toSlug converts a project name to a URL-safe slug: lowercase with spaces
// replaced by underscores.
func toSlug(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
}

// CreateProject creates a new Project row.
func (m *taskManager) CreateProject(ctx context.Context, p Project) (Project, error) {
	if p.Name == "" {
		return Project{}, fmt.Errorf("%w: Project.Name is required", ErrInvalidTask)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	p.ProjectName = toSlug(p.Name)
	p.CreatedAt = now
	p.UpdatedAt = now

	row := gormstore.ProjectToRow(p)
	if err := m.db.WithContext(ctx).Table(m.tables.Projects).Create(&row).Error; err != nil {
		return Project{}, fmt.Errorf("CreateProject: %w", err)
	}
	return gormstore.ProjectFromRow(row), nil
}

// GetProject reads a single Project by its ID.
func (m *taskManager) GetProject(ctx context.Context, projectID string) (Project, error) {
	var row gormstore.ProjectRow
	err := m.db.WithContext(ctx).Table(m.tables.Projects).
		Where("id = ? AND deleted = ?", projectID, false).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("GetProject: %w", err)
	}
	return gormstore.ProjectFromRow(row), nil
}

// GetProjectByName retrieves a Project by its slug (project_name column).
// The caller-supplied projectName is normalized through [toSlug] so that
// display-name casing (e.g. "SharedFarms") resolves to the stored lowercase
// slug ("sharedfarms"), symmetric with [CreateProject].
func (m *taskManager) GetProjectByName(ctx context.Context, projectName string) (Project, error) {
	slug := toSlug(projectName)
	var row gormstore.ProjectRow
	err := m.db.WithContext(ctx).Table(m.tables.Projects).
		Where("project_name = ? AND deleted = ?", slug, false).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("GetProjectByName: %w", err)
	}
	return gormstore.ProjectFromRow(row), nil
}

// UpdateProject patches the mutable fields of an existing Project.
func (m *taskManager) UpdateProject(ctx context.Context, p Project) (Project, error) {
	current, err := m.GetProject(ctx, p.ID)
	if err != nil {
		return Project{}, err
	}
	if p.Name == "" {
		return Project{}, fmt.Errorf("%w: Project.Name is required", ErrInvalidTask)
	}
	p.CreatedAt = current.CreatedAt
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	row := gormstore.ProjectToRow(p)
	if err := m.db.WithContext(ctx).Table(m.tables.Projects).Where("id = ?", p.ID).Save(&row).Error; err != nil {
		return Project{}, fmt.Errorf("UpdateProject: %w", err)
	}
	return gormstore.ProjectFromRow(row), nil
}

// DeleteProject soft-deletes the Project row AND removes every inbound
// `member_of` edge.
func (m *taskManager) DeleteProject(ctx context.Context, projectID string) error {
	if _, err := m.GetProject(ctx, projectID); err != nil {
		return err
	}
	if err := m.db.WithContext(ctx).Table(m.tables.TaskProjectMemberships).
		Where("project_id = ?", projectID).Delete(nil).Error; err != nil {
		return fmt.Errorf("DeleteProject: clear memberships: %w", err)
	}
	if err := m.db.WithContext(ctx).Table(m.tables.Projects).Where("id = ?", projectID).
		UpdateColumn("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteProject: %w", err)
	}
	return nil
}

// ListProjects returns all non-deleted Projects.
func (m *taskManager) ListProjects(ctx context.Context) ([]Project, error) {
	var rows []gormstore.ProjectRow
	if err := m.db.WithContext(ctx).Table(m.tables.Projects).Where("deleted = ?", false).Limit(maxListPage).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListProjects: %w", err)
	}
	out := make([]Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, gormstore.ProjectFromRow(r))
	}
	return out, nil
}

// AddTaskToProject creates the `member_of` edge from taskID to projectID.
func (m *taskManager) AddTaskToProject(ctx context.Context, taskID, projectID string) error {
	_, err := m.CreateRelationship(ctx, Relationship{
		Label:  RelLabelMemberOf,
		FromID: taskID,
		ToID:   projectID,
	})
	if err != nil {
		return fmt.Errorf("AddTaskToProject: %w", err)
	}
	return nil
}

// RemoveTaskFromProject removes the `member_of` edge from taskID to projectID.
func (m *taskManager) RemoveTaskFromProject(ctx context.Context, taskID, projectID string) error {
	return m.DeleteRelationship(ctx, taskID, projectID, RelLabelMemberOf)
}

// ListTasksInProject returns the Tasks that are members of the given project.
func (m *taskManager) ListTasksInProject(ctx context.Context, projectID string) ([]Task, error) {
	edges, err := m.TraverseRelationships(ctx, projectID, RelLabelMemberOf, DirectionInbound)
	if err != nil {
		return nil, fmt.Errorf("ListTasksInProject: traverse: %w", err)
	}
	if len(edges) > maxListPage {
		edges = edges[:maxListPage]
	}
	out := make([]Task, 0, len(edges))
	for _, e := range edges {
		t, err := m.GetTask(ctx, e.FromID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				continue
			}
			return nil, fmt.Errorf("ListTasksInProject: get %s: %w", e.FromID, err)
		}
		out = append(out, t)
	}
	return out, nil
}

// ListProjectsForTask returns the Projects the given Task belongs to.
func (m *taskManager) ListProjectsForTask(ctx context.Context, taskID string) ([]Project, error) {
	edges, err := m.TraverseRelationships(ctx, taskID, RelLabelMemberOf, DirectionOutbound)
	if err != nil {
		return nil, fmt.Errorf("ListProjectsForTask: traverse: %w", err)
	}
	if len(edges) > maxListPage {
		edges = edges[:maxListPage]
	}
	out := make([]Project, 0, len(edges))
	for _, e := range edges {
		p, err := m.GetProject(ctx, e.ToID)
		if err != nil {
			if errors.Is(err, ErrProjectNotFound) {
				continue
			}
			return nil, fmt.Errorf("ListProjectsForTask: get %s: %w", e.ToID, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// GetTaskByName retrieves a task by its project-scoped task_name.
func (m *taskManager) GetTaskByName(ctx context.Context, projectName, taskName string) (Task, error) {
	project, err := m.GetProjectByName(ctx, projectName)
	if err != nil {
		return Task{}, fmt.Errorf("GetTaskByName: resolve project: %w", err)
	}
	tasks, err := m.ListTasksInProject(ctx, project.ID)
	if err != nil {
		return Task{}, fmt.Errorf("GetTaskByName: list: %w", err)
	}
	for _, t := range tasks {
		if t.TaskName == taskName {
			return t, nil
		}
	}
	return Task{}, ErrTaskNotFound
}

// CreateTaskInProject creates a task, auto-generates its task_name from the
// project's task prefix, and writes the member_of edge.
func (m *taskManager) CreateTaskInProject(ctx context.Context, projectName string, task Task) (Task, error) {
	project, err := m.GetProjectByName(ctx, projectName)
	if err != nil {
		return Task{}, fmt.Errorf("CreateTaskInProject: resolve project: %w", err)
	}
	existing, err := m.ListTasksInProject(ctx, project.ID)
	if err != nil {
		return Task{}, fmt.Errorf("CreateTaskInProject: count existing: %w", err)
	}
	task.TaskName = fmt.Sprintf("%s%03d", project.EffectiveTaskPrefix(), len(existing)+1)
	task.ProjectName = projectName

	created, err := m.CreateTask(ctx, task)
	if err != nil {
		return Task{}, fmt.Errorf("CreateTaskInProject: create task: %w", err)
	}
	if err := m.AddTaskToProject(ctx, created.ID, project.ID); err != nil {
		return Task{}, fmt.Errorf("CreateTaskInProject: add member: %w", err)
	}
	return created, nil
}
