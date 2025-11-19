# ISSUES
**Append-only section-based log**

Entries record newly discovered requests or changes, with their outcomes. No instructive content lives here. Read @NOTES.md for the process to follow when fixing issues.

Read @AGENTS.md, @AGENTS.GO.md, @AGENTS.GIT.md @ARCHITECTURE.md, @POLICY.md, @NOTES.md,  @README.md and @ISSUES.md. Start working on open issues. Work autonomously and stack up PRs.

Each issue is formatted as `- [ ] [GX-<number>]`. When resolved it becomes -` [x] [GX-<number>]`

## Features (110–199)

## Improvements (246–299)

## BugFixes (337–399)

- [x] [GX-337] When replacing lines in files only a portion of files is getting the replacement and the rest doesn't. An example is running the @configs/cleanup.yaml flow against this very repo:
```
15:14:40 tyemirov@Vadyms-MacBook-Pro:~/Development/tyemirov/gix - [automation/ns-rewrite/gix-20251118T225204] $ go fmt ./... && go vet ./... && go test ./...
main.go:7:2: no required module provides package github.com/temirov/gix/cmd/cli; to add it:
        go get github.com/temirov/gix/cmd/cli
cmd/cli/application_bootstrap.go:16:2: no required module provides package github.com/temirov/gix/cmd/cli/changelog; to add it:
        go get github.com/temirov/gix/cmd/cli/changelog
cmd/cli/application_bootstrap.go:17:2: no required module provides package github.com/temirov/gix/cmd/cli/commit; to add it:
        go get github.com/temirov/gix/cmd/cli/commit
cmd/cli/application_bootstrap.go:18:2: no required module provides package github.com/temirov/gix/cmd/cli/repos; to add it:
        go get github.com/temirov/gix/cmd/cli/repos
cmd/cli/application_bootstrap.go:19:2: no required module provides package github.com/temirov/gix/cmd/cli/repos/release; to add it:
        go get github.com/temirov/gix/cmd/cli/repos/release
cmd/cli/application_bootstrap.go:20:2: no required module provides package github.com/temirov/gix/cmd/cli/workflow; to add it:
        go get github.com/temirov/gix/cmd/cli/workflow
cmd/cli/application_bootstrap.go:21:2: no required module provides package github.com/temirov/gix/internal/audit; to add it:
        go get github.com/temirov/gix/internal/audit
cmd/cli/application_commands.go:15:2: no required module provides package github.com/temirov/gix/internal/audit/cli; to add it:
        go get github.com/temirov/gix/internal/audit/cli
cmd/cli/application_bootstrap.go:22:2: no required module provides package github.com/temirov/gix/internal/branches; to add it:
        go get github.com/temirov/gix/internal/branches
cmd/cli/application_bootstrap.go:23:2: no required module provides package github.com/temirov/gix/internal/branches/cd; to add it:
        go get github.com/temirov/gix/internal/branches/cd
cmd/cli/application_bootstrap.go:24:2: no required module provides package github.com/temirov/gix/internal/migrate; to add it:
        go get github.com/temirov/gix/internal/migrate
cmd/cli/application_commands.go:18:2: no required module provides package github.com/temirov/gix/internal/migrate/cli; to add it:
        go get github.com/temirov/gix/internal/migrate/cli
cmd/cli/application_bootstrap.go:25:2: no required module provides package github.com/temirov/gix/internal/packages; to add it:
        go get github.com/temirov/gix/internal/packages
cmd/cli/application_bootstrap.go:26:2: no required module provides package github.com/temirov/gix/internal/repos/dependencies; to add it:
        go get github.com/temirov/gix/internal/repos/dependencies
cmd/cli/application_commands.go:20:2: no required module provides package github.com/temirov/gix/internal/repos/prompt; to add it:
        go get github.com/temirov/gix/internal/repos/prompt
cmd/cli/application_commands.go:21:2: no required module provides package github.com/temirov/gix/internal/repos/shared; to add it:
        go get github.com/temirov/gix/internal/repos/shared
cmd/cli/application_bootstrap.go:27:2: no required module provides package github.com/temirov/gix/internal/utils; to add it:
        go get github.com/temirov/gix/internal/utils
cmd/cli/application_bootstrap.go:28:2: no required module provides package github.com/temirov/gix/internal/utils/flags; to add it:
        go get github.com/temirov/gix/internal/utils/flags
cmd/cli/application_bootstrap.go:29:2: no required module provides package github.com/temirov/gix/internal/version; to add it:
        go get github.com/temirov/gix/internal/version
cmd/cli/application_config.go:10:2: no required module provides package github.com/temirov/gix/internal/workflow; to add it:
        go get github.com/temirov/gix/internal/workflow
cmd/cli/changelog/configuration.go:6:2: no required module provides package github.com/temirov/gix/internal/utils/roots; to add it:
        go get github.com/temirov/gix/internal/utils/roots
cmd/cli/changelog/message.go:15:2: no required module provides package github.com/temirov/gix/pkg/llm; to add it:
        go get github.com/temirov/gix/pkg/llm
cmd/cli/changelog/helpers.go:9:2: no required module provides package github.com/temirov/gix/pkg/taskrunner; to add it:
        go get github.com/temirov/gix/pkg/taskrunner
cmd/cli/commit/message.go:11:2: no required module provides package github.com/temirov/gix/internal/commitmsg; to add it:
        go get github.com/temirov/gix/internal/commitmsg
cmd/cli/repos/remove.go:10:2: no required module provides package github.com/temirov/gix/internal/repos/history; to add it:
        go get github.com/temirov/gix/internal/repos/history
cmd/cli/workflow/configuration.go:4:2: no required module provides package github.com/temirov/gix/internal/utils/path; to add it:
        go get github.com/temirov/gix/internal/utils/path
internal/audit/service.go:15:2: no required module provides package github.com/temirov/gix/internal/execshell; to add it:
        go get github.com/temirov/gix/internal/execshell
internal/branches/task_action.go:10:2: no required module provides package github.com/temirov/gix/internal/branches/refresh; to add it:
        go get github.com/temirov/gix/internal/branches/refresh
internal/execshell/executor.go:11:2: no required module provides package github.com/temirov/gix/internal/githubauth; to add it:
        go get github.com/temirov/gix/internal/githubauth
internal/migrate/pages.go:8:2: no required module provides package github.com/temirov/gix/internal/githubcli; to add it:
        go get github.com/temirov/gix/internal/githubcli
internal/migrate/service.go:14:2: no required module provides package github.com/temirov/gix/internal/gitrepo; to add it:
        go get github.com/temirov/gix/internal/gitrepo
internal/packages/command.go:11:2: no required module provides package github.com/temirov/gix/internal/ghcr; to add it:
        go get github.com/temirov/gix/internal/ghcr
internal/releases/service.go:11:2: no required module provides package github.com/temirov/gix/internal/repos/errors; to add it:
        go get github.com/temirov/gix/internal/repos/errors
internal/repos/dependencies/resolve.go:7:2: no required module provides package github.com/temirov/gix/internal/repos/discovery; to add it:
        go get github.com/temirov/gix/internal/repos/discovery
internal/repos/dependencies/resolve.go:8:2: no required module provides package github.com/temirov/gix/internal/repos/filesystem; to add it:
        go get github.com/temirov/gix/internal/repos/filesystem
internal/repos/protocol/executor.go:9:2: no required module provides package github.com/temirov/gix/internal/repos/remotes; to add it:
        go get github.com/temirov/gix/internal/repos/remotes
internal/workflow/task_actions_llm.go:11:2: no required module provides package github.com/temirov/gix/internal/changelog; to add it:
        go get github.com/temirov/gix/internal/changelog
internal/workflow/task_actions.go:13:2: no required module provides package github.com/temirov/gix/internal/releases; to add it:
        go get github.com/temirov/gix/internal/releases
internal/workflow/operations_migrate.go:12:2: no required module provides package github.com/temirov/gix/internal/repos/identity; to add it:
        go get github.com/temirov/gix/internal/repos/identity
internal/workflow/task_actions_namespace.go:11:2: no required module provides package github.com/temirov/gix/internal/repos/namespace; to add it:
        go get github.com/temirov/gix/internal/repos/namespace
internal/workflow/operations_protocol.go:9:2: no required module provides package github.com/temirov/gix/internal/repos/protocol; to add it:
        go get github.com/temirov/gix/internal/repos/protocol
internal/workflow/operations_rename.go:10:2: no required module provides package github.com/temirov/gix/internal/repos/rename; to add it:
        go get github.com/temirov/gix/internal/repos/rename
```
Resolution: Updated workflow replacement planning to walk recursive glob targets so namespace rewrites touch every Go file; covered via new test and passing make lint/test/ci.

- [x] [GX-338] The repository remote is `github.com/tyemirov/gix` (`git remote -v`), but the module path + README badge/install instructions still point to `github.com/temirov/gix`, so `go install github.com/tyemirov/gix@latest` fails with `module declares its path as github.com/temirov/gix` and the release badge points at the wrong owner.
Resolution: Renamed the module + all Go imports to `github.com/tyemirov/gix`, updated the README badge/install instructions/default owner, and re-ran make lint/test/ci to validate the canonical path now matches the remote.

- [x] [GX-339] Documentation (README + ARCHITECTURE) still advertises Go 1.24 support and quick-start instructions say “Go 1.24+”, but `go.mod` now requires Go 1.25, so users compiling with 1.24 hit module version errors.
Resolution: Updated README + ARCHITECTURE to note Go 1.25+, matching go.mod; no code changes required.

- [x] [GX-422] `docs/cli_design.md` still references the old `git-maintenance` binary and module path `github.com/temirov/git-maintenance`, which no longer exist after the rename to `gix` (`github.com/tyemirov/gix`). Doc readers will follow outdated instructions.
Resolution: Updated the CLI design doc to describe the `gix` binary, `github.com/tyemirov/gix` module path, `GIX` env prefix, and config search paths so it matches the shipped tool.

## Maintenance (422–499)

## Planning 
**Do not work on these, not ready**

- [ ] Add an ability to rollback changes. Make flows and complex commands transactional to allow for rollback when a flow that changes things fails
