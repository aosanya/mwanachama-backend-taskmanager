package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func toSlug(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
}

func (m *taskManager) CreateProject(ctx context.Context, p Project) (Project, error) {
	if p.Name == "" {
		return Project{}, fmt.Errorf("%w: Project.Name is required", ErrInvalidTask)
	}
	reference, err := checkContractReference(p.ContractReference)
	if err != nil {
		return Project{}, err
	}
	p.ContractReference = reference
	now := time.Now().UTC().Format(time.RFC3339)
	p.ID = newSpecID()
	p.ProjectName = toSlug(p.Name)
	p.CreatedAt = now
	p.UpdatedAt = now

	if err := m.checkProjectNameFree(ctx, p.ProjectName, ""); err != nil {
		return Project{}, err
	}
	if err := m.store.Insert(ctx, roleProject, p); err != nil {
		return Project{}, fmt.Errorf("CreateProject: %w", err)
	}
	return p, nil
}

func (m *taskManager) checkProjectNameFree(ctx context.Context, projectName, exceptID string) error {
	q := m.store.Query(ctx, roleProject).
		Where("project_name = ? AND deleted = ?", projectName, false)
	if exceptID != "" {
		q = q.Where("id <> ?", exceptID)
	}
	var holders int64
	if err := q.Count(&holders).Error; err != nil {
		return fmt.Errorf("checkProjectNameFree: %w", err)
	}
	if holders > 0 {
		return fmt.Errorf("%w: %q", ErrProjectAlreadyExists, projectName)
	}
	return nil
}

func (m *taskManager) GetProject(ctx context.Context, projectID string) (Project, error) {
	var p Project
	q := m.store.Query(ctx, roleProject).Where("id = ? AND deleted = ?", projectID, false)
	if err := m.store.Take(q, roleProject, &p, ErrProjectNotFound); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return Project{}, err
		}
		return Project{}, fmt.Errorf("GetProject: %w", err)
	}
	return p, nil
}

func (m *taskManager) GetProjectByName(ctx context.Context, projectName string) (Project, error) {
	slug := toSlug(projectName)
	var p Project
	q := m.store.Query(ctx, roleProject).Where("project_name = ? AND deleted = ?", slug, false)
	if err := m.store.Take(q, roleProject, &p, ErrProjectNotFound); err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return Project{}, err
		}
		return Project{}, fmt.Errorf("GetProjectByName: %w", err)
	}
	return p, nil
}

func (m *taskManager) UpdateProject(ctx context.Context, p Project) (Project, error) {
	current, err := m.GetProject(ctx, p.ID)
	if err != nil {
		return Project{}, err
	}
	if p.Name == "" {
		return Project{}, fmt.Errorf("%w: Project.Name is required", ErrInvalidTask)
	}
	if err := m.checkProjectNameFree(ctx, toSlug(p.Name), p.ID); err != nil {
		return Project{}, err
	}
	reference, err := checkContractReference(p.ContractReference)
	if err != nil {
		return Project{}, err
	}
	p.ContractReference = reference
	p.CreatedAt = current.CreatedAt
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	p.Deleted = false

	row, err := encode(m.store.Object(roleProject), p)
	if err != nil {
		return Project{}, fmt.Errorf("UpdateProject: %w", err)
	}
	res := m.store.Query(ctx, roleProject).Where("id = ? AND deleted = ?", p.ID, false).Updates(row)
	if res.Error != nil {
		return Project{}, fmt.Errorf("UpdateProject: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return Project{}, ErrProjectNotFound
	}
	return p, nil
}

func (m *taskManager) DeleteProject(ctx context.Context, projectID string) error {
	if _, err := m.GetProject(ctx, projectID); err != nil {
		return err
	}
	if err := m.store.Query(ctx, roleMembership).
		Where("project_id = ?", projectID).Delete(nil).Error; err != nil {
		return fmt.Errorf("DeleteProject: clear memberships: %w", err)
	}
	if err := m.store.Query(ctx, roleProjectParty).
		Where("project_id = ?", projectID).Delete(nil).Error; err != nil {
		return fmt.Errorf("DeleteProject: clear parties: %w", err)
	}
	if err := m.store.Query(ctx, roleProject).Where("id = ?", projectID).
		UpdateColumn("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteProject: %w", err)
	}
	return nil
}

func (m *taskManager) ListProjects(ctx context.Context) ([]Project, error) {
	q := m.store.Query(ctx, roleProject).Where("deleted = ?", false).Limit(maxListPage)
	out, err := specstore.List[Project](m.store, q, roleProject)
	if err != nil {
		return nil, fmt.Errorf("ListProjects: %w", err)
	}
	return out, nil
}

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

func (m *taskManager) RemoveTaskFromProject(ctx context.Context, taskID, projectID string) error {
	return m.DeleteRelationship(ctx, taskID, projectID, RelLabelMemberOf)
}

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
	sortByRank(out)
	return out, nil
}

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
