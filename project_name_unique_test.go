package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestCreateProjectRefusesASecondProjectWithTheSameName(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	first, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Probe project"})
	if err != nil {
		t.Fatalf("first CreateProject: %v", err)
	}
	_, err = mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Probe project"})
	if !errors.Is(err, mwanachamataskmanager.ErrProjectAlreadyExists) {
		t.Fatalf("second CreateProject = %v, want ErrProjectAlreadyExists", err)
	}
	listed, err := mgr.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != first.ID {
		t.Fatalf("ListProjects = %d projects, want only the first", len(listed))
	}
}

func TestCreateProjectRefusesANameThatMakesTheSameProjectName(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Probe project"}); err != nil {
		t.Fatalf("first CreateProject: %v", err)
	}
	_, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "PROBE PROJECT"})
	if !errors.Is(err, mwanachamataskmanager.ErrProjectAlreadyExists) {
		t.Fatalf("CreateProject with the same slug = %v, want ErrProjectAlreadyExists", err)
	}
}

func TestCreateProjectAcceptsTheNameOfADeletedProject(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	first, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Probe project"})
	if err != nil {
		t.Fatalf("first CreateProject: %v", err)
	}
	if err := mgr.DeleteProject(ctx, first.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Probe project"}); err != nil {
		t.Fatalf("CreateProject after delete: %v", err)
	}
}

func TestUpdateProjectRefusesTheNameOfAnotherProject(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Probe project"}); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	other, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Other project"})
	if err != nil {
		t.Fatalf("CreateProject other: %v", err)
	}
	other.Name = "Probe project"
	_, err = mgr.UpdateProject(ctx, other)
	if !errors.Is(err, mwanachamataskmanager.ErrProjectAlreadyExists) {
		t.Fatalf("UpdateProject onto a taken name = %v, want ErrProjectAlreadyExists", err)
	}
}

func TestConcurrentCreateProjectsWithTheSameNameMakeExactlyOne(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	const racers = 8
	results := make(chan error, racers)
	for i := 0; i < racers; i++ {
		go func() {
			_, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Raced project"})
			results <- err
		}()
	}
	created, refused := 0, 0
	for i := 0; i < racers; i++ {
		err := <-results
		switch {
		case err == nil:
			created++
		case errors.Is(err, mwanachamataskmanager.ErrProjectAlreadyExists):
			refused++
		default:
			t.Fatalf("CreateProject: %v", err)
		}
	}
	if created != 1 || refused != racers-1 {
		t.Fatalf("created %d and refused %d of %d racers, want one created", created, refused, racers)
	}
}
