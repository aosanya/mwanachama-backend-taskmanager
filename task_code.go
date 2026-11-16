package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	defaultCodeSeries = "ISS-"
	numberedIssueKind = "issue"
)

var ErrDuplicateTaskCode = errors.New("another issue of this project already has that code")

var usualCode = regexp.MustCompile(`^([A-Za-z]+-?)([0-9]{1,9})$`)

var titleLeadCode = regexp.MustCompile(`^[A-Za-z]+-?[0-9]+[A-Za-z]?$`)

func splitCode(code string) (string, int) {
	found := usualCode.FindStringSubmatch(code)
	if found == nil {
		return "", 0
	}
	number, err := strconv.Atoi(found[2])
	if err != nil {
		return "", 0
	}
	return found[1], number
}

func codeFromRow(project, linkedRef, title string) (code, source string) {
	if at := strings.LastIndex(linkedRef, "/"); at > 0 {
		owner, id := linkedRef[:at], strings.TrimSpace(linkedRef[at+1:])
		ownedByProject := project == "" || owner == project || strings.HasSuffix(owner, "/"+project)
		if ownedByProject && id != "" && !strings.ContainsAny(id, " \t\n") {
			return id, "board row"
		}
	}
	lead := strings.NewReplacer("**", "", "`", "").Replace(strings.TrimSpace(title))
	if space := strings.IndexAny(lead, " \t"); space > 0 && titleLeadCode.MatchString(lead[:space]) {
		return lead[:space], "title"
	}
	return "", ""
}

type codedIssue struct {
	Project string `gorm:"column:project_name"`
	Code    string `gorm:"column:code"`
	Series  string `gorm:"column:code_series"`
	Number  int    `gorm:"column:code_number"`
	Deleted bool   `gorm:"column:deleted"`
}

type uncodedIssue struct {
	ID        string `gorm:"column:id"`
	Project   string `gorm:"column:project_name"`
	Title     string `gorm:"column:title"`
	LinkedRef string `gorm:"column:linked_ref"`
}

type CodeChange struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Title   string `json:"title"`
	Code    string `json:"code"`
	Source  string `json:"source"`
}

type projectCodes struct {
	Series     string
	Highest    map[string]int
	Changes    []CodeChange
	Unusual    []CodeChange
	Duplicates []CodeChange
}

func planProject(project string, coded []codedIssue, uncoded []uncodedIssue) projectCodes {
	plan := projectCodes{Highest: map[string]int{}}
	held := map[string]bool{}
	seriesCount := map[string]int{}
	note := func(code string) {
		if series, number := splitCode(code); series != "" {
			seriesCount[series]++
			if number > plan.Highest[series] {
				plan.Highest[series] = number
			}
		}
	}
	for _, one := range coded {
		if !one.Deleted {
			held[one.Code] = true
		}
		note(one.Code)
	}

	var unnumbered []uncodedIssue
	for _, one := range uncoded {
		code, source := codeFromRow(project, one.LinkedRef, one.Title)
		if code == "" {
			unnumbered = append(unnumbered, one)
			continue
		}
		change := CodeChange{ID: one.ID, Project: project, Title: one.Title, Code: code, Source: source}
		if held[code] {
			plan.Duplicates = append(plan.Duplicates, change)
			unnumbered = append(unnumbered, one)
			continue
		}
		held[code] = true
		note(code)
		plan.Changes = append(plan.Changes, change)
		if series, _ := splitCode(code); series == "" {
			plan.Unusual = append(plan.Unusual, change)
		}
	}

	plan.Series = commonSeries(seriesCount)
	for _, one := range unnumbered {
		plan.Highest[plan.Series]++
		code := plan.Series + strconv.Itoa(plan.Highest[plan.Series])
		plan.Changes = append(plan.Changes, CodeChange{ID: one.ID, Project: project, Title: one.Title, Code: code, Source: "numbered"})
	}
	return plan
}

func commonSeries(counts map[string]int) string {
	best := defaultCodeSeries
	most := 0
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if counts[name] > most {
			best, most = name, counts[name]
		}
	}
	return best
}

func (m *taskManager) codedIssues(ctx context.Context, tx *gorm.DB, project *string) ([]codedIssue, error) {
	var rows []codedIssue
	q := m.within(tx.WithContext(ctx), roleTask).Select("project_name, code, code_series, code_number, deleted").Where("code <> ?", "")
	if project != nil {
		q = q.Where("project_name = ?", *project)
	}
	err := q.Scan(&rows).Error
	return rows, err
}

func (m *taskManager) uncodedIssues(ctx context.Context, tx *gorm.DB, project *string) ([]uncodedIssue, error) {
	var rows []uncodedIssue
	q := m.within(tx.WithContext(ctx), roleTask).Select("id, project_name, title, linked_ref").Where("code = ?", "").Order(`created_at, "rank", id`)
	if project != nil {
		q = q.Where("project_name = ?", *project)
	}
	err := q.Scan(&rows).Error
	return rows, err
}

func (m *taskManager) planFor(ctx context.Context, tx *gorm.DB, project string) (projectCodes, error) {
	coded, err := m.codedIssues(ctx, tx, &project)
	if err != nil {
		return projectCodes{}, err
	}
	uncoded, err := m.uncodedIssues(ctx, tx, &project)
	if err != nil {
		return projectCodes{}, err
	}
	return planProject(project, coded, uncoded), nil
}

func (m *taskManager) codeIsHeld(ctx context.Context, tx *gorm.DB, project, code, exceptID string) (bool, error) {
	var held int64
	err := m.within(tx.WithContext(ctx), roleTask).
		Where("project_name = ? AND code = ? AND deleted = ? AND id <> ?", project, code, false, exceptID).
		Count(&held).Error
	return held > 0, err
}

func (m *taskManager) refuseHeldCode(ctx context.Context, tx *gorm.DB, task Task) error {
	held, err := m.codeIsHeld(ctx, tx, task.ProjectName, task.Code, task.ID)
	if err != nil {
		return fmt.Errorf("check the code %s: %w", task.Code, err)
	}
	if held {
		return fmt.Errorf("%w: %s", ErrDuplicateTaskCode, task.Code)
	}
	return nil
}

func (m *taskManager) giveNewCode(ctx context.Context, tx *gorm.DB, task *Task) error {
	task.Code = strings.TrimSpace(task.Code)
	if task.Code != "" {
		if err := m.refuseHeldCode(ctx, tx, *task); err != nil {
			return err
		}
		task.CodeSeries, task.CodeNumber = splitCode(task.Code)
		return nil
	}
	plan, err := m.planFor(ctx, tx, task.ProjectName)
	if err != nil {
		return fmt.Errorf("number the issue: %w", err)
	}
	counter := fmt.Sprintf("%s:%s:%s", numberedIssueKind, task.ProjectName, plan.Series)
	for {
		number, err := m.claimNumber(ctx, tx, counter, plan.Highest[plan.Series]+1)
		if err != nil {
			return err
		}
		task.Code = plan.Series + strconv.Itoa(number)
		held, err := m.codeIsHeld(ctx, tx, task.ProjectName, task.Code, task.ID)
		if err != nil {
			return fmt.Errorf("check the code %s: %w", task.Code, err)
		}
		if !held {
			task.CodeSeries, task.CodeNumber = plan.Series, number
			return nil
		}
		plan.Highest[plan.Series] = number
	}
}

func (m *taskManager) insertTask(ctx context.Context, task *Task) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := m.giveNewCode(ctx, tx, task); err != nil {
			return err
		}
		row, err := m.store.Row(roleTask, *task)
		if err != nil {
			return err
		}
		return m.within(tx.WithContext(ctx), roleTask).Create(row).Error
	})
}

func (m *taskManager) settleCodeOnUpdate(ctx context.Context, task *Task, current Task) error {
	task.Code = strings.TrimSpace(task.Code)
	if task.Code == "" {
		task.Code = current.Code
	}
	if task.Code == "" {
		plan, err := m.planFor(ctx, m.db, current.ProjectName)
		if err != nil {
			return fmt.Errorf("number the issue: %w", err)
		}
		for _, change := range plan.Changes {
			if change.ID == current.ID {
				task.Code = change.Code
			}
		}
	}
	if task.Code != current.Code || task.ProjectName != current.ProjectName {
		if err := m.refuseHeldCode(ctx, m.db, *task); err != nil {
			return err
		}
	}
	task.CodeSeries, task.CodeNumber = splitCode(task.Code)
	return nil
}

type CodeBackfill struct {
	Applied    bool         `json:"applied"`
	Read       int          `json:"read"`
	FromRow    int          `json:"from_row"`
	FromTitle  int          `json:"from_title"`
	Numbered   int          `json:"numbered"`
	Unusual    []CodeChange `json:"unusual"`
	Duplicates []CodeChange `json:"duplicates"`
	Changes    []CodeChange `json:"changes"`
}

func BackfillTaskCodes(ctx context.Context, m TaskManager, apply bool) (CodeBackfill, error) {
	impl, ok := m.(*taskManager)
	if !ok || impl == nil {
		return CodeBackfill{}, fmt.Errorf("BackfillTaskCodes: not a task manager of this module")
	}
	return impl.backfillCodes(ctx, apply)
}

func (m *taskManager) backfillCodes(ctx context.Context, apply bool) (CodeBackfill, error) {
	report := CodeBackfill{Applied: apply, Unusual: []CodeChange{}, Duplicates: []CodeChange{}, Changes: []CodeChange{}}
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		coded, err := m.codedIssues(ctx, tx, nil)
		if err != nil {
			return err
		}
		uncoded, err := m.uncodedIssues(ctx, tx, nil)
		if err != nil {
			return err
		}
		report.Read = len(uncoded)

		codedBy := map[string][]codedIssue{}
		for _, one := range coded {
			codedBy[one.Project] = append(codedBy[one.Project], one)
		}
		uncodedBy := map[string][]uncodedIssue{}
		var projects []string
		for _, one := range uncoded {
			if _, seen := uncodedBy[one.Project]; !seen {
				projects = append(projects, one.Project)
			}
			uncodedBy[one.Project] = append(uncodedBy[one.Project], one)
		}
		sort.Strings(projects)

		for _, project := range projects {
			plan := planProject(project, codedBy[project], uncodedBy[project])
			report.Unusual = append(report.Unusual, plan.Unusual...)
			report.Duplicates = append(report.Duplicates, plan.Duplicates...)
			for _, change := range plan.Changes {
				switch change.Source {
				case "board row":
					report.FromRow++
				case "title":
					report.FromTitle++
				default:
					report.Numbered++
				}
				report.Changes = append(report.Changes, change)
				if !apply {
					continue
				}
				series, number := splitCode(change.Code)
				if err := m.within(tx.WithContext(ctx), roleTask).Where("id = ?", change.ID).
					Updates(map[string]any{"code": change.Code, "code_series": series, "code_number": number}).Error; err != nil {
					return fmt.Errorf("write the code of %s: %w", change.ID, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return CodeBackfill{}, fmt.Errorf("BackfillTaskCodes: %w", err)
	}
	return report, nil
}
