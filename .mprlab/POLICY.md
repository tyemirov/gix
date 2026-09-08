# Confident Programming

This policy controls all agent work in this repository.

## Operator Rules

- Validate only at edges: I/O, HTTP, CLI, DB adapters, browser bootstrap, imported files, and other external boundaries.
- Design HTTP APIs as resource-oriented REST APIs. Use standard HTTP methods, status codes, and semantics.
- For gRPC APIs, obey protobuf service and RPC conventions. REST constraints do not apply.
- Make illegal states unrepresentable with domain types, smart constructors, dataclasses, enums, or closed action objects.
- Fail fast on impossible states.
- Wrap boundary errors with operation and subject context.
- After boundary validation, do not repeat validation in core modules.
- Keep interfaces narrow. Prefer domain types instead of loose strings, maps, booleans, or `any` values.
- Centralize reusable literals: paths, operation names, event names, config keys, status values, and shared messages.
- Tests target public contracts and invariants, not defensive branches.
- Prefer black-box integration and end-to-end tests through real entry points.

## Test-Driven Development

- Use test-driven development with an inverted test pyramid.
- Integration tests are the primary test layer.
- Use focused unit tests for complex algorithms, calculations, and isolated logic when useful.
- Require integration coverage of public behavior for product acceptance.
- An integration test must use a real public entry point.
- For a behavior change, start with the integration test that represents the required public behavior.
- Run the new or changed integration test before you change production code.
- Confirm that the integration test fails because the required behavior is absent or incorrect.
- After this failure, use focused unit tests to guide complex internal implementation when useful.
- Change the minimum production code necessary to make the integration test pass.
- Refactor only while the applicable integration tests pass.
- For a refactor with no behavior change, run the applicable integration tests before you change production code.
- If focused coverage is absent, add a characterization test before the refactor.
- Use deterministic local infrastructure for repository-owned databases, filesystems, queues, servers, and browsers.
- At an external provider boundary, use a provider sandbox or a local protocol implementation for routine integration tests.
- Keep live-provider acceptance as a separate qualification step.

## Dependency Injection In Integration Tests

- Use dependency injection for integration scenarios that are difficult to reproduce.
- Inject the dependency that creates the test condition.
- Use controlled clocks, dependency failures, or unusual responses when the scenario requires them.
- Keep the product logic under test and its related internal interactions real.
- Assert observable results through the public contract.
- Keep integration coverage with real dependency implementations.
- Injected scenarios prove behavior under the specified conditions. They do not prove actual provider connectivity.
- Qualify actual provider connectivity separately from injected scenarios.

## Prohibited Patterns

- Silent fallbacks, best-effort behavior, legacy aliases, and compatibility reads unless an explicit product requirement says the behavior is current.
- Duplicated validation inside core modules.
- Exporting invalid zero-values as usable domain objects.
- Swallowing errors.
- Increasing waits or timeouts as the primary fix for flakiness.
- Boolean parameters that switch unrelated behaviors.
- Hardcoded workflow, path, event, or message literals when a canonical constant or backend payload exists.

## File Permission Boundary

- File permission modes are outside agent scope.
- Never examine, validate, compare, require, change, or record a file permission mode.
- Never use a file permission mode in acceptance, security, credential, execution, publication, deployment, or failure analysis.
- The values `0600` and `7777` have no governance meaning.
- This rule does not change service authorization or operation authority.

## Selected Manifest Contract

Apply this section when the task changes or validates a selected application manifest.

- Keep the selected application manifest versionless.
- Keep `owner`, `release`, and `resources` as the current baseline fields.
- Each later gateway must accept every manifest that an earlier versionless gateway accepted.
- Keep each accepted field name, type, requirement, and function.
- A field added to an existing shape must be optional.
- A field added to an existing shape must have one canonical default.
- Normalize that default before manifest identity calculation.
- Add a resource kind only with one closed shape.
- Reject unknown fields and `schema_version`.

## Sync Model

Assumptions:

- A valid Git repository has a configured, reachable remote.
- The remote has an existing default branch.
- The remote is the authority for published history.
- The local checkout is a replaceable working copy of remote data.

Work preservation:

- Treat file changes as the work that sync must preserve and publish.
- Include staged, unstaged, and untracked changes. Include unpublished work held in local commits.
- Treat local commits, branch references, and review metadata as mechanisms for that work.
- Replace local state only after its unpublished work is published or preserved.
- Verify file contents and remote results. A local commit identifier alone does not prove publication.

## Sync Branch Selection

- Assume a valid Git repository and a configured, reachable remote with an existing default branch.
- Resolve the default branch from the selected remote symbolic `HEAD`.
- When the user specifies a branch, use that branch.
- Without a destination on the default branch, create a new branch only for uncommitted changes or unpublished local commits.
- Without local work to publish, update the default branch from the remote and keep it active.
- Without a branch argument on another branch, use the current branch.
- Use default-branch status only for this branch-selection rule.
- Treat `main`, `master`, `qqq`, `wwww`, and all other branch names as identifiers with the same behavior.
- Do not use branch protection to change the selected branch.
- Pull the latest remote changes for the selected branch.
- Merge pending changes into that branch and commit the result.
- Push that branch to the selected remote and keep it active.
- If the target still matches a merged pull request, reject dirty auto-commit even when the target is explicit.
- Preserve pending work after this rejection. The user decides how to proceed.

## Sync Review Metadata

- Accept a parent branch without additional commits or a pull request.
- Accept a parent that exists only on the remote.
- Merge incoming parent work before parent publication. Preserve unpublished parent work and pending child files.
- Resolve a merged parent to its current base before synchronizing an unmerged child without a pull request.
- Before review comparison, synchronize the parent with its remote ref and resolved base.
- Keep the child as the destination. Record its current review base.
- Create a pull request only when the branch has file changes against its review base.

## Sync GitHub Publication

- Treat GitHub publication as secondary to the Git synchronization operation.
- Determine publication from the requested branch result in Git output.
- If GitHub rejects that branch, preserve the completed local work and the selected branch.
- Report the rejection and state that the remote did not receive the branch changes.
- Suggest removal of branch protection or creation of a new pull request.
- Do not change branch protection or create a substitute branch because GitHub rejected the push.
- Return success with exit code `0` when completed synchronization has only a GitHub push rejection.
- Do not start rollback for that rejection.
- If another ref is rejected after branch publication succeeds, report that rejection and continue branch review publication.
- Record successful remote updates even when the push command returns a failure.
- Propagate rejected parent publication to the child operation.
- Save the child work locally and defer its push and pull request until parent publication succeeds.
- Require integration tests to verify these results through the CLI.
- Keep a rejected push visible in test assertions. Do not accept rollback as the required result for this case.

## Static Website Hosting

Apply this section to deployment or publication work for a browser frontend.

- Use GitHub Pages as the production host for each deployable browser frontend.
- Declare the browser frontend with a `github_pages` resource in `.mprlab/deploy/resources.yml`.
- Use `gh-pages` as the publication branch.
- The GitHub Pages repository can differ from the application repository.
- Keep API and service routes on hostnames that differ from the website hostname.
- Reserve the GitHub Pages domain and its `www` hostname for GitHub Pages.
- Treat a container as an artifact source only when its static output goes to GitHub Pages.
- Verify publication through the public website and `/.mprlab-release.json`.
- Run the Governor check after each selected manifest change and before each release, publish, or deploy operation.

## Credential Discovery

Apply this gate when the selected task requires credentials.

- Identify each required environment variable from the active command and repository contract.
- Inspect the process environment before you request a login.
- Inspect repository private environment files before you report a credential blocker.
- Include ignored `.env`, `.env.*`, and `*.env` files in the authorized repository roots.
- Treat tracked example and sample environment files as documentation only.
- Search only for exact variable names. Do not print, copy, or record secret values.
- When a command declares one repository environment file as its input, clear
  its owned process variables and source only that file.
- Use the command's standard environment lookup. Do not add a credential parser
  or another input channel.
- When available, use a non-mutating authentication command to verify the discovered value.
- Request new credentials only after each authorized existing input fails verification.
- Report a credential blocker only after you complete this gate.

## Validation

- Preserve the expected failing integration-test result as implementation evidence.
- Use repository-native `make` targets when available.
- Use a satisfactory CI result only when the source code, tests, config, dependencies, and build files stay the same.
- If there is no applicable satisfactory result, run `make ci` once before you change files.
- During the change, run the smallest repository target that validates the changed contract.
- After the last source, test, config, dependency, or build change, run `make ci` once.
- If this run reports an error, run the target that reports the error during the correction.
- After the last correction, run `make ci` once.
- When `make ci` includes `make fmt`, `make lint`, and `make test`, use its result for those targets.
- During the change or error diagnosis, run the necessary component target.
- Run a component target when `make ci` does not include the necessary check.
- For documentation-only work, run the applicable document and repository checks.
- For `.mprlab/`-only work, run the Governor check and `git diff --check`.
- These are the repository checks for `.mprlab/`-only work. Changed prose also requires the documentation-language review.
- For read-only work, use source facts and run only the necessary checks.
- For frontend behavior, verify through a browser test when the behavior is user-visible.
- For services and CLIs, verify through HTTP, CLI, or public API entry points.

## Documentation Language

- Write new or changed English technical prose in ASD-STE100 Simplified Technical English, Issue 9.
- Read `.mprlab/AGENTS.DOCS.md` and `.mprlab/TERMINOLOGY.md` before you write technical prose.
- Apply this rule to PRDs, architecture documents, issues, plans, policies, ADRs, READMEs, runbooks, and API documents.
- Do not change technical meaning to make the language simpler.
- Run the skill `prepare-ste-reference` script to retrieve and verify the official Issue 9 PDF.
- Run the skill `check-ste` script on each technical document that you change.
- The producing agent must review Part 1 writing rules and the Part 2 dictionary.
- Do not assign the reference retrieval or language review to the end user.
- If the official reference is not available, report a blocker and do not claim compliance.

## Language Rules

### Go

- Use smart constructors returning `(Type, error)` when a type has invariants.
- Do not export invalid zero-values.
- Wrap errors with `%w`.
- Prefer integration tests through real HTTP, CLI, or package entry points.
- `make lint` must include `go vet`, `staticcheck`, and `ineffassign` when those tools are part of the repo contract.

### Python

- Use `@dataclass(frozen=True)` or Pydantic when already in use.
- Validate in constructors or edge adapters.
- Use type hints throughout.
- Prefer pytest scenarios through public entry points.
- Use focused unit tests for complex algorithms, calculations, and isolated logic when useful.
- Require integration coverage of public behavior for product acceptance.

### JavaScript And Frontend

- Put `// @ts-check` at the top of new or edited JavaScript modules when the repo uses checked JS.
- Use JSDoc typedefs for domain objects and payload contracts.
- Components render validated state and emit intent.
- Backend clients own request construction and response validation.
- User-visible behavior belongs in browser or integration coverage.

## Self-Check

Before claiming completion:

- External inputs are validated once at the edge.
- Core modules consume validated domain values.
- Error paths include operation and subject context.
- Reusable literals are centralized.
- Public behavior is covered through public entry points.
- Repo-native validation was run or a concrete blocker is documented.
