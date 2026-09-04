package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/entitygraph"
)

// effectiveTaskPrefix returns the prefix to use when auto-generating task names
// for p. If p.TaskPrefix is set it is used directly; otherwise it defaults to
// "<project_name>-".
func (p Project) effectiveTaskPrefix() string {
	if p.TaskPrefix != "" {
		return p.TaskPrefix
	}
	return p.ProjectName + "-"
}

// toSlug converts a project name to a URL-safe slug: lowercase with spaces
// replaced by underscores.
func toSlug(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
}

// CreateProject creates a new Project vertex in the graph.
func (m *taskManager) CreateProject(ctx context.Context, p Project) (Project, error) {
	if p.Name == "" {
		return Project{}, fmt.Errorf("%w: Project.Name is required", ErrInvalidTask)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	p.ProjectName = toSlug(p.Name)
	p.CreatedAt = now
	p.UpdatedAt = now
	created, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		TypeID:     projectTypeID,
		Properties: projectToProperties(p),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityAlreadyExists) {
			return Project{}, ErrProjectAlreadyExists
		}
		return Project{}, fmt.Errorf("CreateProject: %w", err)
	}
	return projectFromEntity(created), nil
}

// GetProject reads a single Project by its entity ID.
func (m *taskManager) GetProject(ctx context.Context, projectID string) (Project, error) {
	e, err := m.dm.GetEntity(ctx, projectID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("GetProject: %w", err)
	}
	if e.TypeID != projectTypeID {
		return Project{}, ErrProjectNotFound
	}
	return projectFromEntity(e), nil
}

// GetProjectByName retrieves a Project by its slug (project_name property).
// The caller-supplied projectName is normalized through [toSlug] so that
// display-name casing (e.g. "SharedFarms") resolves to the stored lowercase
// slug ("sharedfarms"). This keeps lookup symmetric with [CreateProject],
// which slugifies via the same helper.
func (m *taskManager) GetProjectByName(ctx context.Context, projectName string) (Project, error) {
	slug := toSlug(projectName)
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		TypeID: projectTypeID,
	})
	if err != nil {
		return Project{}, fmt.Errorf("GetProjectByName: %w", err)
	}
	for _, e := range entities {
		p := projectFromEntity(e)
		if p.ProjectName == slug {
			return p, nil
		}
	}
	return Project{}, ErrProjectNotFound
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

	updated, err := m.dm.UpdateEntity(ctx, p.ID, entitygraph.UpdateEntityRequest{
		Properties: projectToProperties(p),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("UpdateProject: %w", err)
	}
	return projectFromEntity(updated), nil
}

// DeleteProject soft-deletes the Project vertex AND hard-deletes every
// inbound `member_of` edge.
func (m *taskManager) DeleteProject(ctx context.Context, projectID string) error {
	if _, err := m.GetProject(ctx, projectID); err != nil {
		return err
	}
	edges, err := m.TraverseRelationships(ctx, projectID, RelLabelMemberOf, DirectionInbound)
	if err != nil {
		return fmt.Errorf("DeleteProject: traverse: %w", err)
	}
	for _, e := range edges {
		if err := m.dm.DeleteRelationship(ctx, e.ID); err != nil {
			if errors.Is(err, entitygraph.ErrRelationshipNotFound) {
				continue
			}
			return fmt.Errorf("DeleteProject: delete edge %s: %w", e.ID, err)
		}
	}
	if err := m.dm.DeleteEntity(ctx, projectID); err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("DeleteProject: %w", err)
	}
	return nil
}

// ListProjects returns all non-deleted Projects.
func (m *taskManager) ListProjects(ctx context.Context) ([]Project, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		TypeID: projectTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("ListProjects: %w", err)
	}
	out := make([]Project, 0, len(entities))
	for _, e := range entities {
		out = append(out, projectFromEntity(e))
	}
	return out, nil
}

// AddTaskToProject creates the `member_of` edge from taskID to projectID.
func (m *taskManager) AddTaskToProject(ctx context.Context, taskID, projectID string) error {
	_, err := m.CreateRelationship(ctx, Relationship{
		Label:  RelLabelMemberOf,
		FromID: taskID,
		ToID:   projectID,
		Properties: map[string]any{
			"added_at": time.Now().UTC().Format(time.RFC3339),
		},
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
	task.TaskName = fmt.Sprintf("%s%03d", project.effectiveTaskPrefix(), len(existing)+1)
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
