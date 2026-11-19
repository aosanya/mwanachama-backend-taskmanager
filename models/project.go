package models

type Project struct {
	ID string `json:"id"`

	Name string `json:"name"`

	ProjectName string `json:"project_name"`

	Description string `json:"description,omitempty"`

	RepoName string `json:"repo_name,omitempty"`

	GithubRepo string `json:"github_repo,omitempty"`

	TaskPrefix string `json:"task_prefix,omitempty"`

	ContractReference string `json:"contract_reference,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	Deleted bool `json:"deleted"`
}

func (p Project) EffectiveTaskPrefix() string {
	if p.TaskPrefix != "" {
		return p.TaskPrefix
	}
	return p.ProjectName + "-"
}
