package packages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/ghcr"
	"github.com/tyemirov/gix/internal/workflow"
)

const taskActionPackagesRetention = "repo.packages.retention"

func init() {
	workflow.RegisterTaskAction(taskActionPackagesRetention, handlePackagesRetentionAction)
}

func handlePackagesRetentionAction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	rawService, ok := parameters["service"]
	if !ok {
		return errors.New("packages retention action requires service")
	}
	service, ok := rawService.(RetentionExecutor)
	if !ok || service == nil {
		return errors.New("packages retention action received invalid service")
	}

	rawResolver, ok := parameters["metadata_resolver"]
	if !ok {
		return errors.New("packages retention action requires metadata resolver")
	}
	resolver, ok := rawResolver.(RepositoryMetadataResolver)
	if !ok || resolver == nil {
		return errors.New("packages retention action received invalid metadata resolver")
	}

	credential, ok := parameters["credential"].(string)
	if !ok || strings.TrimSpace(credential) == "" {
		return errors.New("packages retention action requires credential")
	}

	packageOverride, _ := parameters["package_override"].(string)
	keepCount, keepCountAvailable := parameters["keep_count"].(ghcr.KeepCount)
	if !keepCountAvailable {
		return errors.New("packages retention action requires keep count")
	}
	if _, keepCountError := ghcr.NewKeepCount(keepCount.Value()); keepCountError != nil {
		return keepCountError
	}

	metadata, metadataError := resolver.ResolveMetadata(ctx, repository.Path)
	if metadataError != nil {
		return fmt.Errorf("packages metadata resolution failed: %w", metadataError)
	}

	packageName := strings.TrimSpace(packageOverride)
	if len(packageName) == 0 {
		packageName = metadata.DefaultPackageName
	}

	options := RetentionOptions{
		Owner:       metadata.Owner,
		PackageName: packageName,
		OwnerType:   metadata.OwnerType,
		Credential:  credential,
		Keep:        keepCount,
	}

	_, executionError := service.Execute(ctx, options)
	if executionError != nil {
		return fmt.Errorf("packages retention execution failed: %w", executionError)
	}

	return nil
}
