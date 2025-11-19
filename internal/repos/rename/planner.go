package rename

import (
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

// DirectoryPlan describes the desired folder arrangement for a repository rename.
type DirectoryPlan struct {
	FolderName   string
	IncludeOwner bool
	owner        *shared.OwnerRepository
}

// DirectoryPlanner computes desired directory plans based on rename preferences.
type DirectoryPlanner struct{}

// NewDirectoryPlanner constructs a planner instance for deriving rename targets.
func NewDirectoryPlanner() DirectoryPlanner {
	return DirectoryPlanner{}
}

// Plan evaluates the desired directory layout for a repository.
func (planner DirectoryPlanner) Plan(includeOwner bool, finalOwnerRepository *shared.OwnerRepository, defaultFolderName string) DirectoryPlan {
	trimmedDefaultFolderName := strings.TrimSpace(defaultFolderName)
	plan := DirectoryPlan{
		FolderName: trimmedDefaultFolderName,
	}

	if !includeOwner || finalOwnerRepository == nil {
		return plan
	}

	ownerSegment := finalOwnerRepository.Owner().String()
	repositorySegment := finalOwnerRepository.Repository().String()

	plan.IncludeOwner = true
	plan.owner = finalOwnerRepository
	plan.FolderName = filepath.Join(ownerSegment, repositorySegment)

	return plan
}

// IsNoop determines whether the repository already resides at the desired location.
func (plan DirectoryPlan) IsNoop(repositoryPath string, currentFolderName string) bool {
	trimmedTarget := strings.TrimSpace(plan.FolderName)
	if len(trimmedTarget) == 0 {
		return true
	}

	if plan.IncludeOwner {
		cleanedRepositoryPath := filepath.Clean(repositoryPath)
		expectedSuffix := filepath.Clean(trimmedTarget)
		return strings.HasSuffix(cleanedRepositoryPath, expectedSuffix)
	}

	return trimmedTarget == strings.TrimSpace(currentFolderName)
}
