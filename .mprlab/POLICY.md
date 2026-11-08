# Confident Programming

This policy controls all agent work in this repository.

## Repository Policy Sections

Governor generates this file from its shared template and optional `.mprlab/POLICY.REPOSITORY.md` source.
A repository source section replaces the shared section with the same level-2 heading.
Other repository sections extend this policy.
The repository source requires the title `# Repository Policy Sections` and unique level-2 headings.

- Put repository-specific requirements in the repository source.
- Run the normalizer after a source change.
- Read the generated `.mprlab/POLICY.md` as the complete policy.
- Keep deployment specification and execution policy in the selected application manifest.

## Temporary Task Files

- Put temporary diagnostic files in `_scratch/<task-id>/`.
- Use a unique task identifier for each directory.
- Keep `/_scratch/` in the repository `.gitignore`.
- Record the directory and its task in the task plan before use.
- Keep files from an interrupted task until that task resumes or its owner authorizes removal.
- Before task completion, preserve useful results in maintained tests, tools, or documentation.
- Remove only the temporary directory owned by the completed task.
- Verify that this directory is absent before reporting task completion.
- Preserve directories owned by other tasks.
- If removal fails, report the remaining path and keep task completion pending.

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

## Application Onboarding

These requirements apply to every owned application runtime, including native, browser, service, and container applications.
They are mandatory onboarding requirements.
Governance files alone do not complete application onboarding.

- Read root `AGENTS.md`, this policy, and every applicable guide before the corresponding action.
- Obey their requirements throughout implementation, tests, documentation, and delivery preparation.
- Require working root Make targets for `up`, `down`, and `deploy`.
- Make `up` prepare and start the current local application.
- Make `down` stop only that local application and its owned resources.
- Make an already stopped application a successful `down` result.
- Preserve user data during ordinary start and stop operations.
- Implement repository-owned deployment wiring through inline recipes or repository-owned scripts.
- Require `.mprlab/deploy/resources.yml` and the installed Gateway lifecycle contract.
- Keep local start and stop separate from release, publication, and deployment.
- Reject missing commands, empty targets, placeholder recipes, and deployment wiring that cannot be verified.
- Preserve command failures through each wrapper and public Make target.
- Verify start, stop, isolation, and lifecycle routing through headless integration tests.
- Inject external operation boundaries when actual operations require separate authorization.
- Keep visible applications, desktop input, and production operations outside onboarding validation.
- Run every applicable policy check and record the command and result.
- Resolve failed checks before reporting application onboarding as complete.
- Report remaining requirements as blockers, including their exact owning boundary.

The normalizer checks structural requirements without executing Makefiles or application scripts.
A structural pass does not prove command behavior, policy compliance, or deployment success.
Application ownership requires source review beyond broad language and stack profiles.
For libraries, documentation, or tools without an application runtime, record the source evidence for that scope decision.
Do not create placeholder runtime commands to satisfy an inapplicable application contract.

## Automatic Data Updates And Persistence

This is a binding product principle for every user interface.

- Load current data automatically when a view opens and when its relevant data changes.
- Update stale data automatically after reconnection and when the user returns to the view.
- Never require or expose refresh or reload controls for data synchronization.
- Persist valid user edits automatically. Never require or expose save controls for data persistence.
- Show pending, saved, validation, conflict, and failure states without requiring a manual refresh or save.
- Preserve unsaved edits during background updates, request failures, and concurrent changes.
- Serialize writes and retain newer edits when an earlier write completes.
- Keep explicit controls for user intent, such as creation, deletion, activation, and confirmation of consequential actions.
- Do not use an explicit action as a substitute for automatic data persistence.
- Verify automatic updates, persistence, error recovery, and account isolation through public interface tests.

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

## Device Validation

- Never require a physical device or physical-device access for any task or gate, under any circumstances.
- Apply this prohibition to development, tests, validation, acceptance, issue closure, release, publication, and deployment.
- Never ask the user to provide, obtain, connect, or arrange access to physical devices or physical-device test services.
- Use simulators, emulators, and automated browsers for device validation.
- Accept these environments as sufficient for device validation and acceptance.
- Remove physical-device requirements and blockers from the selected task and its current acceptance records.
- Never replace a removed physical-device gate with another hardware prerequisite.
- Report the actual test environment and observed results without claiming physical-device execution.

## File Permission Boundary

- File permission modes are outside agent scope.
- Never examine, validate, compare, require, change, or record a file permission mode.
- Never use a file permission mode in acceptance, security, credential, execution, publication, deployment, or failure analysis.
- The values `0600` and `7777` have no governance meaning.
- This rule does not change service authorization or operation authority.

## Selected Manifest Contract

Apply this section when the task changes or validates a selected application manifest.

- Keep the selected application manifest versionless.
- Keep `owner`, `release`, and `lifecycle` as the required baseline fields.
- Use the fixed manifest catalogs `artifacts`, `publications`, and `targets`.
- Use the typed `resources` catalog for independently managed provider objects.
- Keep resource definitions separate from their consumers and use typed references.
- Use explicit identity lists and ordered command lists.
- Apply the current iOS baseline below before the manifest preservation rules.
- Apply the approved source cutover before the manifest preservation rules.
- After the cutover, keep each accepted field name, type, requirement, and function.
- A field added to an existing shape must be optional.
- A field added to an existing shape must have one canonical default.
- Normalize that default before manifest identity calculation.
- Add a manifest declaration kind only with one closed shape.
- Reject obsolete root collection shapes, unknown fields, and `schema_version`.

### Current iOS Baseline

The operator selected a fresh iOS contract on September 10, 2026.
Previous iOS adapter acceptance does not constrain this contract.

- Require `build_system: local` and a repository-relative `.mjs` path in `build.ios`.
- Build and sign iOS artifacts locally through the shared native builder.
- Publish the sealed IPA through the shared App Store Connect adapter.
- Reject iOS `.sh` adapters, Xcode Cloud requests, and cloud publication receipts.
- Do not add compatibility dispatch, obsolete aliases, or migration requirements for the removed iOS contract.
- Preserve the declared TestFlight or App Store destination and its export restriction.

This baseline does not authorize removal of stored releases, credentials, or deployment state.

## Static Website Hosting

Apply this section to deployment or publication work for a browser frontend.

- Use GitHub Pages as the production host for each deployable browser frontend.
- Declare the browser frontend in `targets.github_pages.sites[]` in the selected manifest.
- Declare its publication in `publications.github_pages.sites[]`.
- Declare its build output in `artifacts.web_bundles[]`.
- Use `gh-pages` as the publication branch.
- The GitHub Pages repository can differ from the application repository.
- Keep API and service routes on hostnames that differ from the website hostname.
- Reserve the GitHub Pages domain and its `www` hostname for GitHub Pages.
- Treat a container as an artifact source only when its static output goes to GitHub Pages.
- Verify publication through the public website and `/.mprlab-release.json`.
- Run the Governor check after each selected manifest change and before each release, publish, or deploy operation.

## Gateway Lifecycle Execution

Apply this section when the repository uses the selected-manifest lifecycle or owns Gateway infrastructure operations.

- Use the installed `mprlab-gateway` runtime for application and infrastructure operations.
- Use `make deploy` as the full application operator command.
- Delegate it to Gateway v9 `deploy --app-root` for CI, preparation, publication, and deployment.
- When optional `release` or `publish` targets exist, delegate each target to the Gateway command with the same name.
- Keep `app-release`, `app-publish`, and `app-deploy` as internal manifest policy identifiers.
- Pass the selected application Git root through `--app-root`.
- Use `MPRLAB_GATEWAY_EXECUTABLE` for an explicit installed command.
- Preserve application source validation, release policy, resource ownership, private inputs, and sealed receipts.
- Keep each operation and its child calls on one captured runtime package and dependency environment.
- Keep Gateway inventory and private config under `MPRLAB_GATEWAY_OPERATOR_ROOT`.
- Use `$HOME/.config/mprlab-gateway` as the default operator root.
- Select an explicit version with `mprlab-gateway install --version <version>` to change the active runtime.
- Use Gateway source for repository CI, release preparation, and Gateway publication.
- Keep installation, application operations, and operator-controlled production activation as separate gates.

## Manifest Environment Resolution

Use this contract for Gateway lifecycle operations that use a selected manifest.
Manifest processing is the only boundary that reads environment values for Gateway configuration.

- Let manifest values include environment variable references.
- Use the optional `environment.source` field for a relative or absolute environment file path specified in the manifest.
- Use the selected consuming repository as the root for relative paths.
- When the manifest has no `environment.source` field, use only process values.
- Reject a selected file that is missing or that you cannot read, even when process values are available.
- When the two sources have a value for a variable, use its process value.
- Use only the path specified by `environment.source` for an environment file.
- Do not use a default path or examine other paths for environment files.
- Do not make an environment file necessary because of a resource kind or lifecycle phase.
- Read environment values one time at the manifest boundary.
- During manifest processing, change environment variable references to their values.
- Validate the configuration before CI, builds, host contact, or provider effects.
- For each missing necessary reference value, reject the manifest with its variable name and manifest location.
- Keep private values out of error messages and public diagnostics.
- Give the validated configuration to subsequent operations.
- Do not read environment variables or environment files for configuration after manifest processing.
- Do not change environment references to values in downstream Go, Python, Ansible, or shell code.
- Supply deployment, publication, and signing credentials from the validated configuration.
- Keep private configuration values outside public artifacts, plans, receipts, state documents, and logs.
- Keep each selected application's validated configuration separate.
- Validate all selected manifests before application-set effects.
- Use only the selected resources and their specified dependency tree for readiness checks.

Environment files are optional inputs to manifest processing.
They are not a separate deployment specification or a necessary credential store.
Provider-native credential stores are provider inputs under their specified contracts.
They cannot supply Gateway configuration that the selected manifest does not include.

## Credential Discovery

Use this gate when credentials are necessary for the selected task.

- For Gateway lifecycle operations, find credential references in the selected manifest.
- During manifest processing, change those references to their values.
- When a reference has no value, examine only the selected input sources.
- For other commands, examine only credential sources specified in their documentation.
- Examine only the specified variable names.
- Use tracked example and sample environment files as documentation only.
- Do not show or record secret values during diagnosis.
- Do not make copies of secret values during diagnosis.
- Use each provider's non-mutating authentication command when available.
- Tell the operator to supply new credentials only after verification rejects the selected input.
- Show a credential blocker only after this gate finds the missing or incorrect input.
- Keep certificate and key artifact paths specified in the validated configuration.

## Issue Closure

- Close an issue when its required implementation, documentation, and code validation are completed.
- Keep release, publication, deployment, production DNS/TLS, and live acceptance outside implementation issue acceptance criteria.
- Do not keep an implementation issue open or blocked because production operations are not completed.
- Record pending production operations in a runbook or separate operational record.
- Validate deployment tooling and infrastructure configuration through the required isolated integration tests.

Failed or missing required code validation prevents closure.
Production operations outside the task do not remove code validation requirements.

## Native Preparation Authority

The operator selected this contract on October 26, 2026.

- Let CocoaPods and Gradle own dependency resolution and native metadata.
- Use ordinary `pod install` for native dependency preparation.
- Keep preparation records limited to application identity, configuration, dependency versions, toolchain versions, and completed command results.
- Do not use file hashes, fingerprints, whole-file comparisons, or lock restoration as readiness gates.
- Verify required native files and their selected application configuration.
- Permit native metadata changes that retain the required configuration.
- Keep asset and plugin freshness outside configuration verification claims.
- Preserve native command failures, signing checks, application versions, build numbers, and provider results.

## Validation

- Use repository-native `make` targets.
- Do not run a pre-edit or per-issue `make ci` baseline.
- Preserve the expected failing integration-test result as implementation evidence.
- During the change, run the smallest public-entrypoint target that validates the changed contract.
- Run full CI once at the end of the change.
- Use successful full CI from the pull request when its workflow supplies all required checks.
- Open the pull request after focused local checks pass. Let its workflow supply the final full CI result.
- Wait for that result before merge or completion.
- When no pull request supplies full CI, run `make ci` locally after the final change.
- Use focused checks to correct failures.
- Repeat the selected full CI gate only at the next final checkpoint after corrections.
- Reuse successful CI results while the validated code remains unchanged.
- Use equivalent CI results across local validation, pull requests, and release.
- Keep separate release checks for additional artifact, signing, or platform coverage.
- Record the selected CI command, its result, and any required checks outside its coverage.
- Run `make verify` only when the operator explicitly selects the complete qualification lane.
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
- Run the shared `ste.py review` command on changed technical prose.
- Use its dictionary references and findings for the producing agent review.
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
- Use pytest for integration, end-to-end, and focused unit tests under the shared test-driven development rules.

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
