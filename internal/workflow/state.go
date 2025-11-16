package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/temirov/gix/internal/audit"
)

const (
	repositoryRefreshMissingInspectionMessageConstant = "repository inspection not available after refresh"
)

// RepositoryState tracks the mutable details for a discovered repository.
type RepositoryState struct {
	Path                  string
	Inspection            audit.RepositoryInspection
	PathDepth             int
	InitialCleanWorktree  bool
	HasNestedRepositories bool
}

// NewRepositoryState constructs repository state from an inspection snapshot.
func NewRepositoryState(inspection audit.RepositoryInspection) *RepositoryState {
	return &RepositoryState{Path: inspection.Path, Inspection: inspection}
}

// Refresh updates the repository inspection data using the supplied audit service.
func (state *RepositoryState) Refresh(executionContext context.Context, service *audit.Service) error {
	if service == nil {
		return nil
	}

	inspections, inspectionError := service.DiscoverInspections(executionContext, []string{state.Path}, false, false, audit.InspectionDepthFull)
	if inspectionError != nil {
		return inspectionError
	}

	if len(inspections) == 0 {
		return errors.New(repositoryRefreshMissingInspectionMessageConstant)
	}

	state.Path = inspections[0].Path
	state.Inspection = inspections[0]
	return nil
}

// State captures the mutable workflow execution context.
type State struct {
	Roots        []string
	Repositories []*RepositoryState
}

// CloneRepositories returns a shallow copy of the repository slice for safe iteration.
func (state *State) CloneRepositories() []*RepositoryState {
	cloned := make([]*RepositoryState, len(state.Repositories))
	copy(cloned, state.Repositories)
	return cloned
}

// UpdateRepositoryPath overwrites the stored repository path at the provided index.
func (state *State) UpdateRepositoryPath(repositoryIndex int, newPath string) error {
	if repositoryIndex < 0 || repositoryIndex >= len(state.Repositories) {
		return fmt.Errorf("repository index %d out of range", repositoryIndex)
	}
	state.Repositories[repositoryIndex].Path = newPath
	return nil
}
