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
 *   roots?: string[],
 *   rows?: AuditInspectionRow[],
 *   error?: string,
 * }} AuditInspectionResponse
 */

/**
 * @typedef {{
 *   path: string,
 *   folder_name: string,
 *   is_git_repository: boolean,
 *   final_github_repo: string,
 *   origin_remote_status: string,
 *   name_matches: string,
 *   remote_default_branch: string,
 *   local_branch: string,
 *   in_sync: string,
 *   remote_protocol: string,
 *   origin_matches_canonical: string,
 *   worktree_dirty: string,
 *   dirty_files: string,
 * }} AuditInspectionRow
 */

/**
 * @typedef {{
 *   id: string,
 *   kind: string,
 *   path: string,
 *   include_owner?: boolean,
 *   require_clean?: boolean,
 *   source_protocol?: string,
 *   target_protocol?: string,
 *   sync_strategy?: string,
 *   confirm_delete?: boolean,
 * }} AuditQueuedChange
 */

/**
 * @typedef {AuditQueuedChange & {
 *   title: string,
 *   description: string,
 * }} AuditQueueEntry
 */

/**
 * @typedef {{
 *   results?: AuditChangeApplyResult[],
 *   error?: string,
 * }} AuditChangeApplyResponse
 */

/**
 * @typedef {{
 *   id: string,
 *   kind: string,
 *   path: string,
 *   status: string,
 *   message?: string,
 *   stdout?: string,
 *   stderr?: string,
 *   error?: string,
 * }} AuditChangeApplyResult
 */

/**
 * @typedef {{
 *   kind: string,
 *   label: string,
 *   title: string,
 *   description: string,
 *   buildChange: (row: AuditInspectionRow) => Partial<AuditQueuedChange>,
 * }} AuditRowActionDefinition
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
const auditInspectEndpoint = "/api/audit/inspect";
const auditApplyEndpoint = "/api/audit/apply";
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
const commandPathFilesAddValue = "gix files add";
const commandPathFilesReplaceValue = "gix files replace";
const commandPathFilesRemoveValue = "gix files rm";
const commandPathRemoteCanonicalValue = "gix remote update-to-canonical";
const commandPathPullRequestsDeleteValue = "gix prs delete";
const commandPathPackagesDeleteValue = "gix packages delete";
const commandPathWorkflowValue = "gix workflow";
const auditCommandNameValue = "audit";
const auditChangeKindRenameFolderValue = "rename_folder";
const auditChangeKindUpdateCanonicalValue = "update_remote_canonical";
const auditChangeKindDeleteFolderValue = "delete_folder";
const auditChangeStatusSucceededValue = "succeeded";
const auditHeaderMarkerValue = "folder_name,final_github_repo";
const auditHeaderColumns = [
  "folder_name",
  "final_github_repo",
  "origin_remote_status",
  "name_matches",
  "remote_default_branch",
  "local_branch",
  "in_sync",
  "remote_protocol",
  "origin_matches_canonical",
  "worktree_dirty",
  "dirty_files",
];
const typedAuditHeaderColumns = [
  "path",
  "folder_name",
  "final_github_repo",
  "origin_remote_status",
  "name_matches",
  "remote_default_branch",
  "local_branch",
  "in_sync",
  "remote_protocol",
  "origin_matches_canonical",
  "worktree_dirty",
  "dirty_files",
];
const auditColumnLabels = Object.freeze({
  path: "Path",
  folder_name: "Folder",
  final_github_repo: "GitHub Repo",
  origin_remote_status: "Origin Remote",
  name_matches: "Name Matches",
  remote_default_branch: "Remote Default",
  local_branch: "Local Branch",
  in_sync: "In Sync",
  remote_protocol: "Protocol",
  origin_matches_canonical: "Origin Matches Canonical",
  worktree_dirty: "Worktree Dirty",
  dirty_files: "Dirty Files",
});
const taskInspectValue = "inspect";
const taskBranchValue = "branch";
const taskFilesValue = "files";
const taskRemotesValue = "remotes";
const taskCleanupValue = "cleanup";
const taskWorkflowsValue = "workflows";
const taskAdvancedValue = "advanced";
const fileTaskModeAddValue = "add";
const fileTaskModeReplaceValue = "replace";
const fileTaskModeRemoveValue = "remove";
const pathPlaceholderRelativeValue = "RELATIVE/PATH";
const pathPlaceholderGlobValue = "**/*";
const pathPlaceholderMultipleValue = "PATH/ONE\nPATH/TWO";

/** @type {CommandGroupDefinition[]} */
const commandGroupDefinitions = [
  { id: commandGroupBranchValue, title: "Branch Flow", description: "Switch branches and promote branch state across the target repositories." },
  { id: commandGroupRepositoryValue, title: "Repository", description: "Audit and normalize repository state." },
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
 *   activeTask: string,
 *   targetRefMode: string,
 *   targetRefValue: string,
 *   targetPathMode: string,
 *   targetPathValue: string,
 *   fileTaskMode: string,
 *   branches: BranchDescriptor[],
 *   allCommands: CommandDescriptor[],
 *   actionableCommands: CommandDescriptor[],
 *   advancedCommands: CommandDescriptor[],
 *   selectedPath: string,
 *   auditInspectionRoots: string[],
 *   auditInspectionRows: AuditInspectionRow[],
 *   auditInspectionIncludeAll: boolean,
 *   auditQueue: AuditQueueEntry[],
 *   auditQueueVisible: boolean,
 *   auditQueueApplying: boolean,
 *   nextAuditChangeSequence: number,
 *   pollTimer: number | null,
 * }} */
const state = {
  repositoryCatalog: null,
  repositories: [],
  checkedRepositoryIDs: [],
  selectedRepositoryID: "",
  selectedScope: scopeSelectedValue,
  activeTask: taskInspectValue,
  targetRefMode: refModeCurrentValue,
  targetRefValue: "",
  targetPathMode: pathModeNoneValue,
  targetPathValue: "",
  fileTaskMode: fileTaskModeAddValue,
  branches: [],
  allCommands: [],
  actionableCommands: [],
  advancedCommands: [],
  selectedPath: "",
  auditInspectionRoots: [],
  auditInspectionRows: [],
  auditInspectionIncludeAll: false,
  auditQueue: [],
  auditQueueVisible: false,
  auditQueueApplying: false,
  nextAuditChangeSequence: 1,
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
  targetRefValueBlock: document.querySelector("#target-ref-value-block"),
  targetRefValueLabel: document.querySelector("#target-ref-value-label"),
  targetRefSelect: document.querySelector("#target-ref-select"),
  targetRefValue: document.querySelector("#target-ref-value"),
  targetRefDetail: document.querySelector("#target-ref-detail"),
  targetPathSummary: document.querySelector("#target-path-summary"),
  targetPathMode: document.querySelector("#target-path-mode"),
  targetPathValueBlock: document.querySelector("#target-path-value-block"),
  targetPathValueLabel: document.querySelector("#target-path-value-label"),
  targetPathValue: document.querySelector("#target-path-value"),
  targetPathDetail: document.querySelector("#target-path-detail"),
  actionContext: document.querySelector("#action-context"),
  taskCount: document.querySelector("#task-count"),
  taskInspect: document.querySelector("#task-inspect"),
  taskBranch: document.querySelector("#task-branch"),
  taskFiles: document.querySelector("#task-files"),
  taskRemotes: document.querySelector("#task-remotes"),
  taskCleanup: document.querySelector("#task-cleanup"),
  taskWorkflows: document.querySelector("#task-workflows"),
  taskAdvanced: document.querySelector("#task-advanced"),
  taskPanelInspect: document.querySelector("#task-panel-inspect"),
  taskPanelBranch: document.querySelector("#task-panel-branch"),
  taskPanelFiles: document.querySelector("#task-panel-files"),
  taskPanelRemotes: document.querySelector("#task-panel-remotes"),
  taskPanelCleanup: document.querySelector("#task-panel-cleanup"),
  taskPanelWorkflows: document.querySelector("#task-panel-workflows"),
  taskPanelAdvanced: document.querySelector("#task-panel-advanced"),
  auditRootsInput: document.querySelector("#audit-roots-input"),
  auditIncludeAll: document.querySelector("#audit-include-all"),
  taskInspectLoad: document.querySelector("#task-inspect-load"),
  fileTaskMode: document.querySelector("#file-task-mode"),
  fileTaskAddFields: document.querySelector("#file-task-add-fields"),
  fileTaskReplaceFields: document.querySelector("#file-task-replace-fields"),
  fileContentInput: document.querySelector("#file-content-input"),
  fileFindInput: document.querySelector("#file-find-input"),
  fileReplaceInput: document.querySelector("#file-replace-input"),
  fileTaskLoad: document.querySelector("#task-file-load"),
  fileTaskSummary: document.querySelector("#file-task-summary"),
  remoteOwnerInput: document.querySelector("#remote-owner-input"),
  taskRemoteLoad: document.querySelector("#task-remote-load"),
  taskCleanupPullRequests: document.querySelector("#task-cleanup-prs"),
  taskCleanupPackages: document.querySelector("#task-cleanup-packages"),
  workflowTargetInput: document.querySelector("#workflow-target-input"),
  workflowVarsInput: document.querySelector("#workflow-vars-input"),
  workflowVarFilesInput: document.querySelector("#workflow-var-files-input"),
  workflowWorkersInput: document.querySelector("#workflow-workers-input"),
  workflowRequireClean: document.querySelector("#workflow-require-clean"),
  taskWorkflowLoad: document.querySelector("#task-workflow-load"),
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
  auditResultsPanel: document.querySelector("#audit-results-panel"),
  auditResultsSummary: document.querySelector("#audit-results-summary"),
  auditResultsHead: document.querySelector("#audit-results-head"),
  auditResultsBody: document.querySelector("#audit-results-body"),
  auditQueuePanel: document.querySelector("#audit-queue-panel"),
  auditQueueSummary: document.querySelector("#audit-queue-summary"),
  auditQueueList: document.querySelector("#audit-queue-list"),
  auditQueueClear: document.querySelector("#audit-queue-clear"),
  auditQueueApply: document.querySelector("#audit-queue-apply"),
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
  state.advancedCommands = state.actionableCommands.filter((command) => inferTaskForCommand(command) === taskAdvancedValue);

  const initialRepositoryID = repositoryCatalog.selected_repository_id || state.repositories[0]?.id || "";
  if (initialRepositoryID) {
    state.selectedRepositoryID = initialRepositoryID;
    state.checkedRepositoryIDs = [initialRepositoryID];
  }

  elements.repoCount.textContent = String(state.repositories.length);
  elements.taskCount.textContent = "7";
  elements.commandCount.textContent = String(state.advancedCommands.length);
  elements.targetRefMode.value = state.targetRefMode;
  elements.targetPathMode.value = state.targetPathMode;
  elements.fileTaskMode.value = state.fileTaskMode;

  renderRepositoryLaunchSummary();
  renderRepositoryList("");
  renderTargetState();
  renderTaskState();
  renderActionGroups("");

  if (initialRepositoryID) {
    await selectRepository(initialRepositoryID);
  } else {
    renderSelectedRepository();
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

  elements.taskInspect.addEventListener("click", () => {
    setActiveTask(taskInspectValue);
  });

  elements.taskBranch.addEventListener("click", () => {
    setActiveTask(taskBranchValue);
  });

  elements.taskFiles.addEventListener("click", () => {
    setActiveTask(taskFilesValue);
  });

  elements.taskRemotes.addEventListener("click", () => {
    setActiveTask(taskRemotesValue);
  });

  elements.taskCleanup.addEventListener("click", () => {
    setActiveTask(taskCleanupValue);
  });

  elements.taskWorkflows.addEventListener("click", () => {
    setActiveTask(taskWorkflowsValue);
  });

  elements.taskAdvanced.addEventListener("click", () => {
    setActiveTask(taskAdvancedValue);
  });

  elements.taskInspectLoad.addEventListener("click", () => {
    void runAuditTask();
  });

  const handleAuditDraftChange = () => {
    renderTaskState();
    if (state.selectedPath === commandPathAuditValue) {
      repopulateSelectedCommand();
    }
  };

  elements.auditRootsInput.addEventListener("input", handleAuditDraftChange);
  elements.auditIncludeAll.addEventListener("change", handleAuditDraftChange);
  elements.auditResultsBody.addEventListener("click", handleAuditResultsClick);
  elements.auditQueueList.addEventListener("click", handleAuditQueueListClick);
  elements.auditQueueList.addEventListener("change", handleAuditQueueListChange);
  elements.auditQueueClear.addEventListener("click", clearAuditQueue);
  elements.auditQueueApply.addEventListener("click", () => {
    void applyAuditQueue();
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
    syncQuickActions();
    repopulateSelectedCommand();
  });

  elements.targetRefSelect.addEventListener("change", () => {
    state.targetRefValue = elements.targetRefSelect.value;
    renderTargetState();
    syncQuickActions();
    repopulateSelectedCommand();
  });

  elements.targetRefValue.addEventListener("input", () => {
    state.targetRefValue = elements.targetRefValue.value;
    renderTargetState();
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

  elements.fileTaskMode.addEventListener("change", () => {
    state.fileTaskMode = elements.fileTaskMode.value;
    renderTaskState();
  });

  elements.fileContentInput.addEventListener("input", () => {
    renderTaskState();
    repopulateSelectedCommand();
  });

  elements.fileFindInput.addEventListener("input", () => {
    renderTaskState();
    repopulateSelectedCommand();
  });

  elements.fileReplaceInput.addEventListener("input", () => {
    renderTaskState();
    repopulateSelectedCommand();
  });

  elements.fileTaskLoad.addEventListener("click", () => {
    loadFileTaskCommand();
  });

  elements.remoteOwnerInput.addEventListener("input", () => {
    updateActionContext();
    if (state.selectedPath === commandPathRemoteCanonicalValue) {
      loadRemoteTaskCommand();
    }
  });

  elements.taskRemoteLoad.addEventListener("click", () => {
    loadRemoteTaskCommand();
  });

  elements.taskCleanupPullRequests.addEventListener("click", () => {
    selectCommand(commandPathPullRequestsDeleteValue);
  });

  elements.taskCleanupPackages.addEventListener("click", () => {
    selectCommand(commandPathPackagesDeleteValue);
  });

  const handleWorkflowDraftChange = () => {
    updateActionContext();
    if (state.selectedPath === commandPathWorkflowValue) {
      loadWorkflowTaskCommand();
    }
  };

  elements.workflowTargetInput.addEventListener("input", handleWorkflowDraftChange);
  elements.workflowVarsInput.addEventListener("input", handleWorkflowDraftChange);
  elements.workflowVarFilesInput.addEventListener("input", handleWorkflowDraftChange);
  elements.workflowWorkersInput.addEventListener("input", handleWorkflowDraftChange);
  elements.workflowRequireClean.addEventListener("change", handleWorkflowDraftChange);

  elements.taskWorkflowLoad.addEventListener("click", () => {
    loadWorkflowTaskCommand();
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
 * @param {string} taskID
 */
function setActiveTask(taskID) {
  if (![taskInspectValue, taskBranchValue, taskFilesValue, taskRemotesValue, taskCleanupValue, taskWorkflowsValue, taskAdvancedValue].includes(taskID)) {
    return;
  }

  state.activeTask = taskID;
  if (state.activeTask === taskAdvancedValue) {
    syncAdvancedSelection();
    renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  }
  renderTaskState();
}

function syncAdvancedSelection() {
  const selectedCommand = findSelectedCommand();
  if (selectedCommand && inferTaskForCommand(selectedCommand) === taskAdvancedValue) {
    return;
  }

  const preferredFallbackCommand = findCommand(commandPathVersionValue);
  const fallbackCommand = preferredFallbackCommand && inferTaskForCommand(preferredFallbackCommand) === taskAdvancedValue
    ? preferredFallbackCommand
    : state.advancedCommands[0] || null;

  if (!fallbackCommand) {
    state.selectedPath = "";
    clearCommandDetails();
    elements.argumentsInput.value = "";
    renderCommandPreview();
    return;
  }

  state.selectedPath = fallbackCommand.path;
  renderCommandDetails(fallbackCommand);
  populateArguments(fallbackCommand);
}

function renderTaskState() {
  const repositoryTargetsAvailable = repositoryScopeRoots().length > 0;
  const taskButtons = [
    [elements.taskInspect, taskInspectValue],
    [elements.taskBranch, taskBranchValue],
    [elements.taskFiles, taskFilesValue],
    [elements.taskRemotes, taskRemotesValue],
    [elements.taskCleanup, taskCleanupValue],
    [elements.taskWorkflows, taskWorkflowsValue],
    [elements.taskAdvanced, taskAdvancedValue],
  ];
  taskButtons.forEach(([element, taskID]) => {
    element.classList.toggle("active", state.activeTask === taskID);
  });

  elements.taskPanelInspect.hidden = state.activeTask !== taskInspectValue;
  elements.taskPanelBranch.hidden = state.activeTask !== taskBranchValue;
  elements.taskPanelFiles.hidden = state.activeTask !== taskFilesValue;
  elements.taskPanelRemotes.hidden = state.activeTask !== taskRemotesValue;
  elements.taskPanelCleanup.hidden = state.activeTask !== taskCleanupValue;
  elements.taskPanelWorkflows.hidden = state.activeTask !== taskWorkflowsValue;
  elements.taskPanelAdvanced.hidden = state.activeTask !== taskAdvancedValue;

  renderAuditTaskState();
  elements.fileTaskLoad.disabled = !repositoryTargetsAvailable;
  elements.taskRemoteLoad.disabled = !repositoryTargetsAvailable;
  elements.taskCleanupPullRequests.disabled = !repositoryTargetsAvailable;
  elements.taskCleanupPackages.disabled = !repositoryTargetsAvailable;
  elements.taskWorkflowLoad.disabled = !repositoryTargetsAvailable;

  renderFileTaskState();
  updateActionContext();
}

function renderFileTaskState() {
  elements.fileTaskMode.value = state.fileTaskMode;
  const addMode = state.fileTaskMode === fileTaskModeAddValue;
  const replaceMode = state.fileTaskMode === fileTaskModeReplaceValue;
  const removeMode = state.fileTaskMode === fileTaskModeRemoveValue;

  elements.fileTaskAddFields.hidden = !addMode;
  elements.fileTaskReplaceFields.hidden = !replaceMode;

  const pathSummary = buildPathSummary();
  if (addMode) {
    elements.fileTaskSummary.textContent = `Add file draft. Path target ${pathSummary}.`;
    elements.fileTaskLoad.textContent = "Run add file command";
    return;
  }
  if (replaceMode) {
    elements.fileTaskSummary.textContent = `Replace text draft. Path target ${pathSummary}.`;
    elements.fileTaskLoad.textContent = "Run replace text command";
    return;
  }
  if (removeMode) {
    elements.fileTaskSummary.textContent = `Remove path draft. Path target ${pathSummary}.`;
    elements.fileTaskLoad.textContent = "Run remove paths command";
  }
}

function renderAuditTaskState() {
  const fallbackRoots = repositoryScopeRoots();
  const placeholder = fallbackRoots.length > 0
    ? `${fallbackRoots.join("\n")}\n`
    : "One root per line. Leave blank to use the current repository scope.";
  elements.auditRootsInput.placeholder = placeholder;
  elements.taskInspectLoad.disabled = resolveAuditRoots().length === 0;
}

function loadFileTaskCommand() {
  if (state.fileTaskMode === fileTaskModeAddValue) {
    selectCommand(commandPathFilesAddValue);
    return;
  }
  if (state.fileTaskMode === fileTaskModeReplaceValue) {
    selectCommand(commandPathFilesReplaceValue);
    return;
  }
  selectCommand(commandPathFilesRemoveValue);
}

function loadRemoteTaskCommand() {
  selectCommand(commandPathRemoteCanonicalValue);
}

function loadWorkflowTaskCommand() {
  selectCommand(commandPathWorkflowValue);
}

async function runAuditTask() {
  selectCommand(commandPathAuditValue);
  await inspectAuditRoots(true);
}

/**
 * @param {boolean} clearOutput
 */
async function inspectAuditRoots(clearOutput) {
  const auditRoots = resolveAuditRoots();
  if (auditRoots.length === 0) {
    hideAuditResults();
    clearPolling();
    renderRunError("Enter at least one audit root or choose a repository scope to inspect.");
    setStatus("failed");
    return;
  }

  clearPolling();
  hideAuditResults();
  if (clearOutput) {
    clearRunnerOutput();
  }
  renderRunError("");
  setStatus("loading");
  elements.runID.textContent = "audit inspect";
  elements.taskInspectLoad.disabled = true;

  try {
    const response = await fetch(auditInspectEndpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        roots: auditRoots,
        include_all: Boolean(elements.auditIncludeAll.checked),
      }),
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
      throw new Error(payload.error || `Failed to inspect audit roots: ${response.status}`);
    }

    /** @type {AuditInspectionResponse} */
    const inspection = await response.json();
    if (inspection.error) {
      throw new Error(inspection.error);
    }

    renderTypedAuditResults(inspection);
    setStatus("succeeded");
  } catch (error) {
    hideAuditResults();
    renderRunError(String(error));
    setStatus("failed");
  } finally {
    renderTaskState();
  }
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
  renderRefValueField();
  elements.targetRefSummary.textContent = buildRefSummary();
  elements.targetRefDetail.textContent = buildRefDetail();

  elements.targetPathMode.value = state.targetPathMode;
  renderPathValueField();
  elements.targetPathSummary.textContent = buildPathSummary();
  elements.targetPathDetail.textContent = buildPathDetail();

  renderFileTaskState();
  updateActionContext();
}

function renderRefValueField() {
  const namedMode = state.targetRefMode === refModeNamedValue;
  const patternMode = state.targetRefMode === refModePatternValue;
  const explicitValueMode = namedMode || patternMode;
  const namedOptions = namedRefOptions();

  elements.targetRefValueBlock.hidden = !explicitValueMode;
  elements.targetRefValueLabel.hidden = !explicitValueMode;
  elements.targetRefSelect.hidden = !namedMode;
  elements.targetRefSelect.disabled = !namedMode || namedOptions.length === 0;
  elements.targetRefValue.hidden = namedMode;
  elements.targetRefValue.disabled = !patternMode;
  elements.targetRefValue.placeholder = patternMode ? "branch-*" : "Resolved automatically";

  if (namedMode) {
    if (namedOptions.length > 0 && !namedOptions.some((option) => option.value === state.targetRefValue)) {
      state.targetRefValue = namedOptions[0].value;
    }
    if (namedOptions.length === 0) {
      state.targetRefValue = "";
    }

    elements.targetRefSelect.innerHTML = "";
    if (namedOptions.length === 0) {
      const emptyOption = document.createElement("option");
      emptyOption.value = "";
      emptyOption.textContent = "No named refs available";
      elements.targetRefSelect.append(emptyOption);
      elements.targetRefSelect.value = "";
      return;
    }

    namedOptions.forEach((option) => {
      const optionElement = document.createElement("option");
      optionElement.value = option.value;
      optionElement.textContent = option.label;
      elements.targetRefSelect.append(optionElement);
    });
    elements.targetRefSelect.value = state.targetRefValue;
    return;
  }

  elements.targetRefSelect.innerHTML = "";
  elements.targetRefValue.value = patternMode ? state.targetRefValue : "";
}

function renderPathValueField() {
  const pathInputVisible = state.targetPathMode !== pathModeNoneValue;
  const multilinePathMode = state.targetPathMode === pathModeMultipleValue;

  elements.targetPathValueBlock.hidden = !pathInputVisible;
  elements.targetPathValueLabel.hidden = !pathInputVisible;
  elements.targetPathValue.disabled = !pathInputVisible;
  elements.targetPathValue.placeholder = pathInputPlaceholder();
  elements.targetPathValue.value = state.targetPathValue;
  elements.targetPathValue.classList.toggle("target-path-input-expanded", multilinePathMode);
}

/**
 * @returns {{ value: string, label: string }[]}
 */
function namedRefOptions() {
  const repository = selectedRepository();
  const options = [];
  const seenValues = new Set();

  /**
   * @param {string} value
   * @param {string} label
   */
  const appendOption = (value, label) => {
    const trimmedValue = value.trim();
    if (!trimmedValue || seenValues.has(trimmedValue)) {
      return;
    }
    seenValues.add(trimmedValue);
    options.push({ value: trimmedValue, label });
  };

  if (repository?.current_branch) {
    appendOption(repository.current_branch, `${repository.current_branch} · current`);
  }
  if (repository?.default_branch) {
    appendOption(repository.default_branch, `${repository.default_branch} · default`);
  }
  state.branches.forEach((branch) => {
    const branchSuffix = branch.current
      ? "current"
      : repository?.default_branch === branch.name
        ? "default"
        : branch.upstream || "local";
    appendOption(branch.name, `${branch.name} · ${branchSuffix}`);
  });

  return options;
}

/**
 * @returns {string}
 */
function buildRefDetail() {
  const repository = selectedRepository();

  if (!repository) {
    return "Select a repository to resolve ref targets.";
  }

  if (state.targetRefMode === refModeNamedValue) {
    if (namedRefOptions().length === 0) {
      return "No local named refs are available for the selected repository.";
    }
    return state.selectedScope === scopeSelectedValue
      ? "Named refs come from the selected repository branch list."
      : "Named refs come from the selected repository branch list and apply across the active repository scope.";
  }

  if (state.targetRefMode === refModePatternValue) {
    return "Enter a branch pattern when the command accepts pattern-based ref targeting.";
  }

  if (state.targetRefMode === refModeCurrentValue) {
    return state.selectedScope === scopeSelectedValue
      ? `Current resolves to ${repository.current_branch || "the checked out branch"}.`
      : "Current resolves independently inside each active repository.";
  }

  if (state.targetRefMode === refModeDefaultValue) {
    return state.selectedScope === scopeSelectedValue
      ? `Default resolves to ${repository.default_branch || "the inferred default branch"}.`
      : "Default resolves independently inside each active repository.";
  }

  return "Any leaves ref selection to the command or repository state.";
}

/**
 * @returns {string}
 */
function buildPathDetail() {
  if (state.targetPathMode === pathModeNoneValue) {
    return "No path filter will be applied.";
  }

  if (state.targetPathMode === pathModeRelativeValue) {
    return "Target one relative path.";
  }

  if (state.targetPathMode === pathModeGlobValue) {
    return "Target one glob pattern.";
  }

  return "Target multiple paths or patterns, one per line.";
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
    const branchLabel = repository.current_branch || "No current branch";
    const detailParts = [branchLabel];
    if (repository.path) {
      detailParts.push(repository.path);
    }
    button.innerHTML = `
      <div class="repo-row">
        <span class="repo-name">${escapeHTML(repository.name)}</span>
        <span class="repo-inline-tokens">
          ${repository.context_current ? '<span class="flag-token">context</span>' : ""}
          <span class="flag-token ${repository.dirty ? "flag-token-danger" : "flag-token-success"}">${dirtyLabel}</span>
        </span>
      </div>
      <div class="repo-detail-row" title="${escapeHTML(detailParts.join(" · "))}">
        <span class="repo-branch-meta">${escapeHTML(branchLabel)}</span>
        ${repository.path ? `<span class="repo-path-meta">${escapeHTML(repository.path)}</span>` : ""}
      </div>
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
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  syncQuickActions();

  const response = await fetch(`${repositoriesEndpoint}/${encodeURIComponent(repositoryID)}/branches`);
  if (!response.ok) {
    state.branches = [];
    syncQuickActions(`Failed to load branches: ${response.status}`);
    return;
  }

  /** @type {BranchCatalog} */
  const branchCatalog = await response.json();
  if (branchCatalog.error) {
    state.branches = [];
    syncQuickActions(branchCatalog.error);
    return;
  }

  state.branches = (branchCatalog.branches || []).slice().sort(compareBranches);
  renderTargetState();
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
  const repositorySummary = `${scopeRepositories.length} ${scopeRepositories.length === 1 ? "repo" : "repos"}`;
  const refSummary = buildRefSummary();
  const pathSummary = buildPathSummary();

  switch (state.activeTask) {
    case taskInspectValue:
      if (resolveAuditRoots().length === 0) {
        elements.actionContext.textContent = "Enter one or more audit roots or choose a repository target set to inspect.";
        return;
      }
      elements.actionContext.textContent = elements.auditRootsInput.value.trim().length > 0
        ? `Inspect ${resolveAuditRoots().length} explicit audit ${resolveAuditRoots().length === 1 ? "root" : "roots"}.`
        : `Inspect ${repositorySummary} from the current scope.`;
      return;
    case taskBranchValue:
      if (selectedCommand && selectedCommand.target.group === commandGroupBranchValue && commandDraft?.reason) {
        elements.actionContext.textContent = commandDraft.reason;
        return;
      }
      if (scopeRepositories.length === 0) {
        elements.actionContext.textContent = "Choose a repository target set before running branch commands.";
        return;
      }
      elements.actionContext.textContent = `Branch task targets ${repositorySummary}. Ref mode ${refSummary}.`;
      return;
    case taskFilesValue:
      if (selectedCommand && selectedCommand.target.group === commandGroupFilesValue && commandDraft?.reason) {
        elements.actionContext.textContent = commandDraft.reason;
        return;
      }
      if (scopeRepositories.length === 0) {
        elements.actionContext.textContent = "Choose a repository target set before running file commands.";
        return;
      }
      elements.actionContext.textContent = `Files task targets ${repositorySummary}. Ref mode ${refSummary}. Path mode ${pathSummary}.`;
      return;
    case taskRemotesValue:
      if (scopeRepositories.length === 0) {
        elements.actionContext.textContent = "Choose a repository target set before running remote normalization.";
        return;
      }
      if (selectedCommand && selectedCommand.target.group === commandGroupRemoteValue && commandDraft?.reason) {
        elements.actionContext.textContent = commandDraft.reason;
        return;
      }
      elements.actionContext.textContent = `Remote normalization targets ${repositorySummary}.`;
      return;
    case taskCleanupValue:
      if (scopeRepositories.length === 0) {
        elements.actionContext.textContent = "Choose a repository target set before running cleanup commands.";
        return;
      }
      elements.actionContext.textContent = `Cleanup task targets ${repositorySummary}.`;
      return;
    case taskWorkflowsValue: {
      const workflowTarget = elements.workflowTargetInput?.value.trim() || "WORKFLOW_OR_PRESET";
      if (scopeRepositories.length === 0) {
        elements.actionContext.textContent = "Choose a repository target set before running workflow commands.";
        return;
      }
      elements.actionContext.textContent = `Workflow task targets ${repositorySummary}. Workflow ${workflowTarget}.`;
      return;
    }
    case taskAdvancedValue:
      if (selectedCommand && commandDraft?.reason) {
        elements.actionContext.textContent = commandDraft.reason;
        return;
      }
      if (selectedCommand && selectedCommand.target.repository === targetRequirementNoneValue) {
        elements.actionContext.textContent = "This command is global and ignores repository, ref, and path targets.";
        return;
      }
      if (scopeRepositories.length === 0) {
        elements.actionContext.textContent = "Advanced mode exposes raw commands. Select repository targets when the command is repo-scoped.";
        return;
      }
      if (selectedCommand && selectedCommand.target.path !== targetRequirementNoneValue) {
        elements.actionContext.textContent = `Advanced mode targeting ${repositorySummary}. Ref mode ${refSummary}. Path mode ${pathSummary}.`;
        return;
      }
      elements.actionContext.textContent = `Advanced mode targeting ${repositorySummary}. Ref mode ${refSummary}.`;
      return;
    default:
      elements.actionContext.textContent = "";
  }
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
    ? `Run switch to ${repository.default_branch} command`
    : "Run switch to default branch command";

  elements.actionSwitchTarget.textContent = targetSwitchSelection.ready && targetSwitchSelection.branch
    ? `Run switch to ${targetSwitchSelection.branch} command`
    : "Run switch to target ref command";

  elements.actionPromoteTarget.textContent = promoteSelection.ready && promoteSelection.branch
    ? `Run promote ${promoteSelection.branch} command`
    : "Run promote target ref command";

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

  state.advancedCommands.forEach((command) => {
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
    appendEmptyState(elements.commandGroups, query ? "No advanced actions match the current filter." : "All primary actions are covered by dedicated task views.");
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

  state.activeTask = inferTaskForCommand(command);
  state.selectedPath = command.path;
  renderTaskState();
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

function clearCommandDetails() {
  elements.selectedPath.textContent = "";
  elements.commandSummary.textContent = "Select an advanced command to inspect its metadata.";
  elements.commandUsage.textContent = "";
  renderAliases([]);
  renderFlags([]);
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
  if (command.path === commandPathAuditValue) {
    return buildAuditTaskArguments();
  }

  if (command.path === commandPathWorkflowValue) {
    return buildWorkflowTaskArguments();
  }

  const rootArguments = buildRootArgumentsForScope(command);
  if (command.target.repository !== targetRequirementNoneValue && rootArguments.length === 0) {
    return {
      arguments: [],
      reason: "Select at least one repository in the target bar to run this command.",
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

  if (command.path === commandPathRemoteCanonicalValue) {
    return buildRemoteTaskArguments(rootArguments);
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
 * @returns {{ arguments: string[], reason: string }}
 */
function buildAuditTaskArguments() {
  const auditRoots = resolveAuditRoots();
  if (auditRoots.length === 0) {
    return {
      arguments: [],
      reason: "Enter at least one audit root or choose a repository target set to inspect.",
    };
  }

  const argumentsList = ["audit"];
  if (elements.auditIncludeAll.checked) {
    argumentsList.push("--all");
  }
  auditRoots.forEach((rootPath) => {
    argumentsList.push("--roots", rootPath);
  });

  return { arguments: argumentsList, reason: "" };
}

/**
 * @param {string[]} rootArguments
 * @returns {{ arguments: string[], reason: string }}
 */
function buildRemoteTaskArguments(rootArguments) {
  if (rootArguments.length === 0) {
    return {
      arguments: [],
      reason: "Select at least one repository in the target bar to run remote normalization.",
    };
  }

  const ownerConstraint = elements.remoteOwnerInput?.value.trim() || "";
  const argumentsList = ["remote", "update-to-canonical"];
  if (ownerConstraint) {
    argumentsList.push("--owner", ownerConstraint);
  }
  argumentsList.push(...rootArguments);
  return { arguments: argumentsList, reason: "" };
}

/**
 * @returns {{ arguments: string[], reason: string }}
 */
function buildWorkflowTaskArguments() {
  const repositoryRoots = repositoryScopeRoots();
  if (repositoryRoots.length === 0) {
    return {
      arguments: [],
      reason: "Select at least one repository in the target bar to run a workflow command.",
    };
  }

  const workflowTarget = elements.workflowTargetInput?.value.trim() || "WORKFLOW_OR_PRESET";
  const variableAssignments = splitNonEmptyLines(elements.workflowVarsInput?.value || "");
  const variableFiles = splitNonEmptyLines(elements.workflowVarFilesInput?.value || "");
  const workflowWorkers = Math.max(1, Number.parseInt(elements.workflowWorkersInput?.value || "1", 10) || 1);
  const requireClean = Boolean(elements.workflowRequireClean?.checked);

  const argumentsList = ["workflow", workflowTarget];
  if (requireClean) {
    argumentsList.push("--require-clean");
  }
  variableAssignments.forEach((assignment) => {
    argumentsList.push("--var", assignment);
  });
  variableFiles.forEach((filePath) => {
    argumentsList.push("--var-file", filePath);
  });
  if (workflowWorkers !== 1) {
    argumentsList.push("--workflow-workers", String(workflowWorkers));
  }
  repositoryRoots.forEach((rootPath) => {
    argumentsList.push("--roots", rootPath);
  });

  return { arguments: argumentsList, reason: "" };
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
    const fileContent = readTextValue(elements.fileContentInput, "FILE CONTENT");
    return {
      arguments: ["files", "add", ...rootArguments, "--path", firstPathValue(pathValues), "--content", fileContent],
      reason: "",
    };
  }

  if (command.target.draft_template === draftTemplateFilesReplaceValue) {
    const findValue = readTextValue(elements.fileFindInput, "TEXT_TO_FIND");
    const replaceValue = readTextValue(elements.fileReplaceInput, "TEXT_TO_REPLACE");
    const argumentsList = ["files", "replace", ...rootArguments];
    replacementPatterns(pathValues).forEach((patternValue) => {
      argumentsList.push("--pattern", patternValue);
    });
    if (optionalRefValue) {
      argumentsList.push("--branch", optionalRefValue);
    }
    argumentsList.push("--find", findValue, "--replace", replaceValue);
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
    return { ready: false, branch: "", reason: "Enter a named ref to run the switch-branch command." };
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
    reason: "Select a named or default ref to run the switch-branch command.",
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
    return { ready: false, branch: "", reason: "Enter a named ref to run the promote-default command." };
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
    reason: "Select a concrete ref to run the promote-default command.",
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
 * @param {HTMLTextAreaElement | null} element
 * @param {string} fallback
 * @returns {string}
 */
function readTextValue(element, fallback) {
  const value = element?.value.trim() || "";
  return value || fallback;
}

/**
 * @param {string} value
 * @returns {string[]}
 */
function splitNonEmptyLines(value) {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

/**
 * @returns {string[]}
 */
function resolveAuditRoots() {
  const explicitRoots = splitNonEmptyLines(elements.auditRootsInput?.value || "");
  const rootValues = explicitRoots.length > 0 ? explicitRoots : repositoryScopeRoots();
  return Array.from(new Set(rootValues));
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
  if (elements.runButton.disabled) {
    return;
  }

  clearPolling();
  renderRunError("");
  hideAuditResults();
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
  renderRunAuditResults(snapshot);
  renderRunError(snapshot.error || "");
  setStatus(snapshot.status);
}

function clearRunnerOutput() {
  elements.stdoutOutput.textContent = "";
  elements.stderrOutput.textContent = "";
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
 * @param {RunSnapshot | null} snapshot
 */
function renderRunAuditResults(snapshot) {
  const auditResults = resolveAuditResults(snapshot);
  if (!auditResults) {
    hideAuditResults();
    return;
  }

  renderAuditTable(auditResults.headers, auditResults.rows, "No audit rows returned.");
}

/**
 * @param {AuditInspectionResponse} inspection
 */
function renderTypedAuditResults(inspection) {
  state.auditInspectionRoots = (inspection.roots || []).slice();
  state.auditInspectionRows = (inspection.rows || []).slice();
  state.auditInspectionIncludeAll = Boolean(elements.auditIncludeAll.checked);
  state.auditQueueVisible = true;

  renderTypedAuditTable(state.auditInspectionRows);
  renderAuditQueue();
}

/**
 * @param {string[]} headers
 * @param {string[][]} rows
 * @param {string} emptyMessage
 */
function renderAuditTable(headers, rows, emptyMessage) {
  elements.auditResultsHead.innerHTML = "";
  elements.auditResultsBody.innerHTML = "";

  const headerRow = document.createElement("tr");
  headers.forEach((headerName) => {
    const headerCell = document.createElement("th");
    headerCell.scope = "col";
    headerCell.textContent = auditColumnLabels[headerName] || headerName;
    headerRow.append(headerCell);
  });
  elements.auditResultsHead.append(headerRow);

  if (rows.length === 0) {
    const emptyRow = document.createElement("tr");
    const emptyCell = document.createElement("td");
    emptyCell.colSpan = headers.length;
    emptyCell.textContent = emptyMessage;
    emptyRow.append(emptyCell);
    elements.auditResultsBody.append(emptyRow);
  } else {
    rows.forEach((record) => {
      const rowElement = document.createElement("tr");
      record.forEach((value, valueIndex) => {
        const cell = document.createElement(valueIndex === 0 ? "th" : "td");
        if (valueIndex === 0) {
          cell.scope = "row";
        }
        cell.textContent = value || " ";
        rowElement.append(cell);
      });
      elements.auditResultsBody.append(rowElement);
    });
  }

  const rowCount = rows.length;
  elements.auditResultsSummary.textContent = `${rowCount} ${rowCount === 1 ? "row" : "rows"}`;
  elements.auditResultsPanel.hidden = false;
}

/**
 * @param {AuditInspectionRow[]} rows
 */
function renderTypedAuditTable(rows) {
  elements.auditResultsHead.innerHTML = "";
  elements.auditResultsBody.innerHTML = "";

  const headerRow = document.createElement("tr");
  typedAuditHeaderColumns.forEach((headerName) => {
    const headerCell = document.createElement("th");
    headerCell.scope = "col";
    headerCell.textContent = auditColumnLabels[headerName] || headerName;
    headerRow.append(headerCell);
  });

  const actionsHeaderCell = document.createElement("th");
  actionsHeaderCell.scope = "col";
  actionsHeaderCell.textContent = "Actions";
  headerRow.append(actionsHeaderCell);
  elements.auditResultsHead.append(headerRow);

  if (rows.length === 0) {
    const emptyRow = document.createElement("tr");
    const emptyCell = document.createElement("td");
    emptyCell.colSpan = typedAuditHeaderColumns.length + 1;
    emptyCell.textContent = "No repositories or folders matched the inspected roots.";
    emptyRow.append(emptyCell);
    elements.auditResultsBody.append(emptyRow);
  } else {
    rows.forEach((row) => {
      const rowElement = document.createElement("tr");
      const record = typedAuditRecord(row);

      record.forEach((value, valueIndex) => {
        const cell = document.createElement(valueIndex === 0 ? "th" : "td");
        if (valueIndex === 0) {
          cell.scope = "row";
        }
        cell.textContent = value || " ";
        rowElement.append(cell);
      });

      const actionsCell = document.createElement("td");
      actionsCell.className = "audit-actions-cell";

      const actions = buildAuditRowActions(row);
      if (actions.length === 0) {
        const mutedLabel = document.createElement("span");
        mutedLabel.className = "panel-note";
        mutedLabel.textContent = "No queued fixes";
        actionsCell.append(mutedLabel);
      } else {
        const actionsList = document.createElement("div");
        actionsList.className = "audit-actions-list";
        actions.forEach((action) => {
          const button = document.createElement("button");
          button.type = "button";
          button.className = "secondary-button audit-action-button";
          button.dataset.auditAction = action.kind;
          button.dataset.auditPath = row.path;
          button.textContent = action.label;
          actionsList.append(button);
        });
        actionsCell.append(actionsList);
      }

      rowElement.append(actionsCell);
      elements.auditResultsBody.append(rowElement);
    });
  }

  const rowCount = rows.length;
  elements.auditResultsSummary.textContent = `${rowCount} ${rowCount === 1 ? "row" : "rows"}`;
  elements.auditResultsPanel.hidden = false;
}

function hideAuditResults() {
  elements.auditResultsPanel.hidden = true;
  elements.auditResultsSummary.textContent = "";
  elements.auditResultsHead.innerHTML = "";
  elements.auditResultsBody.innerHTML = "";
}

/**
 * @param {AuditInspectionRow} row
 * @returns {string[]}
 */
function typedAuditRecord(row) {
  return typedAuditHeaderColumns.map((headerName) => {
    switch (headerName) {
      case "path":
        return row.path;
      case "folder_name":
        return row.folder_name;
      case "final_github_repo":
        return row.final_github_repo;
      case "origin_remote_status":
        return row.origin_remote_status;
      case "name_matches":
        return row.name_matches;
      case "remote_default_branch":
        return row.remote_default_branch;
      case "local_branch":
        return row.local_branch;
      case "in_sync":
        return row.in_sync;
      case "remote_protocol":
        return row.remote_protocol;
      case "origin_matches_canonical":
        return row.origin_matches_canonical;
      case "worktree_dirty":
        return row.worktree_dirty;
      case "dirty_files":
        return row.dirty_files;
      default:
        return "";
    }
  });
}

/**
 * @param {MouseEvent} event
 */
function handleAuditResultsClick(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLElement)) {
    return;
  }

  const actionButton = eventTarget.closest("[data-audit-action]");
  if (!(actionButton instanceof HTMLButtonElement)) {
    return;
  }

  const actionKind = actionButton.dataset.auditAction || "";
  const actionPath = actionButton.dataset.auditPath || "";
  if (!actionKind || !actionPath) {
    return;
  }

  const row = state.auditInspectionRows.find((candidate) => candidate.path === actionPath);
  if (!row) {
    renderRunError(`Audit row ${actionPath} is no longer available.`);
    return;
  }

  const action = buildAuditRowActions(row).find((candidate) => candidate.kind === actionKind);
  if (!action) {
    renderRunError(`Audit action ${actionKind} is not available for ${actionPath}.`);
    return;
  }

  queueAuditChange(row, action);
}

/**
 * @param {MouseEvent} event
 */
function handleAuditQueueListClick(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLElement)) {
    return;
  }

  const removeButton = eventTarget.closest("[data-queue-remove-id]");
  if (!(removeButton instanceof HTMLButtonElement)) {
    return;
  }

  const changeID = removeButton.dataset.queueRemoveId || "";
  if (!changeID) {
    return;
  }

  state.auditQueue = state.auditQueue.filter((change) => change.id !== changeID);
  renderRunError("");
  renderAuditQueue();
}

/**
 * @param {Event} event
 */
function handleAuditQueueListChange(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLElement)) {
    return;
  }

  const deleteCheckbox = eventTarget.closest("[data-queue-confirm-delete]");
  if (!(deleteCheckbox instanceof HTMLInputElement)) {
    return;
  }

  const changeID = deleteCheckbox.dataset.queueConfirmDelete || "";
  if (!changeID) {
    return;
  }

  state.auditQueue = state.auditQueue.map((change) => {
    if (change.id !== changeID) {
      return change;
    }
    return {
      ...change,
      confirm_delete: deleteCheckbox.checked,
    };
  });
  renderRunError("");
  renderAuditQueue();
}

/**
 * @param {AuditInspectionRow} row
 * @returns {AuditRowActionDefinition[]}
 */
function buildAuditRowActions(row) {
  /** @type {AuditRowActionDefinition[]} */
  const actions = [];
  if (row.is_git_repository) {
    if (row.name_matches === "no") {
      actions.push({
        kind: auditChangeKindRenameFolderValue,
        label: "Queue rename",
        title: "Rename folder",
        description: row.final_github_repo && row.final_github_repo !== "n/a"
          ? `Rename the folder to match ${row.final_github_repo}.`
          : "Rename the folder to match the audited repository name.",
        buildChange: () => ({
          kind: auditChangeKindRenameFolderValue,
          path: row.path,
          require_clean: true,
        }),
      });
    }

    if (row.origin_remote_status === "configured" && row.origin_matches_canonical === "no") {
      actions.push({
        kind: auditChangeKindUpdateCanonicalValue,
        label: "Queue remote fix",
        title: "Fix canonical remote",
        description: row.final_github_repo && row.final_github_repo !== "n/a"
          ? `Update origin to the canonical repository ${row.final_github_repo}.`
          : "Update origin to the canonical repository.",
        buildChange: () => ({
          kind: auditChangeKindUpdateCanonicalValue,
          path: row.path,
        }),
      });
    }
  }

  if (row.path) {
    actions.push({
      kind: auditChangeKindDeleteFolderValue,
      label: "Queue delete",
      title: "Delete folder",
      description: `Delete ${row.path} from disk from the web audit workspace after explicit confirmation.`,
      buildChange: () => ({
        kind: auditChangeKindDeleteFolderValue,
        path: row.path,
        confirm_delete: false,
      }),
    });
  }

  return actions;
}

/**
 * @param {AuditInspectionRow} row
 * @param {AuditRowActionDefinition} action
 */
function queueAuditChange(row, action) {
  const nextChangeID = `audit-change-${state.nextAuditChangeSequence}`;
  const changeDefinition = action.buildChange(row);
  /** @type {AuditQueueEntry} */
  const candidate = {
    id: nextChangeID,
    kind: action.kind,
    path: row.path,
    title: action.title,
    description: action.description,
    ...changeDefinition,
  };

  const existingDeleteChange = state.auditQueue.find((change) => change.path === candidate.path && change.kind === auditChangeKindDeleteFolderValue);
  if (existingDeleteChange && candidate.kind !== auditChangeKindDeleteFolderValue) {
    renderRunError(`Delete folder is already queued for ${candidate.path}. Remove it before adding another fix.`);
    return;
  }

  const existingIndex = state.auditQueue.findIndex((change) => change.path === candidate.path && change.kind === candidate.kind);
  if (candidate.kind === auditChangeKindDeleteFolderValue && state.auditQueue.some((change) => change.path === candidate.path && change.kind !== candidate.kind)) {
    renderRunError(`Remove existing queued fixes for ${candidate.path} before queueing folder deletion.`);
    return;
  }

  if (existingIndex >= 0) {
    const existingChange = state.auditQueue[existingIndex];
    state.auditQueue[existingIndex] = {
      ...existingChange,
      ...candidate,
      id: existingChange.id,
    };
  } else {
    state.nextAuditChangeSequence += 1;
    state.auditQueue = state.auditQueue.concat(candidate);
  }

  state.auditQueueVisible = true;
  renderRunError("");
  renderAuditQueue();
}

function clearAuditQueue() {
  state.auditQueue = [];
  renderRunError("");
  renderAuditQueue();
}

function renderAuditQueue() {
  const shouldShowQueue = state.auditQueueVisible || state.auditQueue.length > 0;
  if (!shouldShowQueue) {
    elements.auditQueuePanel.hidden = true;
    elements.auditQueueSummary.textContent = "";
    elements.auditQueueList.innerHTML = "";
    return;
  }

  elements.auditQueuePanel.hidden = false;
  elements.auditQueueSummary.textContent = auditQueueSummary(state.auditQueue.length);
  elements.auditQueueList.innerHTML = "";

  if (state.auditQueue.length === 0) {
    appendEmptyState(elements.auditQueueList, "No pending changes queued from the current audit table.");
  } else {
    state.auditQueue.forEach((change) => {
      const container = document.createElement("article");
      container.className = "audit-queue-item";

      const heading = document.createElement("div");
      heading.className = "audit-queue-item-heading";

      const title = document.createElement("strong");
      title.textContent = change.title;
      heading.append(title);

      const removeButton = document.createElement("button");
      removeButton.type = "button";
      removeButton.className = "secondary-button audit-queue-remove";
      removeButton.dataset.queueRemoveId = change.id;
      removeButton.textContent = "Remove";
      heading.append(removeButton);

      const description = document.createElement("p");
      description.className = "audit-queue-description";
      description.textContent = change.description;

      const meta = document.createElement("div");
      meta.className = "audit-queue-meta";
      appendToken(meta, change.kind, "token-default");
      appendToken(meta, change.path, "token-context");

      container.append(heading, description, meta);
      const options = renderAuditQueueOptions(change);
      if (options) {
        container.append(options);
      }
      elements.auditQueueList.append(container);
    });
  }

  elements.auditQueueClear.disabled = state.auditQueue.length === 0 || state.auditQueueApplying;
  elements.auditQueueApply.disabled = state.auditQueue.length === 0 || state.auditQueueApplying || !auditQueueCanApply();
}

async function applyAuditQueue() {
  if (state.auditQueue.length === 0 || state.auditQueueApplying) {
    return;
  }
  if (!auditQueueCanApply()) {
    renderRunError("Review the pending changes and complete all required confirmations before applying the queue.");
    return;
  }

  state.auditQueueApplying = true;
  renderAuditQueue();
  clearRunnerOutput();
  clearPolling();
  renderRunError("");
  setStatus("loading");
  elements.runID.textContent = "audit apply";

  try {
    const response = await fetch(auditApplyEndpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        changes: state.auditQueue.map((change) => ({
          id: change.id,
          kind: change.kind,
          path: change.path,
          include_owner: Boolean(change.include_owner),
          require_clean: Boolean(change.require_clean),
          source_protocol: change.source_protocol || "",
          target_protocol: change.target_protocol || "",
          sync_strategy: change.sync_strategy || "",
          confirm_delete: Boolean(change.confirm_delete),
        })),
      }),
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
      throw new Error(payload.error || `Failed to apply queued audit changes: ${response.status}`);
    }

    /** @type {AuditChangeApplyResponse} */
    const applyResponse = await response.json();
    if (applyResponse.error) {
      throw new Error(applyResponse.error);
    }

    const results = applyResponse.results || [];
    renderAuditApplyResults(results);

    const succeededIDs = new Set(
      results
        .filter((result) => result.status === auditChangeStatusSucceededValue)
        .map((result) => result.id),
    );
    state.auditQueue = state.auditQueue.filter((change) => !succeededIDs.has(change.id));

    if (state.auditInspectionRoots.length > 0) {
      await inspectAuditRoots(false);
    }

    const failedResults = results.filter((result) => result.status !== auditChangeStatusSucceededValue);
    if (failedResults.length > 0) {
      renderRunError(failedResults.map((result) => result.error || `${result.kind} failed for ${result.path}`).join("\n"));
      setStatus("failed");
    } else {
      setStatus("succeeded");
    }
  } catch (error) {
    renderRunError(String(error));
    setStatus("failed");
  } finally {
    state.auditQueueApplying = false;
    renderAuditQueue();
  }
}

/**
 * @param {AuditChangeApplyResult[]} results
 */
function renderAuditApplyResults(results) {
  const stdoutSections = [];
  const stderrSections = [];

  results.forEach((result) => {
    const headingParts = [formatAuditChangeKind(result.kind)];
    if (result.path) {
      headingParts.push(result.path);
    }
    const heading = headingParts.join(" · ");

    const stdoutLines = [];
    if (result.message) {
      stdoutLines.push(result.message);
    }
    if (result.stdout) {
      stdoutLines.push(result.stdout.trim());
    }
    if (stdoutLines.length > 0) {
      stdoutSections.push(`${heading}\n${stdoutLines.filter(Boolean).join("\n")}`.trim());
    }

    const stderrLines = [];
    if (result.error) {
      stderrLines.push(result.error);
    }
    if (result.stderr) {
      stderrLines.push(result.stderr.trim());
    }
    if (stderrLines.length > 0) {
      stderrSections.push(`${heading}\n${stderrLines.filter(Boolean).join("\n")}`.trim());
    }
  });

  elements.stdoutOutput.textContent = stdoutSections.join("\n\n");
  elements.stderrOutput.textContent = stderrSections.join("\n\n");
}

/**
 * @param {number} count
 * @returns {string}
 */
function auditQueueSummary(count) {
  return `${count} pending ${count === 1 ? "change" : "changes"}`;
}

/**
 * @param {string} kind
 * @returns {string}
 */
function formatAuditChangeKind(kind) {
  switch (kind) {
    case auditChangeKindRenameFolderValue:
      return "Rename folder";
    case auditChangeKindUpdateCanonicalValue:
      return "Fix canonical remote";
    case auditChangeKindDeleteFolderValue:
      return "Delete folder";
    default:
      return kind;
  }
}

/**
 * @param {AuditQueueEntry} change
 * @returns {HTMLElement | null}
 */
function renderAuditQueueOptions(change) {
  if (change.kind !== auditChangeKindDeleteFolderValue) {
    return null;
  }

  const container = document.createElement("div");
  container.className = "audit-queue-options";

  const warning = document.createElement("p");
  warning.className = "audit-queue-warning";
  warning.textContent = "This permanently removes the folder from disk. Confirm before applying.";

  const label = document.createElement("label");
  label.className = "checkbox-row audit-queue-confirm";

  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.checked = Boolean(change.confirm_delete);
  checkbox.dataset.queueConfirmDelete = change.id;

  const copy = document.createElement("span");
  copy.textContent = "I understand this deletes the folder permanently";

  label.append(checkbox, copy);
  container.append(warning, label);
  return container;
}

/**
 * @returns {boolean}
 */
function auditQueueCanApply() {
  return state.auditQueue.every((change) => {
    if (change.kind === auditChangeKindDeleteFolderValue) {
      return Boolean(change.confirm_delete);
    }
    return true;
  });
}

/**
 * @param {RunSnapshot | null} snapshot
 * @returns {{ headers: string[], rows: string[][] } | null}
 */
function resolveAuditResults(snapshot) {
  if (!snapshot || !isAuditRun(snapshot)) {
    return null;
  }

  const parsedRecords = parseAuditCSV(snapshot.stdout || "");
  if (!parsedRecords || parsedRecords.length === 0) {
    return null;
  }

  const [headers, ...records] = parsedRecords;
  if (!auditHeadersMatch(headers)) {
    return null;
  }

  const rows = [];
  for (const record of records) {
    if (record.length === 1 && record[0].trim().length === 0) {
      continue;
    }
    if (record.length !== headers.length) {
      return null;
    }
    rows.push(record);
  }

  return { headers, rows };
}

/**
 * @param {RunSnapshot} snapshot
 * @returns {boolean}
 */
function isAuditRun(snapshot) {
  return Array.isArray(snapshot.arguments) && snapshot.arguments[0] === auditCommandNameValue;
}

/**
 * @param {string} rawOutput
 * @returns {string[][] | null}
 */
function parseAuditCSV(rawOutput) {
  const headerIndex = rawOutput.indexOf(auditHeaderMarkerValue);
  if (headerIndex === -1) {
    return null;
  }

  const csvPayload = rawOutput.slice(headerIndex).trim();
  if (!csvPayload) {
    return null;
  }

  return parseCSVRecords(csvPayload);
}

/**
 * @param {string[]} headers
 * @returns {boolean}
 */
function auditHeadersMatch(headers) {
  if (headers.length !== auditHeaderColumns.length) {
    return false;
  }

  return headers.every((headerValue, headerIndex) => headerValue === auditHeaderColumns[headerIndex]);
}

/**
 * @param {string} rawCSV
 * @returns {string[][] | null}
 */
function parseCSVRecords(rawCSV) {
  if (!rawCSV) {
    return null;
  }

  /** @type {string[][]} */
  const records = [];
  /** @type {string[]} */
  let currentRecord = [];
  let currentField = "";
  let insideQuotes = false;

  const commitField = () => {
    currentRecord.push(currentField);
    currentField = "";
  };

  const commitRecord = () => {
    commitField();
    records.push(currentRecord);
    currentRecord = [];
  };

  for (let characterIndex = 0; characterIndex < rawCSV.length; characterIndex += 1) {
    const character = rawCSV[characterIndex];

    if (insideQuotes) {
      if (character === "\"") {
        const nextCharacter = rawCSV[characterIndex + 1];
        if (nextCharacter === "\"") {
          currentField += "\"";
          characterIndex += 1;
          continue;
        }
        insideQuotes = false;
        continue;
      }

      currentField += character;
      continue;
    }

    if (character === "\"") {
      insideQuotes = true;
      continue;
    }

    if (character === ",") {
      commitField();
      continue;
    }

    if (character === "\r") {
      continue;
    }

    if (character === "\n") {
      commitRecord();
      continue;
    }

    currentField += character;
  }

  if (insideQuotes) {
    return null;
  }

  if (currentField.length > 0 || currentRecord.length > 0) {
    commitRecord();
  }

  return records;
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
 * @param {CommandDescriptor} command
 * @returns {string}
 */
function inferTaskForCommand(command) {
  if (!command) {
    return taskAdvancedValue;
  }

  if (command.path === commandPathAuditValue) {
    return taskInspectValue;
  }

  if (command.target.group === commandGroupBranchValue) {
    return taskBranchValue;
  }

  if (command.target.group === commandGroupFilesValue) {
    return taskFilesValue;
  }

  if (command.target.group === commandGroupRemoteValue) {
    return taskRemotesValue;
  }

  if (command.target.group === commandGroupPullRequestsValue || command.target.group === commandGroupPackagesValue) {
    return taskCleanupValue;
  }

  if (command.path === commandPathWorkflowValue) {
    return taskWorkflowsValue;
  }

  return taskAdvancedValue;
}

/**
 * @param {RepositoryDescriptor} repository
 * @returns {string}
 */
function buildRepositorySummary(repository) {
  const fragments = [repository.context_current ? "Launch-context repository for selected scope actions" : "Selected repository for scope-sensitive actions"];
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
