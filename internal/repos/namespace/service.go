package namespace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/imports"

	"github.com/temirov/gix/internal/execshell"
	repoerrors "github.com/temirov/gix/internal/repos/errors"
	"github.com/temirov/gix/internal/repos/shared"
)

const (
	defaultBranchPrefix          = "namespace-rewrite"
	defaultCommitMessageTemplate = "chore(namespace): rewrite %s -> %s"
	goFileExtension              = ".go"
	goTestFileSuffix             = "_test.go"
	gitDirectoryName             = ".git"
	vendorDirectoryName          = "vendor"
	gitIgnoredSkipReason         = "namespace rewrite skipped: files ignored by git"
)

var (
	errDependenciesMissing = errors.New("namespace rewrite dependencies missing")
	errInvalidPrefix       = errors.New("namespace prefix invalid")
)

// Dependencies supplies collaborators required to rewrite Go module namespaces.
type Dependencies struct {
	FileSystem        shared.FileSystem
	GitExecutor       shared.GitExecutor
	RepositoryManager shared.GitRepositoryManager
	Clock             shared.Clock
}

func (dependencies Dependencies) sanitize() Dependencies {
	if dependencies.Clock == nil {
		dependencies.Clock = shared.SystemClock{}
	}
	return dependencies
}

// Service orchestrates namespace rewrites using smart constructors and injected dependencies.
type Service struct {
	fileSystem        shared.FileSystem
	gitExecutor       shared.GitExecutor
	repositoryManager shared.GitRepositoryManager
	clock             shared.Clock
}

// NewService constructs a namespace rewrite service.
func NewService(dependencies Dependencies) (*Service, error) {
	sanitized := dependencies.sanitize()
	if sanitized.FileSystem == nil || sanitized.GitExecutor == nil || sanitized.RepositoryManager == nil {
		return nil, repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, "", repoerrors.ErrExecutorDependenciesMissing, errDependenciesMissing)
	}
	return &Service{
		fileSystem:        sanitized.FileSystem,
		gitExecutor:       sanitized.GitExecutor,
		repositoryManager: sanitized.RepositoryManager,
		clock:             sanitized.Clock,
	}, nil
}

// ModulePrefix captures validated module namespace prefixes.
type ModulePrefix struct {
	value string
}

// NewModulePrefix validates and normalizes namespace prefixes.
func NewModulePrefix(raw string) (ModulePrefix, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimSuffix(trimmed, "/")
	if len(trimmed) == 0 {
		return ModulePrefix{}, fmt.Errorf("%w: empty", errInvalidPrefix)
	}
	if !strings.Contains(trimmed, "/") {
		return ModulePrefix{}, fmt.Errorf("%w: missing path separator", errInvalidPrefix)
	}
	return ModulePrefix{value: trimmed}, nil
}

// String exposes the namespace prefix value.
func (prefix ModulePrefix) String() string {
	if len(prefix.value) == 0 {
		panic("namespace.ModulePrefix: zero value")
	}
	return prefix.value
}

// Options configures namespace rewrite execution.
type Options struct {
	RepositoryPath shared.RepositoryPath
	OldPrefix      ModulePrefix
	NewPrefix      ModulePrefix
	BranchPrefix   string
	PushRemote     string
	Push           bool
	CommitMessage  string
	DryRun         bool
}

func (options Options) sanitize(clock shared.Clock) (Options, error) {
	if options.RepositoryPath.String() == "" {
		return Options{}, fmt.Errorf("namespace rewrite: repository path required")
	}
	if options.OldPrefix.value == "" || options.NewPrefix.value == "" {
		return Options{}, fmt.Errorf("namespace rewrite: prefixes required")
	}
	if options.OldPrefix.value == options.NewPrefix.value {
		return Options{}, fmt.Errorf("namespace rewrite: prefixes match")
	}

	branchPrefix := strings.TrimSpace(options.BranchPrefix)
	if len(branchPrefix) == 0 {
		branchPrefix = defaultBranchPrefix
	}

	commitMessage := strings.TrimSpace(options.CommitMessage)
	if len(commitMessage) == 0 {
		commitMessage = fmt.Sprintf(defaultCommitMessageTemplate, options.OldPrefix.String(), options.NewPrefix.String())
	}

	pushRemote := strings.TrimSpace(options.PushRemote)
	if options.Push && len(pushRemote) == 0 {
		pushRemote = "origin"
	}
	if options.Push {
		if _, remoteErr := shared.NewRemoteName(pushRemote); remoteErr != nil {
			return Options{}, fmt.Errorf("namespace rewrite: invalid push remote: %w", remoteErr)
		}
	}

	return Options{
		RepositoryPath: options.RepositoryPath,
		OldPrefix:      options.OldPrefix,
		NewPrefix:      options.NewPrefix,
		BranchPrefix:   branchPrefix,
		PushRemote:     pushRemote,
		Push:           options.Push,
		CommitMessage:  commitMessage,
		DryRun:         options.DryRun,
	}, nil
}

// Result reports namespace rewrite outcomes.
type Result struct {
	BranchName        string
	GoModChanged      bool
	ChangedFiles      []string
	CommitCreated     bool
	PushPerformed     bool
	PushSkippedReason string
	Skipped           bool
	SkipReason        string
}

type changePlan struct {
	goMod       bool
	goFilePaths map[string]struct{}
}

func newChangePlan() changePlan {
	return changePlan{goFilePaths: map[string]struct{}{}}
}

func (plan changePlan) requiresRewrite() bool {
	if plan.goMod {
		return true
	}
	return len(plan.goFilePaths) > 0
}

func (plan changePlan) relativeGoFiles() []string {
	if len(plan.goFilePaths) == 0 {
		return nil
	}
	paths := make([]string, 0, len(plan.goFilePaths))
	for path := range plan.goFilePaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (service *Service) excludeIgnoredPlanEntries(ctx context.Context, repositoryPath string, plan changePlan) (changePlan, error) {
	candidates := make([]string, 0, len(plan.goFilePaths)+1)
	seen := make(map[string]struct{}, len(plan.goFilePaths)+1)

	if plan.goMod {
		candidates = append(candidates, "go.mod")
		seen["go.mod"] = struct{}{}
	}

	for relativePath := range plan.goFilePaths {
		if _, already := seen[relativePath]; already {
			continue
		}
		seen[relativePath] = struct{}{}
		candidates = append(candidates, relativePath)
	}

	ignoredSet, ignoreErr := service.collectIgnoredPaths(ctx, repositoryPath, candidates)
	if ignoreErr != nil {
		return changePlan{}, ignoreErr
	}

	filtered := newChangePlan()
	if plan.goMod {
		if _, ignored := ignoredSet["go.mod"]; !ignored {
			filtered.goMod = true
		}
	}

	for relativePath := range plan.goFilePaths {
		if _, ignored := ignoredSet[relativePath]; ignored {
			continue
		}
		filtered.goFilePaths[relativePath] = struct{}{}
	}

	return filtered, nil
}

// Rewrite applies namespace updates across go.mod and Go source files.
func (service *Service) Rewrite(ctx context.Context, options Options) (Result, error) {
	if service == nil {
		return Result{}, fmt.Errorf("namespace rewrite: %w", errDependenciesMissing)
	}

	sanitized, sanitizeError := options.sanitize(service.clock)
	if sanitizeError != nil {
		return Result{}, sanitizeError
	}

	repositoryPath := sanitized.RepositoryPath.String()
	if sanitized.DryRun {
		return service.planRewrite(repositoryPath, sanitized)
	}

	return service.applyRewrite(ctx, repositoryPath, sanitized)
}

func (service *Service) planRewrite(repositoryPath string, options Options) (Result, error) {
	plan, planError := service.buildChangePlan(repositoryPath, options.OldPrefix)
	if planError != nil {
		return Result{}, planError
	}
	plan, planError = service.excludeIgnoredPlanEntries(context.Background(), repositoryPath, plan)
	if planError != nil {
		return Result{}, planError
	}
	if !plan.requiresRewrite() {
		return Result{Skipped: true, SkipReason: gitIgnoredSkipReason}, nil
	}

	result := Result{
		BranchName:    service.buildBranchName(options.BranchPrefix),
		GoModChanged:  plan.goMod,
		ChangedFiles:  plan.relativeGoFiles(),
		PushPerformed: options.Push, // indicates intent
	}
	return result, nil
}

func (service *Service) applyRewrite(ctx context.Context, repositoryPath string, options Options) (Result, error) {
	plan, planError := service.buildChangePlan(repositoryPath, options.OldPrefix)
	if planError != nil {
		return Result{}, planError
	}
	plan, planError = service.excludeIgnoredPlanEntries(ctx, repositoryPath, plan)
	if planError != nil {
		return Result{}, planError
	}
	if !plan.requiresRewrite() {
		return Result{Skipped: true, SkipReason: gitIgnoredSkipReason}, nil
	}

	if err := service.ensureGitRepository(repositoryPath); err != nil {
		return Result{}, repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrExecutorDependenciesMissing, err)
	}

	clean, cleanError := service.repositoryManager.CheckCleanWorktree(ctx, repositoryPath)
	if cleanError != nil {
		return Result{}, repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrDirtyWorktree, cleanError)
	}
	if !clean {
		return Result{}, repoerrors.WrapMessage(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrDirtyWorktree, "working tree must be clean before namespace rewrite")
	}

	branchName := service.buildBranchName(options.BranchPrefix)
	if err := service.createBranch(ctx, repositoryPath, branchName); err != nil {
		return Result{}, err
	}

	result := Result{
		BranchName:   branchName,
		GoModChanged: plan.goMod,
		ChangedFiles: plan.relativeGoFiles(),
	}

	if err := service.applyFileChanges(repositoryPath, plan, options); err != nil {
		return Result{}, err
	}

	if err := service.stageChanges(ctx, repositoryPath, plan); err != nil {
		return Result{}, err
	}

	changesStaged, stagedError := service.hasStagedChanges(ctx, repositoryPath)
	if stagedError != nil {
		return Result{}, stagedError
	}
	if !changesStaged {
		result.Skipped = true
		result.SkipReason = "no staged changes detected"
		return result, nil
	}

	if err := service.ensureGitIdentity(ctx, repositoryPath); err != nil {
		return Result{}, err
	}

	if err := service.commitChanges(ctx, repositoryPath, options.CommitMessage); err != nil {
		return Result{}, err
	}
	result.CommitCreated = true

	if options.Push {
		performed, skipReason, pushErr := service.pushBranch(ctx, repositoryPath, options.PushRemote, branchName)
		if len(strings.TrimSpace(skipReason)) > 0 {
			result.PushSkippedReason = skipReason
		}
		if pushErr != nil {
			return result, pushErr
		}
		result.PushPerformed = performed
	}

	return result, nil
}

func (service *Service) buildBranchName(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if len(trimmed) == 0 {
		trimmed = defaultBranchPrefix
	}
	sanitized := sanitizeBranchComponent(trimmed)
	timestamp := service.clock.Now().UTC().Format("20060102-150405Z")
	return fmt.Sprintf("%s/%s", sanitized, timestamp)
}

func sanitizeBranchComponent(value string) string {
	replacer := strings.NewReplacer(
		" ", "-",
		"\t", "-",
		"\n", "-",
		"\\", "-",
		"@", "-",
		"#", "-",
	)
	sanitized := replacer.Replace(value)
	sanitized = strings.ReplaceAll(sanitized, "//", "/")
	sanitized = strings.Trim(sanitized, "/")
	if len(sanitized) == 0 {
		return defaultBranchPrefix
	}
	return sanitized
}

func (service *Service) buildChangePlan(repositoryPath string, oldPrefix ModulePrefix) (changePlan, error) {
	plan := newChangePlan()

	goModPath := filepath.Join(repositoryPath, "go.mod")
	goModRequiresRewrite, goModError := service.goModContainsPrefix(goModPath, oldPrefix)
	if goModError != nil {
		return changePlan{}, repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, goModError)
	}
	plan.goMod = goModRequiresRewrite

	fileSet := token.NewFileSet()
	walkErr := filepath.WalkDir(repositoryPath, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == vendorDirectoryName || strings.HasPrefix(name, gitDirectoryName) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), goFileExtension) {
			return nil
		}
		if strings.HasSuffix(entry.Name(), goTestFileSuffix) {
			return nil
		}

		contains, detectErr := service.goFileImportsPrefix(fileSet, path, oldPrefix)
		if detectErr != nil {
			return detectErr
		}
		if !contains {
			return nil
		}

		relativePath, relErr := filepath.Rel(repositoryPath, path)
		if relErr != nil {
			return relErr
		}
		plan.goFilePaths[filepath.ToSlash(relativePath)] = struct{}{}
		return nil
	})
	if walkErr != nil {
		return changePlan{}, repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, walkErr)
	}

	return plan, nil
}

func (service *Service) goModContainsPrefix(goModPath string, oldPrefix ModulePrefix) (bool, error) {
	info, err := service.fileSystem.Stat(goModPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, nil
	}

	data, readErr := service.fileSystem.ReadFile(goModPath)
	if readErr != nil {
		return false, readErr
	}

	file, parseErr := modfile.Parse(goModPath, data, nil)
	if parseErr != nil {
		return false, parseErr
	}

	replacer := namespaceReplacer{old: oldPrefix.String(), new: oldPrefix.String()}

	if file.Module != nil && replacer.hasPrefix(file.Module.Mod.Path) {
		return true, nil
	}

	for _, require := range file.Require {
		if require == nil {
			continue
		}
		if replacer.hasPrefix(require.Mod.Path) {
			return true, nil
		}
	}

	for _, replace := range file.Replace {
		if replace == nil {
			continue
		}
		if replacer.hasPrefix(replace.Old.Path) || replacer.hasPrefix(replace.New.Path) {
			return true, nil
		}
	}

	for _, exclude := range file.Exclude {
		if exclude == nil {
			continue
		}
		if replacer.hasPrefix(exclude.Mod.Path) {
			return true, nil
		}
	}

	return false, nil
}

func (service *Service) goFileImportsPrefix(fileSet *token.FileSet, absolutePath string, prefix ModulePrefix) (bool, error) {
	file, parseErr := parser.ParseFile(fileSet, absolutePath, nil, parser.ImportsOnly)
	if parseErr != nil {
		return false, parseErr
	}
	for _, spec := range file.Imports {
		if spec.Path == nil {
			continue
		}
		unquoted := strings.Trim(spec.Path.Value, `"`)
		if strings.HasPrefix(unquoted, prefix.String()+"/") {
			return true, nil
		}
	}
	return false, nil
}

func (service *Service) ensureGitRepository(repositoryPath string) error {
	gitPath := filepath.Join(repositoryPath, gitDirectoryName)
	info, err := service.fileSystem.Stat(gitPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("git metadata missing at %s", gitPath)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("git metadata path %s is not a directory", gitPath)
	}
	return nil
}

func (service *Service) createBranch(ctx context.Context, repositoryPath string, branchName string) error {
	details := execshell.CommandDetails{
		Arguments:        []string{"checkout", "-b", branchName},
		WorkingDirectory: repositoryPath,
	}
	if _, err := service.gitExecutor.ExecuteGit(ctx, details); err != nil {
		return repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrCreateBranchFailed, err)
	}
	return nil
}

func (service *Service) applyFileChanges(repositoryPath string, plan changePlan, options Options) error {
	if plan.goMod {
		goModPath := filepath.Join(repositoryPath, "go.mod")
		if err := service.rewriteGoMod(goModPath, options.OldPrefix, options.NewPrefix); err != nil {
			return repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, err)
		}
	}

	if len(plan.goFilePaths) == 0 {
		return nil
	}

	for _, relativePath := range plan.relativeGoFiles() {
		absolutePath := filepath.Join(repositoryPath, relativePath)
		if err := service.rewriteGoFile(absolutePath, options.OldPrefix, options.NewPrefix); err != nil {
			return repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, fmt.Errorf("rewrite %s: %w", relativePath, err))
		}
	}
	return nil
}

func (service *Service) rewriteGoMod(goModPath string, oldPrefix ModulePrefix, newPrefix ModulePrefix) error {
	content, readErr := service.fileSystem.ReadFile(goModPath)
	if readErr != nil {
		return readErr
	}

	file, parseErr := modfile.Parse("go.mod", content, nil)
	if parseErr != nil {
		return parseErr
	}

	replacer := namespaceReplacer{old: oldPrefix.String(), new: newPrefix.String()}
	didChange := false

	if file.Module != nil {
		if updated, changed := replacer.replace(file.Module.Mod.Path); changed {
			file.Module.Mod.Path = updated
			if syntax := file.Module.Syntax; syntax != nil && len(syntax.Token) >= 2 {
				syntax.Token[1] = modfile.AutoQuote(updated)
			}
			didChange = true
		}
	}

	for _, require := range file.Require {
		if require == nil {
			continue
		}
		if updated, changed := replacer.replace(require.Mod.Path); changed {
			require.Mod.Path = updated
			if syntax := require.Syntax; syntax != nil {
				token := modfile.AutoQuote(updated)
				if syntax.InBlock {
					if len(syntax.Token) >= 1 {
						syntax.Token[0] = token
					}
				} else if len(syntax.Token) >= 2 {
					syntax.Token[1] = token
				}
			}
			didChange = true
		}
	}

	for _, replace := range file.Replace {
		if replace == nil {
			continue
		}
		if updated, changed := replacer.replace(replace.Old.Path); changed {
			replace.Old.Path = updated
			if syntax := replace.Syntax; syntax != nil {
				token := modfile.AutoQuote(updated)
				if syntax.InBlock {
					if len(syntax.Token) >= 1 {
						syntax.Token[0] = token
					}
				} else if len(syntax.Token) >= 2 {
					syntax.Token[1] = token
				}
			}
			didChange = true
		}
		if updated, changed := replacer.replace(replace.New.Path); changed {
			replace.New.Path = updated
			if syntax := replace.Syntax; syntax != nil {
				token := modfile.AutoQuote(updated)
				if syntax.InBlock {
					if len(syntax.Token) >= 3 {
						syntax.Token[2] = token
					}
				} else if len(syntax.Token) >= 4 {
					syntax.Token[3] = token
				}
			}
			didChange = true
		}
	}

	for _, exclude := range file.Exclude {
		if exclude == nil {
			continue
		}
		if updated, changed := replacer.replace(exclude.Mod.Path); changed {
			exclude.Mod.Path = updated
			if syntax := exclude.Syntax; syntax != nil {
				token := modfile.AutoQuote(updated)
				if syntax.InBlock {
					if len(syntax.Token) >= 1 {
						syntax.Token[0] = token
					}
				} else if len(syntax.Token) >= 2 {
					syntax.Token[1] = token
				}
			}
			didChange = true
		}
	}

	if !didChange {
		return nil
	}

	file.Cleanup()
	formatted, formatErr := file.Format()
	if formatErr != nil {
		return formatErr
	}

	mode := filePermissionsOrDefault(service.fileSystem, goModPath, 0o644)
	return service.fileSystem.WriteFile(goModPath, formatted, mode)
}

type namespaceReplacer struct {
	old string
	new string
}

func (replacer namespaceReplacer) replace(path string) (string, bool) {
	if path == "" {
		return path, false
	}
	if path == replacer.old {
		return replacer.new, replacer.old != replacer.new
	}
	oldWithSlash := replacer.old + "/"
	if strings.HasPrefix(path, oldWithSlash) {
		return replacer.new + path[len(replacer.old):], true
	}
	return path, false
}

func (replacer namespaceReplacer) hasPrefix(path string) bool {
	if path == "" {
		return false
	}
	if path == replacer.old {
		return true
	}
	return strings.HasPrefix(path, replacer.old+"/")
}

func (service *Service) rewriteGoFile(absolutePath string, oldPrefix ModulePrefix, newPrefix ModulePrefix) error {
	original, readErr := service.fileSystem.ReadFile(absolutePath)
	if readErr != nil {
		return readErr
	}

	fileSet := token.NewFileSet()
	astFile, parseErr := parser.ParseFile(fileSet, absolutePath, original, parser.ParseComments)
	if parseErr != nil {
		return parseErr
	}

	didChange := false
	for _, spec := range astFile.Imports {
		if spec.Path == nil {
			continue
		}
		unquoted := strings.Trim(spec.Path.Value, `"`)
		if !strings.HasPrefix(unquoted, oldPrefix.String()+"/") {
			continue
		}
		newValue := newPrefix.String() + strings.TrimPrefix(unquoted, oldPrefix.String())
		if newValue != unquoted {
			spec.Path.Value = `"` + newValue + `"`
			didChange = true
		}
	}
	if !didChange {
		return nil
	}

	var formattedBuffer bytes.Buffer
	config := &printer.Config{Mode: printer.TabIndent | printer.UseSpaces, Tabwidth: 8}
	if err := config.Fprint(&formattedBuffer, fileSet, astFile); err != nil {
		return err
	}

	formatted, formatErr := imports.Process(absolutePath, formattedBuffer.Bytes(), &imports.Options{
		Comments:   true,
		Fragment:   false,
		FormatOnly: false,
		TabIndent:  true,
		TabWidth:   8,
	})
	if formatErr != nil {
		return formatErr
	}

	mode := filePermissionsOrDefault(service.fileSystem, absolutePath, 0o644)
	return service.fileSystem.WriteFile(absolutePath, formatted, mode)
}

func filePermissionsOrDefault(fs shared.FileSystem, path string, fallback fs.FileMode) fs.FileMode {
	if fs == nil {
		return fallback
	}
	info, err := fs.Stat(path)
	if err != nil {
		return fallback
	}
	mode := info.Mode()
	if mode == 0 {
		return fallback
	}
	return mode
}

func (service *Service) stageChanges(ctx context.Context, repositoryPath string, plan changePlan) error {
	if plan.goMod {
		if err := service.gitAdd(ctx, repositoryPath, "go.mod"); err != nil {
			return err
		}
	}
	for _, relativePath := range plan.relativeGoFiles() {
		if err := service.gitAdd(ctx, repositoryPath, relativePath); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) gitAdd(ctx context.Context, repositoryPath string, relativePath string) error {
	details := execshell.CommandDetails{
		Arguments:        []string{"add", relativePath},
		WorkingDirectory: repositoryPath,
	}
	if _, err := service.gitExecutor.ExecuteGit(ctx, details); err != nil {
		return repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, err)
	}
	return nil
}

func (service *Service) collectIgnoredPaths(ctx context.Context, repositoryPath string, paths []string) (map[string]struct{}, error) {
	ignored := make(map[string]struct{})
	if len(paths) == 0 {
		return ignored, nil
	}

	details := execshell.CommandDetails{
		Arguments:        []string{"check-ignore", "--stdin"},
		WorkingDirectory: repositoryPath,
		StandardInput:    []byte(strings.Join(paths, "\n")),
	}

	result, err := service.gitExecutor.ExecuteGit(ctx, details)
	if err != nil {
		var commandError execshell.CommandFailedError
		if errors.As(err, &commandError) && commandError.Result.ExitCode == 1 {
			return ignored, nil
		}
		return nil, repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, err)
	}

	for _, line := range strings.Split(result.StandardOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		ignored[trimmed] = struct{}{}
	}

	return ignored, nil
}

func (service *Service) hasStagedChanges(ctx context.Context, repositoryPath string) (bool, error) {
	details := execshell.CommandDetails{
		Arguments:        []string{"diff", "--cached", "--quiet"},
		WorkingDirectory: repositoryPath,
	}
	_, err := service.gitExecutor.ExecuteGit(ctx, details)
	if err == nil {
		return false, nil
	}

	var commandError execshell.CommandFailedError
	if errors.As(err, &commandError) && commandError.Result.ExitCode == 1 {
		return true, nil
	}
	return false, repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, err)
}

func (service *Service) ensureGitIdentity(ctx context.Context, repositoryPath string) error {
	useConfigOnly, configErr := service.gitConfigBool(ctx, repositoryPath, "user.useConfigOnly")
	if configErr != nil {
		return repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, configErr)
	}
	if !useConfigOnly {
		return nil
	}

	nameConfigured, nameErr := service.gitConfigLocalHasValue(ctx, repositoryPath, "user.name")
	if nameErr != nil {
		return repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, nameErr)
	}
	emailConfigured, emailErr := service.gitConfigLocalHasValue(ctx, repositoryPath, "user.email")
	if emailErr != nil {
		return repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, emailErr)
	}
	if !nameConfigured || !emailConfigured {
		return repoerrors.WrapMessage(
			repoerrors.OperationNamespaceRewrite,
			repositoryPath,
			repoerrors.ErrNamespaceRewriteFailed,
			"git local identity (user.name and user.email) required when user.useConfigOnly is true",
		)
	}
	return nil
}

func (service *Service) gitConfigBool(ctx context.Context, repositoryPath string, key string) (bool, error) {
	details := execshell.CommandDetails{
		Arguments:        []string{"config", "--bool", key},
		WorkingDirectory: repositoryPath,
	}
	result, err := service.gitExecutor.ExecuteGit(ctx, details)
	if err == nil {
		return strings.EqualFold(strings.TrimSpace(result.StandardOutput), "true"), nil
	}

	var commandError execshell.CommandFailedError
	if errors.As(err, &commandError) && commandError.Result.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

func (service *Service) gitConfigLocalHasValue(ctx context.Context, repositoryPath string, key string) (bool, error) {
	details := execshell.CommandDetails{
		Arguments:        []string{"config", "--local", "--get", key},
		WorkingDirectory: repositoryPath,
	}
	result, err := service.gitExecutor.ExecuteGit(ctx, details)
	if err == nil {
		return strings.TrimSpace(result.StandardOutput) != "", nil
	}

	var commandError execshell.CommandFailedError
	if errors.As(err, &commandError) && commandError.Result.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

func (service *Service) commitChanges(ctx context.Context, repositoryPath string, message string) error {
	details := execshell.CommandDetails{
		Arguments:        []string{"commit", "-m", message},
		WorkingDirectory: repositoryPath,
	}
	if _, err := service.gitExecutor.ExecuteGit(ctx, details); err != nil {
		return repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespaceRewriteFailed, err)
	}
	return nil
}

func (service *Service) pushBranch(ctx context.Context, repositoryPath string, remote string, branch string) (bool, string, error) {
	remoteURLKey := fmt.Sprintf("remote.%s.url", remote)
	remoteCheck := execshell.CommandDetails{
		Arguments:        []string{"config", "--get", remoteURLKey},
		WorkingDirectory: repositoryPath,
	}
	if _, err := service.gitExecutor.ExecuteGit(ctx, remoteCheck); err != nil {
		reason := fmt.Sprintf("remote %s not configured", remote)
		wrapped := repoerrors.WrapMessage(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrRemoteMissing, reason)
		return false, reason, wrapped
	}

	headResult, headErr := service.gitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{"rev-parse", "HEAD"},
		WorkingDirectory: repositoryPath,
	})
	if headErr == nil {
		remoteResult, remoteErr := service.gitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
			Arguments:        []string{"ls-remote", "--heads", remote, branch},
			WorkingDirectory: repositoryPath,
		})
		if remoteErr == nil {
			remoteOutput := strings.TrimSpace(remoteResult.StandardOutput)
			localHash := strings.TrimSpace(headResult.StandardOutput)
			if len(remoteOutput) > 0 && len(localHash) > 0 {
				fields := strings.Fields(remoteOutput)
				if len(fields) > 0 && fields[0] == localHash {
					reason := fmt.Sprintf("branch %s already up to date on %s", branch, remote)
					return false, reason, nil
				}
			}
		}
	}

	pushDetails := execshell.CommandDetails{
		Arguments:        []string{"push", "--set-upstream", remote, branch},
		WorkingDirectory: repositoryPath,
	}
	if _, err := service.gitExecutor.ExecuteGit(ctx, pushDetails); err != nil {
		reason := "push failed; see error log"
		return false, reason, repoerrors.Wrap(repoerrors.OperationNamespaceRewrite, repositoryPath, repoerrors.ErrNamespacePushFailed, err)
	}
	return true, "", nil
}
