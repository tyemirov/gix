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
 *   path: string,
 *   use: string,
 *   name: string,
 *   short?: string,
 *   long?: string,
 *   example?: string,
 *   aliases?: string[],
 *   runnable: boolean,
 *   actionable: boolean,
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

/** @type {CommandGroupDefinition[]} */
const commandGroupDefinitions = [
  { id: "branch", title: "Branch Flow", description: "Switch branches and promote branch state inside the selected repository." },
  { id: "repository", title: "Repository", description: "Inspect and normalize repository state." },
  { id: "remote", title: "Remote", description: "Align remotes and transport settings." },
  { id: "prs", title: "Pull Requests", description: "Clean up local and remote PR branches." },
  { id: "packages", title: "Packages", description: "Prune package artifacts tied to the repository." },
  { id: "general", title: "General", description: "Global commands that are not tied to one repository." },
];

/** @type {{
 *   repositoryCatalog: RepositoryCatalog | null,
 *   repositories: RepositoryDescriptor[],
 *   selectedRepositoryID: string,
 *   branches: BranchDescriptor[],
 *   selectedBranchName: string,
 *   commands: CommandDescriptor[],
 *   selectedPath: string,
 *   pollTimer: number | null,
 * }} */
const state = {
  repositoryCatalog: null,
  repositories: [],
  selectedRepositoryID: "",
  branches: [],
  selectedBranchName: "",
  commands: [],
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
  branchCount: document.querySelector("#branch-count"),
  branchFilter: document.querySelector("#branch-filter"),
  branchList: document.querySelector("#branch-list"),
  actionContext: document.querySelector("#action-context"),
  commandCount: document.querySelector("#command-count"),
  commandFilter: document.querySelector("#command-filter"),
  commandGroups: document.querySelector("#command-groups"),
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
  actionSwitchSelected: document.querySelector("#action-switch-selected"),
  actionPromoteSelected: document.querySelector("#action-promote-selected"),
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
  state.commands = (commandCatalog.commands || []).filter((command) => command.actionable).sort((left, right) => left.path.localeCompare(right.path));

  elements.repoCount.textContent = String(state.repositories.length);
  elements.commandCount.textContent = String(state.commands.length);

  renderRepositoryLaunchSummary();
  renderRepositoryList("");
  renderActionGroups("");

  const initialCommand = findCommand("gix audit") || findCommand("gix version") || state.commands[0] || null;
  if (initialCommand) {
    selectCommand(initialCommand.path);
  }

  const initialRepositoryID = repositoryCatalog.selected_repository_id || state.repositories[0]?.id || "";
  if (initialRepositoryID) {
    await selectRepository(initialRepositoryID);
  } else {
    renderSelectedRepository();
    renderBranches("");
    syncQuickActions();
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

  elements.actionSwitchSelected.addEventListener("click", () => {
    loadQuickAction("switch-selected");
  });

  elements.actionPromoteSelected.addEventListener("click", () => {
    loadQuickAction("promote-selected");
  });
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
    elements.repoList.append(button);
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
  state.selectedBranchName = "";
  state.branches = [];

  renderRepositoryList(elements.repoFilter.value.trim().toLowerCase());
  renderSelectedRepository();
  renderBranches(elements.branchFilter.value.trim().toLowerCase());
  syncQuickActions();
  updateActionContext();
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());

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
  const initialBranch = state.branches.find((branch) => branch.current) || state.branches[0] || null;
  state.selectedBranchName = initialBranch ? initialBranch.name : "";
  renderBranches(elements.branchFilter.value.trim().toLowerCase());
  syncQuickActions();

  const selectedCommand = findSelectedCommand();
  if (selectedCommand) {
    populateArguments(selectedCommand);
  }
}

function renderSelectedRepository() {
  const repository = selectedRepository();
  if (!repository) {
    elements.repoTitle.textContent = state.repositoryCatalog?.error ? "Repository context unavailable" : "No repository selected";
    elements.repoPath.textContent = state.repositoryCatalog?.launch_path || "";
    elements.repoSummary.textContent = state.repositoryCatalog?.error || "Select a repository to scope branch and repository actions.";
    elements.repoStateTokens.innerHTML = "";
    updateActionContext();
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

  updateActionContext();
}

function updateActionContext() {
  const repository = selectedRepository();
  if (!repository) {
    elements.actionContext.textContent = "General actions remain available. Repository-scoped actions need a selected repo.";
    return;
  }

  const branch = selectedBranch();
  if (branch) {
    elements.actionContext.textContent = `Scoped to ${repository.name} on branch ${branch.name}. Quick actions load branch-aware commands into the runner.`;
    return;
  }

  elements.actionContext.textContent = `Scoped to ${repository.name}. Repository actions will be prefilled with --roots ${repository.path}.`;
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
  elements.branchCount.textContent = String(state.branches.length);

  if (!repository) {
    appendEmptyState(elements.branchList, "Select a repository to inspect its local branches.");
    return;
  }

  if (visibleBranches.length === 0) {
    appendEmptyState(elements.branchList, state.branches.length === 0 ? "No local branches were detected for the selected repository." : "No branches match the current filter.");
    return;
  }

  visibleBranches.forEach((branch) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `branch-button${branch.name === state.selectedBranchName ? " active" : ""}`;
    const isDefaultBranch = repository.default_branch && repository.default_branch === branch.name;
    button.innerHTML = `
      <div class="branch-row">
        <span class="branch-name">${escapeHTML(branch.name)}</span>
        ${branch.current ? '<span class="flag-token flag-token-success">current</span>' : ""}
        ${isDefaultBranch ? '<span class="flag-token">default</span>' : ""}
      </div>
      <div class="branch-meta">${escapeHTML(branch.upstream || "No upstream")}</div>
    `;
    button.addEventListener("click", () => {
      state.selectedBranchName = branch.name;
      renderBranches(elements.branchFilter.value.trim().toLowerCase());
      syncQuickActions();
      updateActionContext();
    });
    elements.branchList.append(button);
  });
}

function syncQuickActions(errorText = "") {
  const repository = selectedRepository();
  const branch = selectedBranch();

  elements.actionSwitchDefault.disabled = !repository;
  elements.actionSwitchSelected.disabled = !repository || !branch || branch.current;
  elements.actionPromoteSelected.disabled = !repository || !branch;

  elements.actionSwitchDefault.textContent = repository && repository.default_branch
    ? `Load switch to ${repository.default_branch}`
    : "Load switch to default branch";

  elements.actionSwitchSelected.textContent = branch
    ? branch.current
      ? `Already on ${branch.name}`
      : `Load switch to ${branch.name}`
    : "Load switch to selected branch";

  elements.actionPromoteSelected.textContent = branch
    ? `Load promote ${branch.name}`
    : "Load promote selected branch";

  if (errorText) {
    elements.actionContext.textContent = errorText;
  }
}

/**
 * @param {"switch-default" | "switch-selected" | "promote-selected"} quickActionID
 */
function loadQuickAction(quickActionID) {
  const repository = selectedRepository();
  const branch = selectedBranch();
  if (!repository) {
    return;
  }

  if (quickActionID === "switch-default") {
    selectCommand("gix cd", { argumentsOverride: ["cd", "--roots", repository.path] });
    return;
  }

  if (!branch) {
    return;
  }

  if (quickActionID === "switch-selected") {
    selectCommand("gix cd", { argumentsOverride: ["cd", branch.name, "--roots", repository.path] });
    return;
  }

  selectCommand("gix default", { argumentsOverride: ["default", branch.name, "--roots", repository.path] });
}

/**
 * @param {string} query
 */
function renderActionGroups(query) {
  const groupedCommands = new Map();
  commandGroupDefinitions.forEach((group) => {
    groupedCommands.set(group.id, []);
  });

  state.commands.forEach((command) => {
    if (query) {
      const haystack = [command.path, command.short || "", ...(command.aliases || [])].join(" ").toLowerCase();
      if (!haystack.includes(query)) {
        return;
      }
    }
    const group = groupForCommand(command);
    const commands = groupedCommands.get(group.id);
    if (commands) {
      commands.push(command);
    }
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
      const requiresRepository = commandNeedsRepository(command);
      const disabled = requiresRepository && !selectedRepository();
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
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  renderCommandDetails(command);
  populateArguments(command, options.argumentsOverride || null);
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
  const preparedArguments = argumentsOverride || buildCommandArguments(command);
  elements.argumentsInput.value = preparedArguments.join("\n");
  renderCommandPreview();
}

/**
 * @param {CommandDescriptor} command
 * @returns {string[]}
 */
function buildCommandArguments(command) {
  const argumentsList = command.path.split(" ").slice(1);
  const repository = selectedRepository();
  if (repository && commandNeedsRepository(command)) {
    argumentsList.push("--roots", repository.path);
  }
  return argumentsList;
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
 * @returns {BranchDescriptor | null}
 */
function selectedBranch() {
  if (!state.selectedBranchName) {
    return null;
  }
  return state.branches.find((branch) => branch.name === state.selectedBranchName) || null;
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
  return state.commands.find((command) => command.path === commandPath);
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
 * @param {CommandDescriptor} command
 * @returns {CommandGroupDefinition}
 */
function groupForCommand(command) {
  if (command.path === "gix cd" || command.path === "gix default") {
    return commandGroupDefinitions[0];
  }
  if (command.path.startsWith("gix remote ")) {
    return commandGroupDefinitions[2];
  }
  if (command.path.startsWith("gix prs ")) {
    return commandGroupDefinitions[3];
  }
  if (command.path.startsWith("gix packages ")) {
    return commandGroupDefinitions[4];
  }
  if (command.path === "gix version") {
    return commandGroupDefinitions[5];
  }
  return commandGroupDefinitions[1];
}

/**
 * @param {CommandDescriptor} command
 * @returns {boolean}
 */
function commandNeedsRepository(command) {
  return command.path !== "gix version";
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
