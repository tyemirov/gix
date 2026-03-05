// @ts-check

/**
 * @typedef {{
 *   launch_path?: string,
 *   launch_mode?: string,
 *   selected_repository_id?: string,
 *   repositories?: RepositoryDescriptor[],
 *   error?: string,
 * }} RepositoryCatalog
 */

/**
 * @typedef {{
 *   id: string,
 *   name: string,
 *   path: string,
 *   current_branch?: string,
 *   default_branch?: string,
 *   dirty: boolean,
 *   context_current: boolean,
 *   error?: string,
 * }} RepositoryDescriptor
 */

/**
 * @typedef {{
 *   repository_id?: string,
 *   repository_path?: string,
 *   branches?: BranchDescriptor[],
 *   error?: string,
 * }} BranchCatalog
 */

/**
 * @typedef {{
 *   name: string,
 *   current: boolean,
 *   upstream?: string,
 * }} BranchDescriptor
 */

/**
 * @typedef {{
 *   application: string,
 *   commands: CommandDescriptor[],
 * }} CommandCatalog
 */

/**
 * @typedef {{
 *   group?: string,
 *   repository: string,
 *   ref: string,
 *   path: string,
 *   supports_batch: boolean,
 *   draft_template?: string,
 * }} CommandTargetDescriptor
 */

/**
 * @typedef {{
 *   path: string,
 *   use: string,
 *   name: string,
 *   short?: string,
 *   long?: string,
 *   example?: string,
 *   aliases?: string[],
 *   runnable: boolean,
 *   actionable: boolean,
 *   target: CommandTargetDescriptor,
 *   flags?: FlagDescriptor[],
 *   subcommands?: string[],
 * }} CommandDescriptor
 */

/**
 * @typedef {{
 *   name: string,
 *   shorthand?: string,
 *   usage?: string,
 *   type?: string,
 *   default?: string,
 *   no_opt_default?: string,
 *   required: boolean,
 * }} FlagDescriptor
 */

/**
 * @typedef {{
 *   id: string,
 *   arguments: string[],
 *   status: string,
 *   stdout: string,
 *   stderr: string,
 *   error?: string,
 *   exit_code: number,
 *   started_at: string,
 *   completed_at?: string,
 * }} RunSnapshot
 */

/**
 * @typedef {{
 *   id: string,
 *   title: string,
 *   description: string,
 * }} CommandGroupDefinition
 */

const repositoriesEndpoint = "/api/repos";
const commandsEndpoint = "/api/commands";
const runsEndpoint = "/api/runs";
const pollIntervalMilliseconds = 800;
const currentRepoLaunchMode = "current_repo";
const scopeSelectedValue = "selected";
const scopeCheckedValue = "checked";
const scopeAllValue = "all";
const refModeCurrentValue = "current";
const refModeDefaultValue = "default";
const refModeNamedValue = "named";
const refModePatternValue = "pattern";
const refModeAnyValue = "any";
const pathModeNoneValue = "none";
const pathModeRelativeValue = "relative";
const pathModeGlobValue = "glob";
const pathModeMultipleValue = "multiple";
const targetRequirementNoneValue = "none";
const targetRequirementOptionalValue = "optional";
const targetRequirementRequiredValue = "required";
const commandGroupBranchValue = "branch";
const commandGroupRepositoryValue = "repository";
const commandGroupRemoteValue = "remote";
const commandGroupPullRequestsValue = "prs";
const commandGroupPackagesValue = "packages";
const commandGroupFilesValue = "files";
const commandGroupGeneralValue = "general";
const draftTemplateFilesAddValue = "files_add";
const draftTemplateFilesReplaceValue = "files_replace";
const draftTemplateFilesRemoveValue = "files_remove";
const commandPathVersionValue = "gix version";
const commandPathAuditValue = "gix audit";
const commandPathBranchChangeValue = "gix cd";
const commandPathDefaultValue = "gix default";
const pathPlaceholderRelativeValue = "RELATIVE/PATH";
const pathPlaceholderGlobValue = "**/*";
const pathPlaceholderMultipleValue = "PATH/ONE\nPATH/TWO";

/** @type {CommandGroupDefinition[]} */
const commandGroupDefinitions = [
  { id: commandGroupBranchValue, title: "Branch Flow", description: "Switch branches and promote branch state across the target repositories." },
  { id: commandGroupRepositoryValue, title: "Repository", description: "Inspect and normalize repository state." },
  { id: commandGroupRemoteValue, title: "Remote", description: "Align remotes and transport settings." },
  { id: commandGroupPullRequestsValue, title: "Pull Requests", description: "Clean up local and remote PR branches." },
  { id: commandGroupPackagesValue, title: "Packages", description: "Prune package artifacts tied to the repository." },
  { id: commandGroupFilesValue, title: "Files", description: "Draft file additions, replacements, and removals across repository targets." },
  { id: commandGroupGeneralValue, title: "General", description: "Commands that are not tied to a repository target." },
];

/** @type {{
 *   repositoryCatalog: RepositoryCatalog | null,
 *   repositories: RepositoryDescriptor[],
 *   checkedRepositoryIDs: string[],
 *   selectedRepositoryID: string,
 *   selectedScope: string,
 *   targetRefMode: string,
 *   targetRefValue: string,
 *   targetPathMode: string,
 *   targetPathValue: string,
 *   branches: BranchDescriptor[],
 *   allCommands: CommandDescriptor[],
 *   actionableCommands: CommandDescriptor[],
 *   selectedPath: string,
 *   pollTimer: number | null,
 * }} */
const state = {
  repositoryCatalog: null,
  repositories: [],
  checkedRepositoryIDs: [],
  selectedRepositoryID: "",
  selectedScope: scopeSelectedValue,
  targetRefMode: refModeCurrentValue,
  targetRefValue: "",
  targetPathMode: pathModeNoneValue,
  targetPathValue: "",
  branches: [],
  allCommands: [],
  actionableCommands: [],
  selectedPath: "",
  pollTimer: null,
};

const elements = {
  repoCount: document.querySelector("#repo-count"),
  repoLaunchSummary: document.querySelector("#repo-launch-summary"),
  repoFilter: document.querySelector("#repo-filter"),
  repoList: document.querySelector("#repo-list"),
  repoTitle: document.querySelector("#repo-title"),
  repoPath: document.querySelector("#repo-path"),
  repoSummary: document.querySelector("#repo-summary"),
  repoStateTokens: document.querySelector("#repo-state-tokens"),
  targetRepoSummary: document.querySelector("#target-repo-summary"),
  targetRepoDetail: document.querySelector("#target-repo-detail"),
  scopeSelected: document.querySelector("#scope-selected"),
  scopeChecked: document.querySelector("#scope-checked"),
  scopeAll: document.querySelector("#scope-all"),
  targetRefSummary: document.querySelector("#target-ref-summary"),
  targetRefMode: document.querySelector("#target-ref-mode"),
  targetRefValue: document.querySelector("#target-ref-value"),
  targetPathSummary: document.querySelector("#target-path-summary"),
  targetPathMode: document.querySelector("#target-path-mode"),
  targetPathValue: document.querySelector("#target-path-value"),
  branchCount: document.querySelector("#branch-count"),
  branchFilter: document.querySelector("#branch-filter"),
  branchList: document.querySelector("#branch-list"),
  actionContext: document.querySelector("#action-context"),
  commandCount: document.querySelector("#command-count"),
  commandFilter: document.querySelector("#command-filter"),
  commandGroups: document.querySelector("#command-groups"),
  draftList: document.querySelector("#draft-list"),
  selectedPath: document.querySelector("#selected-path"),
  commandSummary: document.querySelector("#command-summary"),
  commandUsage: document.querySelector("#command-usage"),
  commandAliases: document.querySelector("#command-aliases"),
  commandFlags: document.querySelector("#command-flags"),
  commandPreview: document.querySelector("#command-preview"),
  argumentsInput: document.querySelector("#arguments-input"),
  stdinInput: document.querySelector("#stdin-input"),
  runButton: document.querySelector("#run-command"),
  runStatus: document.querySelector("#run-status"),
  runID: document.querySelector("#run-id"),
  stdoutOutput: document.querySelector("#stdout-output"),
  stderrOutput: document.querySelector("#stderr-output"),
  runError: document.querySelector("#run-error"),
  actionSwitchDefault: document.querySelector("#action-switch-default"),
  actionSwitchTarget: document.querySelector("#action-switch-target"),
  actionPromoteTarget: document.querySelector("#action-promote-target"),
};

initialize().catch((error) => {
  renderRunError(String(error));
  setStatus("failed");
});

async function initialize() {
  bindEvents();
  setStatus("loading");

  const [repositoriesResponse, commandsResponse] = await Promise.all([
    fetch(repositoriesEndpoint),
    fetch(commandsEndpoint),
  ]);
  if (!repositoriesResponse.ok) {
    throw new Error(`Failed to load repositories: ${repositoriesResponse.status}`);
  }
  if (!commandsResponse.ok) {
    throw new Error(`Failed to load actions: ${commandsResponse.status}`);
  }

  /** @type {RepositoryCatalog} */
  const repositoryCatalog = await repositoriesResponse.json();
  /** @type {CommandCatalog} */
  const commandCatalog = await commandsResponse.json();

  state.repositoryCatalog = repositoryCatalog;
  state.repositories = (repositoryCatalog.repositories || []).slice().sort(compareRepositories);
  state.allCommands = (commandCatalog.commands || []).slice().sort((left, right) => left.path.localeCompare(right.path));
  state.actionableCommands = state.allCommands.filter((command) => command.actionable);

  const initialRepositoryID = repositoryCatalog.selected_repository_id || state.repositories[0]?.id || "";
  if (initialRepositoryID) {
    state.selectedRepositoryID = initialRepositoryID;
    state.checkedRepositoryIDs = [initialRepositoryID];
  }

  elements.repoCount.textContent = String(state.repositories.length);
  elements.commandCount.textContent = String(state.actionableCommands.length);
  elements.targetRefMode.value = state.targetRefMode;
  elements.targetPathMode.value = state.targetPathMode;

  renderRepositoryLaunchSummary();
  renderRepositoryList("");
  renderTargetState();
  renderDraftList();
  renderActionGroups("");

  if (initialRepositoryID) {
    await selectRepository(initialRepositoryID);
  } else {
    renderSelectedRepository();
    renderBranches("");
    syncQuickActions();
  }

  const initialCommand = findCommand(commandPathAuditValue) || findCommand(commandPathVersionValue) || state.actionableCommands[0] || null;
  if (initialCommand) {
    selectCommand(initialCommand.path);
  }

  setStatus("idle");
}

function bindEvents() {
  elements.repoFilter.addEventListener("input", () => {
    renderRepositoryList(elements.repoFilter.value.trim().toLowerCase());
  });

  elements.branchFilter.addEventListener("input", () => {
    renderBranches(elements.branchFilter.value.trim().toLowerCase());
  });

  elements.scopeSelected.addEventListener("click", () => {
    setScope(scopeSelectedValue);
  });

  elements.scopeChecked.addEventListener("click", () => {
    setScope(scopeCheckedValue);
  });

  elements.scopeAll.addEventListener("click", () => {
    setScope(scopeAllValue);
  });

  elements.targetRefMode.addEventListener("change", () => {
    state.targetRefMode = elements.targetRefMode.value;
    if (state.targetRefMode !== refModeNamedValue && state.targetRefMode !== refModePatternValue) {
      state.targetRefValue = "";
    }
    renderTargetState();
    renderBranches(elements.branchFilter.value.trim().toLowerCase());
    syncQuickActions();
    repopulateSelectedCommand();
  });

  elements.targetRefValue.addEventListener("input", () => {
    state.targetRefValue = elements.targetRefValue.value;
    renderTargetState();
    renderBranches(elements.branchFilter.value.trim().toLowerCase());
    syncQuickActions();
    repopulateSelectedCommand();
  });

  elements.targetPathMode.addEventListener("change", () => {
    state.targetPathMode = elements.targetPathMode.value;
    if (state.targetPathMode === pathModeNoneValue) {
      state.targetPathValue = "";
    }
    renderTargetState();
    repopulateSelectedCommand();
  });

  elements.targetPathValue.addEventListener("input", () => {
    state.targetPathValue = elements.targetPathValue.value;
    renderTargetState();
    repopulateSelectedCommand();
  });

  elements.commandFilter.addEventListener("input", () => {
    renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  });

  elements.argumentsInput.addEventListener("input", () => {
    renderCommandPreview();
  });

  elements.runButton.addEventListener("click", () => {
    void submitRun();
  });

  elements.actionSwitchDefault.addEventListener("click", () => {
    loadQuickAction("switch-default");
  });

  elements.actionSwitchTarget.addEventListener("click", () => {
    loadQuickAction("switch-target");
  });

  elements.actionPromoteTarget.addEventListener("click", () => {
    loadQuickAction("promote-target");
  });
}

/**
 * @param {string} mode
 * @param {string} [value]
 */
function setRefTarget(mode, value = "") {
  state.targetRefMode = mode;
  state.targetRefValue = value;
  renderTargetState();
  renderBranches(elements.branchFilter.value.trim().toLowerCase());
  syncQuickActions();
  repopulateSelectedCommand();
}

function renderRepositoryLaunchSummary() {
  const catalog = state.repositoryCatalog;
  if (!catalog) {
    elements.repoLaunchSummary.textContent = "";
    return;
  }

  if (catalog.error) {
    elements.repoLaunchSummary.textContent = catalog.error;
    return;
  }

  const launchMode = catalog.launch_mode === currentRepoLaunchMode ? "Current repo mode" : "Discovery mode";
  const launchPath = catalog.launch_path || "";
  elements.repoLaunchSummary.textContent = launchPath ? `${launchMode} from ${launchPath}` : launchMode;
}

function renderTargetState() {
  const scopeRepositories = repositoryScopeRepositories();
  const scopeLabel = state.selectedScope === scopeSelectedValue
    ? "selected"
    : state.selectedScope === scopeCheckedValue
      ? "checked"
      : "all";

  elements.scopeSelected.classList.toggle("active", state.selectedScope === scopeSelectedValue);
  elements.scopeChecked.classList.toggle("active", state.selectedScope === scopeCheckedValue);
  elements.scopeAll.classList.toggle("active", state.selectedScope === scopeAllValue);
  elements.scopeChecked.disabled = checkedRepositories().length === 0;
  elements.scopeAll.disabled = state.repositories.length === 0;

  elements.targetRepoSummary.textContent = `${scopeRepositories.length} ${scopeRepositories.length === 1 ? "repo" : "repos"}`;
  elements.targetRepoDetail.textContent = buildRepositoryScopeDetail(scopeLabel, scopeRepositories);

  elements.targetRefMode.value = state.targetRefMode;
  elements.targetRefValue.disabled = state.targetRefMode !== refModeNamedValue && state.targetRefMode !== refModePatternValue;
  elements.targetRefValue.placeholder = state.targetRefMode === refModePatternValue ? "branch-*" : "branch-name";
  elements.targetRefValue.value = state.targetRefValue;
  elements.targetRefSummary.textContent = buildRefSummary();

  elements.targetPathMode.value = state.targetPathMode;
  elements.targetPathValue.disabled = state.targetPathMode === pathModeNoneValue;
  elements.targetPathValue.placeholder = pathInputPlaceholder();
  elements.targetPathValue.value = state.targetPathValue;
  elements.targetPathSummary.textContent = buildPathSummary();

  updateActionContext();
}

/**
 * @param {string} query
 */
function renderRepositoryList(query) {
  const filteredRepositories = state.repositories.filter((repository) => {
    if (!query) {
      return true;
    }
    const haystack = [repository.name, repository.path, repository.current_branch || "", repository.default_branch || ""]
      .join(" ")
      .toLowerCase();
    return haystack.includes(query);
  });

  elements.repoList.innerHTML = "";
  if (filteredRepositories.length === 0) {
    appendEmptyState(elements.repoList, state.repositoryCatalog?.error || "No repositories match the current filter.");
    return;
  }

  filteredRepositories.forEach((repository) => {
    const container = document.createElement("div");
    container.className = `repo-entry${repository.id === state.selectedRepositoryID ? " selected" : ""}`;

    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.className = "repo-checkbox";
    checkbox.checked = state.checkedRepositoryIDs.includes(repository.id);
    checkbox.setAttribute("aria-label", `Include ${repository.name} in checked repositories`);
    checkbox.addEventListener("click", (event) => {
      event.stopPropagation();
    });
    checkbox.addEventListener("change", () => {
      toggleCheckedRepository(repository.id, checkbox.checked);
    });

    const button = document.createElement("button");
    button.type = "button";
    button.className = `repo-button${repository.id === state.selectedRepositoryID ? " active" : ""}`;
    const dirtyLabel = repository.dirty ? "dirty" : "clean";
    button.innerHTML = `
      <div class="repo-row">
        <span class="repo-name">${escapeHTML(repository.name)}</span>
        ${repository.context_current ? '<span class="flag-token">context</span>' : ""}
        <span class="flag-token ${repository.dirty ? "flag-token-danger" : "flag-token-success"}">${dirtyLabel}</span>
      </div>
      <div class="repo-meta">${escapeHTML(repository.current_branch || "No current branch")}</div>
      <div class="repo-path-meta">${escapeHTML(repository.path)}</div>
    `;
    button.addEventListener("click", () => {
      void selectRepository(repository.id);
    });

    container.append(checkbox, button);
    elements.repoList.append(container);
  });
}

/**
 * @param {string} repositoryID
 */
async function selectRepository(repositoryID) {
  const repository = findRepository(repositoryID);
  if (!repository) {
    return;
  }

  state.selectedRepositoryID = repositoryID;
  if (state.checkedRepositoryIDs.length === 0) {
    state.checkedRepositoryIDs = [repositoryID];
  }
  state.branches = [];

  renderRepositoryList(elements.repoFilter.value.trim().toLowerCase());
  renderTargetState();
  renderSelectedRepository();
  renderBranches(elements.branchFilter.value.trim().toLowerCase());
  renderDraftList();
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  syncQuickActions();

  const response = await fetch(`${repositoriesEndpoint}/${encodeURIComponent(repositoryID)}/branches`);
  if (!response.ok) {
    state.branches = [];
    renderBranches("");
    syncQuickActions(`Failed to load branches: ${response.status}`);
    return;
  }

  /** @type {BranchCatalog} */
  const branchCatalog = await response.json();
  if (branchCatalog.error) {
    state.branches = [];
    renderBranches("");
    syncQuickActions(branchCatalog.error);
    return;
  }

  state.branches = (branchCatalog.branches || []).slice().sort(compareBranches);
  renderTargetState();
  renderBranches(elements.branchFilter.value.trim().toLowerCase());
  syncQuickActions();
  repopulateSelectedCommand();
}

/**
 * @param {string} repositoryID
 * @param {boolean} checked
 */
function toggleCheckedRepository(repositoryID, checked) {
  const checkedRepositoryIDs = new Set(state.checkedRepositoryIDs);
  if (checked) {
    checkedRepositoryIDs.add(repositoryID);
  } else {
    checkedRepositoryIDs.delete(repositoryID);
  }
  state.checkedRepositoryIDs = Array.from(checkedRepositoryIDs);

  if (state.selectedScope === scopeCheckedValue && state.checkedRepositoryIDs.length === 0) {
    state.selectedScope = scopeSelectedValue;
  }

  renderRepositoryList(elements.repoFilter.value.trim().toLowerCase());
  renderTargetState();
  renderDraftList();
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  syncQuickActions();
  repopulateSelectedCommand();
}

function renderSelectedRepository() {
  const repository = selectedRepository();
  if (!repository) {
    elements.repoTitle.textContent = state.repositoryCatalog?.error ? "Repository context unavailable" : "No repository selected";
    elements.repoPath.textContent = state.repositoryCatalog?.launch_path || "";
    elements.repoSummary.textContent = state.repositoryCatalog?.error || "Select a repository to scope branch and repository actions.";
    elements.repoStateTokens.innerHTML = "";
    return;
  }

  elements.repoTitle.textContent = repository.name;
  elements.repoPath.textContent = repository.path;
  elements.repoSummary.textContent = buildRepositorySummary(repository);
  elements.repoStateTokens.innerHTML = "";

  appendToken(elements.repoStateTokens, repository.current_branch || "No current branch", "token-branch");
  if (repository.default_branch) {
    appendToken(elements.repoStateTokens, `default ${repository.default_branch}`, "token-default");
  }
  appendToken(elements.repoStateTokens, repository.dirty ? "dirty worktree" : "clean worktree", repository.dirty ? "token-danger" : "token-success");
  if (repository.context_current) {
    appendToken(elements.repoStateTokens, "launch context", "token-context");
  }
  if (repository.error) {
    appendToken(elements.repoStateTokens, "inspection warning", "token-warning");
  }
}

function updateActionContext() {
  const scopeRepositories = repositoryScopeRepositories();
  const selectedCommand = findSelectedCommand();
  const commandDraft = selectedCommand ? resolveCommandDraft(selectedCommand) : null;

  if (selectedCommand && commandDraft?.reason) {
    elements.actionContext.textContent = commandDraft.reason;
    return;
  }

  if (selectedCommand && selectedCommand.target.repository === targetRequirementNoneValue) {
    elements.actionContext.textContent = "This action is global and ignores repository, ref, and path targets.";
    return;
  }

  if (scopeRepositories.length === 0) {
    elements.actionContext.textContent = "Choose a repository target set to enable repository-scoped actions and file drafts.";
    return;
  }

  const repositorySummary = `${scopeRepositories.length} ${scopeRepositories.length === 1 ? "repo" : "repos"}`;
  const refSummary = buildRefSummary();
  const pathSummary = buildPathSummary();

  if (selectedCommand && selectedCommand.target.path !== targetRequirementNoneValue) {
    elements.actionContext.textContent = `Targeting ${repositorySummary}. Ref mode ${refSummary}. Path mode ${pathSummary}.`;
    return;
  }

  elements.actionContext.textContent = `Targeting ${repositorySummary}. Ref mode ${refSummary}.`;
}

/**
 * @param {string} query
 */
function renderBranches(query) {
  const repository = selectedRepository();
  const visibleBranches = state.branches.filter((branch) => {
    if (!query) {
      return true;
    }
    const haystack = [branch.name, branch.upstream || ""].join(" ").toLowerCase();
    return haystack.includes(query);
  });

  elements.branchList.innerHTML = "";

  if (!repository) {
  elements.branchCount.textContent = "0";
    appendEmptyState(elements.branchList, "Select a repository to browse current, default, and local branch refs.");
    return;
  }

  const refEntries = [];
  if (repository.current_branch) {
    refEntries.push({
      key: "current",
      label: "Current branch",
      meta: `${repository.current_branch} checked out`,
      current: true,
      isDefault: Boolean(repository.default_branch && repository.default_branch === repository.current_branch),
      active: state.targetRefMode === refModeCurrentValue,
      onSelect: () => setRefTarget(refModeCurrentValue),
    });
  }

  if (repository.default_branch) {
    refEntries.push({
      key: "default",
      label: "Default branch",
      meta: `${repository.default_branch} inferred from origin/HEAD`,
      current: Boolean(repository.current_branch && repository.current_branch === repository.default_branch),
      isDefault: true,
      active: state.targetRefMode === refModeDefaultValue,
      onSelect: () => setRefTarget(refModeDefaultValue),
    });
  }

  visibleBranches.forEach((branch) => {
    refEntries.push({
      key: `branch:${branch.name}`,
      label: branch.name,
      meta: branch.upstream || "No upstream",
      current: branch.current,
      isDefault: Boolean(repository.default_branch && repository.default_branch === branch.name),
      active: state.targetRefMode === refModeNamedValue && state.targetRefValue.trim() === branch.name,
      onSelect: () => setRefTarget(refModeNamedValue, branch.name),
    });
  });

  elements.branchCount.textContent = String(refEntries.length);

  if (refEntries.length === 0) {
    appendEmptyState(elements.branchList, state.branches.length === 0 ? "No local branches were detected for the selected repository." : "No refs match the current filter.");
    return;
  }

  refEntries.forEach((entry) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `branch-button${entry.active ? " active" : ""}`;
    button.innerHTML = `
      <div class="branch-row">
        <span class="branch-name">${escapeHTML(entry.label)}</span>
        ${entry.current ? '<span class="flag-token flag-token-success">current</span>' : ""}
        ${entry.isDefault ? '<span class="flag-token">default</span>' : ""}
        ${entry.active ? '<span class="flag-token">target</span>' : ""}
      </div>
      <div class="branch-meta">${escapeHTML(entry.meta)}</div>
    `;
    button.addEventListener("click", entry.onSelect);
    elements.branchList.append(button);
  });
}

function renderDraftList() {
  const draftCommands = state.allCommands
    .filter((command) => Boolean(command.target.draft_template))
    .sort((left, right) => left.path.localeCompare(right.path));

  elements.draftList.innerHTML = "";
  if (draftCommands.length === 0) {
    appendEmptyState(elements.draftList, "No file drafts are currently available.");
    return;
  }

  const repositoryTargetsAvailable = repositoryScopeRoots().length > 0;
  draftCommands.forEach((command) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `secondary-button draft-button${command.path === state.selectedPath ? " active" : ""}`;
    button.disabled = !repositoryTargetsAvailable || (state.selectedScope !== scopeSelectedValue && !command.target.supports_batch);
    button.textContent = command.short || command.path;
    button.addEventListener("click", () => {
      if (!button.disabled) {
        selectCommand(command.path);
      }
    });
    elements.draftList.append(button);
  });
}

function syncQuickActions(errorText = "") {
  const repository = selectedRepository();
  const branchQuickActionsDisabled = state.selectedScope !== scopeSelectedValue;
  const targetSwitchSelection = resolveBranchChangeSelection();
  const promoteSelection = resolveDefaultTargetBranch();

  elements.actionSwitchDefault.disabled = !repository || branchQuickActionsDisabled;
  elements.actionSwitchTarget.disabled = !repository || branchQuickActionsDisabled || !targetSwitchSelection.ready;
  elements.actionPromoteTarget.disabled = !repository || branchQuickActionsDisabled || !promoteSelection.ready;

  elements.actionSwitchDefault.textContent = repository && repository.default_branch
    ? `Load switch to ${repository.default_branch}`
    : "Load switch to default branch";

  elements.actionSwitchTarget.textContent = targetSwitchSelection.ready && targetSwitchSelection.branch
    ? `Load switch to ${targetSwitchSelection.branch}`
    : "Load switch to target ref";

  elements.actionPromoteTarget.textContent = promoteSelection.ready && promoteSelection.branch
    ? `Load promote ${promoteSelection.branch}`
    : "Load promote target ref";

  if (errorText) {
    elements.actionContext.textContent = errorText;
  }
}

/**
 * @param {"switch-default" | "switch-target" | "promote-target"} quickActionID
 */
function loadQuickAction(quickActionID) {
  const repository = selectedRepository();
  if (!repository) {
    return;
  }

  setScope(scopeSelectedValue);

  if (quickActionID === "switch-default") {
    selectCommand(commandPathBranchChangeValue, { argumentsOverride: ["cd", "--roots", repository.path] });
    return;
  }

  if (quickActionID === "switch-target") {
    const selection = resolveBranchChangeSelection();
    if (!selection.ready) {
      return;
    }
    const argumentsOverride = selection.branch
      ? ["cd", selection.branch, "--roots", repository.path]
      : ["cd", "--roots", repository.path];
    selectCommand(commandPathBranchChangeValue, { argumentsOverride });
    return;
  }

  const selection = resolveDefaultTargetBranch();
  if (!selection.ready) {
    return;
  }
  selectCommand(commandPathDefaultValue, { argumentsOverride: ["default", selection.branch, "--roots", repository.path] });
}

/**
 * @param {string} query
 */
function renderActionGroups(query) {
  const groupedCommands = new Map();
  commandGroupDefinitions.forEach((group) => {
    groupedCommands.set(group.id, []);
  });

  state.actionableCommands.forEach((command) => {
    if (query) {
      const haystack = [command.path, command.short || "", ...(command.aliases || [])].join(" ").toLowerCase();
      if (!haystack.includes(query)) {
        return;
      }
    }

    const groupID = command.target.group || commandGroupGeneralValue;
    const commands = groupedCommands.get(groupID);
    if (commands) {
      commands.push(command);
      return;
    }

    groupedCommands.set(groupID, [command]);
  });

  elements.commandGroups.innerHTML = "";
  let renderedAnyGroup = false;

  commandGroupDefinitions.forEach((group) => {
    const commands = groupedCommands.get(group.id) || [];
    if (commands.length === 0) {
      return;
    }

    renderedAnyGroup = true;
    const section = document.createElement("section");
    section.className = "group-section";
    section.innerHTML = `
      <div class="group-heading">
        <div>
          <h4>${escapeHTML(group.title)}</h4>
          <p class="group-note">${escapeHTML(group.description)}</p>
        </div>
      </div>
    `;

    const list = document.createElement("div");
    list.className = "command-group-list";
    commands.forEach((command) => {
      const button = document.createElement("button");
      const disabled = (
        (command.target.repository !== targetRequirementNoneValue && repositoryScopeRoots().length === 0) ||
        (command.target.repository !== targetRequirementNoneValue && state.selectedScope !== scopeSelectedValue && !command.target.supports_batch)
      );
      button.type = "button";
      button.className = `command-button${command.path === state.selectedPath ? " active" : ""}`;
      button.disabled = disabled;
      button.innerHTML = `
        <span class="command-path">${escapeHTML(command.path)}</span>
        <span class="command-short">${escapeHTML(command.short || command.long || "No description available")}</span>
      `;
      button.addEventListener("click", () => {
        if (!disabled) {
          selectCommand(command.path);
        }
      });
      list.append(button);
    });

    section.append(list);
    elements.commandGroups.append(section);
  });

  if (!renderedAnyGroup) {
    appendEmptyState(elements.commandGroups, "No actions match the current filter.");
  }
}

/**
 * @param {string} commandPath
 * @param {{ argumentsOverride?: string[] }} [options]
 */
function selectCommand(commandPath, options = {}) {
  const command = findCommand(commandPath);
  if (!command) {
    return;
  }

  state.selectedPath = command.path;
  renderDraftList();
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  renderCommandDetails(command);
  populateArguments(command, options.argumentsOverride || null);
  updateActionContext();
}

/**
 * @param {CommandDescriptor} command
 */
function renderCommandDetails(command) {
  elements.selectedPath.textContent = command.path;
  elements.commandSummary.textContent = command.long || command.short || "No description available.";
  elements.commandUsage.textContent = command.use || command.path;
  renderAliases(command.aliases || []);
  renderFlags(command.flags || []);
}

/**
 * @param {CommandDescriptor} command
 * @param {string[] | null} [argumentsOverride]
 */
function populateArguments(command, argumentsOverride = null) {
  const preparedArguments = argumentsOverride || resolveCommandDraft(command).arguments;
  elements.argumentsInput.value = preparedArguments.join("\n");
  renderCommandPreview();
}

/**
 * @param {CommandDescriptor} command
 * @returns {{ arguments: string[], reason: string }}
 */
function resolveCommandDraft(command) {
  const rootArguments = buildRootArgumentsForScope(command);
  if (command.target.repository !== targetRequirementNoneValue && rootArguments.length === 0) {
    return {
      arguments: [],
      reason: "Select at least one repository in the target bar to load this action.",
    };
  }

  if (command.target.repository !== targetRequirementNoneValue && state.selectedScope !== scopeSelectedValue && !command.target.supports_batch) {
    return {
      arguments: [],
      reason: "This action only supports Selected repo scope.",
    };
  }

  if (command.target.draft_template === draftTemplateFilesAddValue || command.target.draft_template === draftTemplateFilesReplaceValue || command.target.draft_template === draftTemplateFilesRemoveValue) {
    return buildDraftArguments(command, rootArguments);
  }

  if (command.path === commandPathBranchChangeValue) {
    return buildBranchChangeArguments(rootArguments);
  }

  if (command.path === commandPathDefaultValue) {
    return buildDefaultCommandArguments(rootArguments);
  }

  return {
    arguments: [...command.path.split(" ").slice(1), ...rootArguments],
    reason: "",
  };
}

/**
 * @param {CommandDescriptor} command
 * @param {string[]} rootArguments
 * @returns {{ arguments: string[], reason: string }}
 */
function buildDraftArguments(command, rootArguments) {
  const optionalRefValue = resolveOptionalGuardRefValue();
  const pathValues = resolvePathValues();

  if (command.target.draft_template === draftTemplateFilesAddValue) {
    return {
      arguments: ["files", "add", ...rootArguments, "--path", firstPathValue(pathValues), "--content", "FILE CONTENT"],
      reason: "",
    };
  }

  if (command.target.draft_template === draftTemplateFilesReplaceValue) {
    const argumentsList = ["files", "replace", ...rootArguments];
    replacementPatterns(pathValues).forEach((patternValue) => {
      argumentsList.push("--pattern", patternValue);
    });
    if (optionalRefValue) {
      argumentsList.push("--branch", optionalRefValue);
    }
    argumentsList.push("--find", "TEXT_TO_FIND", "--replace", "TEXT_TO_REPLACE");
    return { arguments: argumentsList, reason: "" };
  }

  return {
    arguments: ["files", "rm", ...rootArguments, ...removePaths(pathValues)],
    reason: "",
  };
}

/**
 * @param {string[]} rootArguments
 * @returns {{ arguments: string[], reason: string }}
 */
function buildBranchChangeArguments(rootArguments) {
  const selection = resolveBranchChangeSelection();
  if (!selection.ready) {
    return { arguments: [], reason: selection.reason };
  }

  const argumentsList = ["cd"];
  if (selection.branch) {
    argumentsList.push(selection.branch);
  }
  argumentsList.push(...rootArguments);
  return { arguments: argumentsList, reason: "" };
}

/**
 * @param {string[]} rootArguments
 * @returns {{ arguments: string[], reason: string }}
 */
function buildDefaultCommandArguments(rootArguments) {
  const selection = resolveDefaultTargetBranch();
  if (!selection.ready) {
    return { arguments: [], reason: selection.reason };
  }

  return {
    arguments: ["default", selection.branch, ...rootArguments],
    reason: "",
  };
}

/**
 * @returns {{ ready: boolean, branch: string, reason: string }}
 */
function resolveBranchChangeSelection() {
  const repository = selectedRepository();

  if (state.targetRefMode === refModeNamedValue) {
    const namedBranch = state.targetRefValue.trim();
    if (namedBranch) {
      return { ready: true, branch: namedBranch, reason: "" };
    }
    return { ready: false, branch: "", reason: "Enter a named ref to load the switch-branch action." };
  }

  if (state.targetRefMode === refModeDefaultValue) {
    if (state.selectedScope !== scopeSelectedValue) {
      return { ready: true, branch: "", reason: "" };
    }
    return { ready: true, branch: repository?.default_branch || "", reason: "" };
  }

  if (state.targetRefMode === refModeCurrentValue) {
    if (state.selectedScope !== scopeSelectedValue) {
      return {
        ready: false,
        branch: "",
        reason: "Current ref mode cannot be expanded across multiple repositories for switch-branch. Use Selected repo, Named, or Default mode.",
      };
    }
    const currentBranch = currentRepositoryBranchName();
    if (currentBranch) {
      return { ready: true, branch: currentBranch, reason: "" };
    }
    return { ready: false, branch: "", reason: "No current branch is available for the selected repository." };
  }

  if (state.targetRefMode === refModePatternValue) {
    return {
      ready: false,
      branch: "",
      reason: "Switch-branch requires one concrete branch name. Use Named or Default ref mode.",
    };
  }

  return {
    ready: false,
    branch: "",
    reason: "Select a named or default ref to load the switch-branch action.",
  };
}

/**
 * @returns {{ ready: boolean, branch: string, reason: string }}
 */
function resolveDefaultTargetBranch() {
  const repository = selectedRepository();

  if (state.targetRefMode === refModeNamedValue) {
    const namedBranch = state.targetRefValue.trim();
    if (namedBranch) {
      return { ready: true, branch: namedBranch, reason: "" };
    }
    return { ready: false, branch: "", reason: "Enter a named ref to load the promote-default action." };
  }

  if (state.selectedScope !== scopeSelectedValue) {
    return {
      ready: false,
      branch: "",
      reason: "Promoting a default branch across multiple repositories requires Named ref mode.",
    };
  }

  if (state.targetRefMode === refModeCurrentValue) {
    const currentBranch = currentRepositoryBranchName();
    if (currentBranch) {
      return { ready: true, branch: currentBranch, reason: "" };
    }
    return { ready: false, branch: "", reason: "No current branch is available for the selected repository." };
  }

  if (state.targetRefMode === refModeDefaultValue) {
    if (repository?.default_branch) {
      return { ready: true, branch: repository.default_branch, reason: "" };
    }
    return { ready: false, branch: "", reason: "The selected repository does not expose a default branch to promote." };
  }

  if (state.targetRefMode === refModePatternValue) {
    return {
      ready: false,
      branch: "",
      reason: "Promoting a default branch requires one concrete branch name. Use Named or Current mode.",
    };
  }

  return {
    ready: false,
    branch: "",
    reason: "Select a concrete ref to load the promote-default action.",
  };
}

/**
 * @returns {string}
 */
function resolveOptionalGuardRefValue() {
  if (state.targetRefMode === refModeNamedValue) {
    return state.targetRefValue.trim();
  }

  if (state.selectedScope !== scopeSelectedValue) {
    return "";
  }

  if (state.targetRefMode === refModeCurrentValue) {
    return currentRepositoryBranchName();
  }

  if (state.targetRefMode === refModeDefaultValue) {
    return selectedRepository()?.default_branch || "";
  }

  return "";
}

/**
 * @returns {string[]}
 */
function buildRootArgumentsForScope(command) {
  if (command.target.repository === targetRequirementNoneValue) {
    return [];
  }

  const argumentsList = [];
  repositoryScopeRoots().forEach((rootPath) => {
    argumentsList.push("--roots", rootPath);
  });
  return argumentsList;
}

/**
 * @param {string[]} pathValues
 * @returns {string}
 */
function firstPathValue(pathValues) {
  if (pathValues.length > 0) {
    return pathValues[0];
  }

  if (state.targetPathMode === pathModeGlobValue) {
    return pathPlaceholderGlobValue;
  }
  return pathPlaceholderRelativeValue;
}

/**
 * @param {string[]} pathValues
 * @returns {string[]}
 */
function replacementPatterns(pathValues) {
  if (pathValues.length > 0) {
    return pathValues;
  }

  if (state.targetPathMode === pathModeMultipleValue) {
    return pathPlaceholderMultipleValue.split("\n");
  }
  if (state.targetPathMode === pathModeGlobValue) {
    return [pathPlaceholderGlobValue];
  }
  return [pathPlaceholderRelativeValue];
}

/**
 * @param {string[]} pathValues
 * @returns {string[]}
 */
function removePaths(pathValues) {
  if (pathValues.length > 0) {
    return pathValues;
  }

  if (state.targetPathMode === pathModeMultipleValue) {
    return pathPlaceholderMultipleValue.split("\n");
  }
  return [firstPathValue(pathValues)];
}

/**
 * @returns {string[]}
 */
function resolvePathValues() {
  const sanitizedLines = state.targetPathValue
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);

  if (state.targetPathMode === pathModeNoneValue) {
    return [];
  }

  if (state.targetPathMode === pathModeRelativeValue || state.targetPathMode === pathModeGlobValue) {
    if (sanitizedLines.length === 0) {
      return [];
    }
    return [sanitizedLines[0]];
  }

  return sanitizedLines;
}

/**
 * @param {string[]} aliases
 */
function renderAliases(aliases) {
  elements.commandAliases.innerHTML = "";
  if (aliases.length === 0) {
    appendToken(elements.commandAliases, "No aliases", "token-muted");
    return;
  }

  aliases.forEach((alias) => {
    appendToken(elements.commandAliases, alias, "token-muted");
  });
}

/**
 * @param {FlagDescriptor[]} flags
 */
function renderFlags(flags) {
  elements.commandFlags.innerHTML = "";
  if (flags.length === 0) {
    appendEmptyState(elements.commandFlags, "This action does not expose public flags.");
    return;
  }

  flags.forEach((flag) => {
    const container = document.createElement("div");
    container.className = "flag-item";

    const defaultTokens = [];
    if (flag.default) {
      defaultTokens.push(`default ${flag.default}`);
    }
    if (flag.no_opt_default) {
      defaultTokens.push(`implicit ${flag.no_opt_default}`);
    }
    if (flag.type) {
      defaultTokens.push(flag.type);
    }

    container.innerHTML = `
      <div class="flag-name-row">
        <strong>${escapeHTML(`--${flag.name}`)}</strong>
        ${flag.shorthand ? `<span class="flag-token">-${escapeHTML(flag.shorthand)}</span>` : ""}
        ${flag.required ? '<span class="flag-token flag-token-danger">required</span>' : ""}
      </div>
      <div class="flag-description">${escapeHTML(flag.usage || "No usage text available")}</div>
      ${defaultTokens.length > 0 ? `<div class="flag-description">${escapeHTML(defaultTokens.join(" • "))}</div>` : ""}
    `;
    elements.commandFlags.append(container);
  });
}

function renderCommandPreview() {
  const argumentsList = readArguments();
  if (argumentsList.length === 0) {
    elements.commandPreview.textContent = "gix";
    return;
  }

  const quotedArguments = argumentsList.map((argument) => quoteShellArgument(argument));
  elements.commandPreview.textContent = `gix ${quotedArguments.join(" ")}`;
}

async function submitRun() {
  clearPolling();
  renderRunError("");
  setStatus("running");
  elements.runButton.disabled = true;

  const argumentsList = readArguments();
  if (argumentsList.length === 0) {
    renderRunError("Add at least one argument before running an action.");
    setStatus("failed");
    elements.runButton.disabled = false;
    return;
  }

  const response = await fetch(runsEndpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      arguments: argumentsList,
      stdin: elements.stdinInput.value,
    }),
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
    renderRunError(payload.error || `Failed to start run: ${response.status}`);
    setStatus("failed");
    elements.runButton.disabled = false;
    return;
  }

  /** @type {RunSnapshot} */
  const snapshot = await response.json();
  updateRun(snapshot);
  await pollRun(snapshot.id);
}

/**
 * @returns {string[]}
 */
function readArguments() {
  return elements.argumentsInput.value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

/**
 * @param {string} runID
 */
async function pollRun(runID) {
  const tick = async () => {
    const response = await fetch(`${runsEndpoint}/${encodeURIComponent(runID)}`);
    if (!response.ok) {
      renderRunError(`Failed to fetch run ${runID}: ${response.status}`);
      setStatus("failed");
      elements.runButton.disabled = false;
      clearPolling();
      return;
    }

    /** @type {RunSnapshot} */
    const snapshot = await response.json();
    updateRun(snapshot);
    if (snapshot.status === "running") {
      state.pollTimer = window.setTimeout(() => {
        void tick();
      }, pollIntervalMilliseconds);
      return;
    }

    clearPolling();
    elements.runButton.disabled = false;
  };

  await tick();
}

/**
 * @param {RunSnapshot} snapshot
 */
function updateRun(snapshot) {
  elements.runID.textContent = snapshot.id;
  elements.stdoutOutput.textContent = snapshot.stdout || "";
  elements.stderrOutput.textContent = snapshot.stderr || "";
  renderRunError(snapshot.error || "");
  setStatus(snapshot.status);
}

/**
 * @param {string} value
 */
function renderRunError(value) {
  elements.runError.textContent = value;
}

/**
 * @param {string} status
 */
function setStatus(status) {
  elements.runStatus.textContent = status;
  elements.runStatus.className = "status-pill";

  if (status === "running" || status === "loading") {
    elements.runStatus.classList.add("status-running");
    return;
  }
  if (status === "succeeded") {
    elements.runStatus.classList.add("status-succeeded");
    return;
  }
  if (status === "failed") {
    elements.runStatus.classList.add("status-failed");
    return;
  }
  elements.runStatus.classList.add("status-idle");
}

function clearPolling() {
  if (state.pollTimer !== null) {
    window.clearTimeout(state.pollTimer);
    state.pollTimer = null;
  }
}

function repopulateSelectedCommand() {
  const selectedCommand = findSelectedCommand();
  if (!selectedCommand) {
    renderCommandPreview();
    return;
  }

  populateArguments(selectedCommand);
}

/**
 * @param {HTMLElement} container
 * @param {string} text
 */
function appendEmptyState(container, text) {
  const emptyState = document.createElement("div");
  emptyState.className = "empty-state";
  emptyState.textContent = text;
  container.append(emptyState);
}

/**
 * @param {HTMLElement} container
 * @param {string} text
 * @param {string} className
 */
function appendToken(container, text, className) {
  const token = document.createElement("span");
  token.className = `context-token ${className}`;
  token.textContent = text;
  container.append(token);
}

/**
 * @returns {RepositoryDescriptor | null}
 */
function selectedRepository() {
  return findRepository(state.selectedRepositoryID) || null;
}

/**
 * @returns {RepositoryDescriptor[]}
 */
function checkedRepositories() {
  return state.repositories.filter((repository) => state.checkedRepositoryIDs.includes(repository.id));
}

/**
 * @returns {RepositoryDescriptor[]}
 */
function repositoryScopeRepositories() {
  if (state.selectedScope === scopeAllValue) {
    return state.repositories.slice();
  }

  if (state.selectedScope === scopeCheckedValue) {
    return checkedRepositories();
  }

  const repository = selectedRepository();
  return repository ? [repository] : [];
}

/**
 * @returns {string[]}
 */
function repositoryScopeRoots() {
  return repositoryScopeRepositories().map((repository) => repository.path);
}

/**
 * @returns {string}
 */
function currentRepositoryBranchName() {
  return selectedRepository()?.current_branch || "";
}

/**
 * @param {string} repositoryID
 * @returns {RepositoryDescriptor | undefined}
 */
function findRepository(repositoryID) {
  return state.repositories.find((repository) => repository.id === repositoryID);
}

/**
 * @returns {CommandDescriptor | null}
 */
function findSelectedCommand() {
  return findCommand(state.selectedPath) || null;
}

/**
 * @param {string} commandPath
 * @returns {CommandDescriptor | undefined}
 */
function findCommand(commandPath) {
  return state.allCommands.find((command) => command.path === commandPath);
}

/**
 * @param {RepositoryDescriptor} repository
 * @returns {string}
 */
function buildRepositorySummary(repository) {
  const fragments = [];
  if (repository.current_branch) {
    fragments.push(`Current branch ${repository.current_branch}`);
  }
  if (repository.default_branch) {
    fragments.push(`default branch ${repository.default_branch}`);
  }
  fragments.push(repository.dirty ? "worktree has uncommitted changes" : "worktree is clean");
  if (repository.error) {
    fragments.push(`inspection warning: ${repository.error}`);
  }
  return fragments.join(". ");
}

/**
 * @param {string} scopeLabel
 * @param {RepositoryDescriptor[]} repositories
 * @returns {string}
 */
function buildRepositoryScopeDetail(scopeLabel, repositories) {
  if (repositories.length === 0) {
    return `No repositories are active for the ${scopeLabel} scope.`;
  }

  const names = repositories.slice(0, 3).map((repository) => repository.name).join(", ");
  if (repositories.length > 3) {
    return `${scopeLabel} scope includes ${names}, and ${repositories.length - 3} more.`;
  }
  return `${scopeLabel} scope includes ${names}.`;
}

/**
 * @returns {string}
 */
function buildRefSummary() {
  if (state.targetRefMode === refModeNamedValue || state.targetRefMode === refModePatternValue) {
    const value = state.targetRefValue.trim();
    return value ? `${state.targetRefMode}:${value}` : state.targetRefMode;
  }

  if (state.targetRefMode === refModeCurrentValue) {
    const currentBranch = currentRepositoryBranchName();
    if (state.selectedScope === scopeSelectedValue && currentBranch) {
      return `current:${currentBranch}`;
    }
    return state.selectedScope === scopeSelectedValue ? refModeCurrentValue : "current per repo";
  }

  if (state.targetRefMode === refModeDefaultValue) {
    if (state.selectedScope === scopeSelectedValue && selectedRepository()?.default_branch) {
      return `default:${selectedRepository()?.default_branch || ""}`;
    }
    return state.selectedScope === scopeSelectedValue ? refModeDefaultValue : "default per repo";
  }

  return state.targetRefMode;
}

/**
 * @returns {string}
 */
function buildPathSummary() {
  const values = resolvePathValues();
  if (state.targetPathMode === pathModeNoneValue) {
    return pathModeNoneValue;
  }
  if (values.length === 0) {
    return state.targetPathMode;
  }
  return `${state.targetPathMode}:${values.length}`;
}

/**
 * @returns {string}
 */
function pathInputPlaceholder() {
  if (state.targetPathMode === pathModeGlobValue) {
    return pathPlaceholderGlobValue;
  }
  if (state.targetPathMode === pathModeMultipleValue) {
    return pathPlaceholderMultipleValue;
  }
  return pathPlaceholderRelativeValue;
}

/**
 * @param {RepositoryDescriptor} left
 * @param {RepositoryDescriptor} right
 * @returns {number}
 */
function compareRepositories(left, right) {
  if (left.context_current !== right.context_current) {
    return left.context_current ? -1 : 1;
  }
  return left.name.localeCompare(right.name) || left.path.localeCompare(right.path);
}

/**
 * @param {BranchDescriptor} left
 * @param {BranchDescriptor} right
 * @returns {number}
 */
function compareBranches(left, right) {
  if (left.current !== right.current) {
    return left.current ? -1 : 1;
  }
  return left.name.localeCompare(right.name);
}

/**
 * @param {string} scope
 */
function setScope(scope) {
  if (scope !== scopeSelectedValue && scope !== scopeCheckedValue && scope !== scopeAllValue) {
    return;
  }

  if (scope === scopeCheckedValue && checkedRepositories().length === 0) {
    return;
  }

  if (scope === state.selectedScope) {
    return;
  }

  state.selectedScope = scope;
  renderTargetState();
  renderDraftList();
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  syncQuickActions();
  repopulateSelectedCommand();
}

/**
 * @param {string} value
 * @returns {string}
 */
function quoteShellArgument(value) {
  if (/^[A-Za-z0-9_./:@=-]+$/.test(value)) {
    return value;
  }
  return `'${value.replaceAll("'", "'\\''")}'`;
}

/**
 * @param {string} value
 * @returns {string}
 */
function escapeHTML(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
