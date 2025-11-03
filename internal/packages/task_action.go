package packages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/workflow"
)

const taskActionPackagesPurge = "repo.packages.purge"

func init() {
	workflow.RegisterTaskAction(taskActionPackagesPurge, handlePackagesPurgeAction)
}

func handlePackagesPurgeAction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	rawService, ok := parameters["service"]
	if !ok {
		return errors.New("packages purge action requires service")
	}
	service, ok := rawService.(PurgeExecutor)
	if !ok || service == nil {
		return errors.New("packages purge action received invalid service")
	}

	rawResolver, ok := parameters["metadata_resolver"]
	if !ok {
		return errors.New("packages purge action requires metadata resolver")
	}
	resolver, ok := rawResolver.(RepositoryMetadataResolver)
	if !ok || resolver == nil {
		return errors.New("packages purge action received invalid metadata resolver")
	}

	tokenSource, ok := parameters["token_source"].(TokenSourceConfiguration)
	if !ok {
		return errors.New("packages purge action requires token source configuration")
	}

	packageOverride, _ := parameters["package_override"].(string)

	dryRun := false
	if value, exists := parameters["dry_run"].(bool); exists {
		dryRun = value
	}

	metadata, metadataError := resolver.ResolveMetadata(ctx, repository.Path)
	if metadataError != nil {
		return fmt.Errorf("packages metadata resolution failed: %w", metadataError)
	}

	packageName := strings.TrimSpace(packageOverride)
	if len(packageName) == 0 {
		packageName = metadata.DefaultPackageName
	}

	options := PurgeOptions{
		Owner:       metadata.Owner,
		PackageName: packageName,
		OwnerType:   metadata.OwnerType,
		TokenSource: tokenSource,
		DryRun:      dryRun,
	}

	_, executionError := service.Execute(ctx, options)
	if executionError != nil {
		return fmt.Errorf("packages purge execution failed: %w", executionError)
	}

	return nil
}
