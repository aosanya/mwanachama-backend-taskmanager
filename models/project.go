package models

// Project is an optional container that groups related tasks (e.g. a
// sprint, a milestone, or an epic). Tasks become members via the
// TaskProjectMembership join table — many-to-many; a Task may belong to
// multiple Projects.
type Project struct {
	ID string `json:"id"`

	// Name is the short human-readable label. Required.
	Name string `json:"name"`

	// ProjectName is the URL-safe slug derived from Name: lowercase with
	// spaces replaced by underscores (e.g. "My Sprint" → "my_sprint").
	ProjectName string `json:"project_name"`

	Description string `json:"description,omitempty"`

	// RepoName is the mwanachama-backend-git repository name associated
	// with this project. Used to scope file hydration to the correct repo.
	RepoName string `json:"repo_name,omitempty"`

	// GithubRepo is the canonical GitHub repository, e.g. "owner/name".
	GithubRepo string `json:"github_repo,omitempty"`

	// TaskPrefix is prepended to the auto-generated task name counter when
	// tasks are created via CreateTaskInProject (e.g. "MVP-" → "MVP-001").
	// If empty, defaults to "<project_name>-" at creation time.
	TaskPrefix string `json:"task_prefix,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// EffectiveTaskPrefix returns the prefix to use when auto-generating task
// names for p. If p.TaskPrefix is set it is used directly; otherwise it
// defaults to "<project_name>-".
func (p Project) EffectiveTaskPrefix() string {
	if p.TaskPrefix != "" {
		return p.TaskPrefix
	}
	return p.ProjectName + "-"
}
