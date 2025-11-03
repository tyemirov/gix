package migrate

import (
	"context"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/githubcli"
)

const (
	pagesUpdateLogMessageConstant      = "Updating GitHub Pages source branch"
	pagesSkipLogMessageConstant        = "GitHub Pages update not required"
	pagesSourceBranchFieldNameConstant = "pages_source_branch"
	pagesTargetBranchFieldNameConstant = "pages_target_branch"
	pagesSourcePathFieldNameConstant   = "pages_source_path"
	pagesBuildTypeFieldNameConstant    = "pages_build_type"
)

// PagesManager coordinates GitHub Pages configuration updates.
type PagesManager struct {
	logger       *zap.Logger
	githubClient GitHubOperations
}

// NewPagesManager constructs a PagesManager.
func NewPagesManager(logger *zap.Logger, client GitHubOperations) *PagesManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PagesManager{logger: logger, githubClient: client}
}

// EnsureLegacyBranch updates Pages configuration when legacy builds target the source branch.
func (manager *PagesManager) EnsureLegacyBranch(executionContext context.Context, config PagesUpdateConfig) (bool, error) {
	if manager.githubClient == nil {
		return false, nil
	}

	status, statusError := manager.githubClient.GetPagesConfig(executionContext, config.RepositoryIdentifier)
	if statusError != nil {
		return false, statusError
	}

	if !status.Enabled {
		manager.logger.Debug(pagesSkipLogMessageConstant, zap.String(pagesBuildTypeFieldNameConstant, string(status.BuildType)))
		return false, nil
	}

	if status.BuildType != githubcli.PagesBuildTypeLegacy {
		manager.logger.Debug(pagesSkipLogMessageConstant, zap.String(pagesBuildTypeFieldNameConstant, string(status.BuildType)))
		return false, nil
	}

	if status.SourceBranch != string(config.SourceBranch) {
		manager.logger.Debug(pagesSkipLogMessageConstant, zap.String(pagesSourceBranchFieldNameConstant, status.SourceBranch))
		return false, nil
	}

	if status.SourceBranch == string(config.TargetBranch) {
		manager.logger.Debug(pagesSkipLogMessageConstant, zap.String(pagesSourceBranchFieldNameConstant, status.SourceBranch))
		return false, nil
	}

	updateError := manager.githubClient.UpdatePagesConfig(executionContext, config.RepositoryIdentifier, githubcli.PagesConfiguration{
		SourceBranch: string(config.TargetBranch),
		SourcePath:   status.SourcePath,
	})
	if updateError != nil {
		return false, updateError
	}

	manager.logger.Info(pagesUpdateLogMessageConstant,
		zap.String(pagesSourceBranchFieldNameConstant, status.SourceBranch),
		zap.String(pagesTargetBranchFieldNameConstant, string(config.TargetBranch)),
		zap.String(pagesSourcePathFieldNameConstant, status.SourcePath),
	)

	return true, nil
}
