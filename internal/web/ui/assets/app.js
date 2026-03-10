// @ts-check

import { Wunderbaum } from "https://cdn.jsdelivr.net/npm/wunderbaum@0/+esm";

window.addEventListener("error", (event) => {
  reportBootstrapFailure(event.error?.stack || event.message || String(event.error || ""));
});

window.addEventListener("unhandledrejection", (event) => {
  reportBootstrapFailure(event.reason?.stack || event.reason?.message || String(event.reason || ""));
});

/**
 * @param {string} message
 * @returns {void}
 */
function reportBootstrapFailure(message) {
  const failureMessage = String(message || "").trim();
  if (!failureMessage) {
    return;
  }

  const runErrorElement = document.querySelector("#run-error");
  if (runErrorElement instanceof HTMLElement) {
    runErrorElement.textContent = failureMessage;
  }

  const statusElement = document.querySelector("#run-status");
  if (statusElement instanceof HTMLElement) {
    statusElement.textContent = "failed";
    statusElement.className = "status-pill status-failed";
  }
}

/**
 * @typedef {{
 *   launch_path?: string,
 *   launch_roots?: string[],
 *   explorer_root?: string,
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
 *   include_all?: boolean,
 * }} AuditInspectionRequest
 */

/**
 * @typedef {{
 *   name: string,
 *   path: string,
 * }} FolderDescriptor
 */

/**
 * @typedef {{
 *   path?: string,
 *   folders?: FolderDescriptor[],
 *   error?: string,
 * }} DirectoryListing
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
 *   queued?: boolean,
 *   queuedChangeID?: string,
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

/**
 * @typedef {{
 *   key: string,
 *   title: string,
 *   path: string,
 *   absolute_path?: string,
 *   configured_root?: boolean,
 *   configured_root_ancestor?: boolean,
 *   kind: "folder" | "repository",
 *   search_text: string,
 *   repository?: RepositoryDescriptor,
 *   children: RepoTreeNodeModel[],
 * }} RepoTreeNodeModel
 */

const repositoriesEndpoint = "/api/repos";
const commandsEndpoint = "/api/commands";
const foldersEndpoint = "/api/folders";
const auditInspectEndpoint = "/api/audit/inspect";
const auditApplyEndpoint = "/api/audit/apply";
const runsEndpoint = "/api/runs";
const pollIntervalMilliseconds = 800;
const currentRepoLaunchMode = "current_repo";
const configuredRootsLaunchMode = "configured_roots";
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
const auditChangeKindConvertProtocolValue = "convert_protocol";
const auditChangeKindDeleteFolderValue = "delete_folder";
const auditChangeKindSyncWithRemoteValue = "sync_with_remote";
const auditSyncStrategyRequireCleanValue = "require_clean";
const auditSyncStrategyStashChangesValue = "stash_changes";
const auditSyncStrategyCommitChangesValue = "commit_changes";
const auditChangeStatusSucceededValue = "succeeded";
const auditChangeStatusSkippedValue = "skipped";
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
  path: "Repo Path",
  folder_name: "Folder",
  final_github_repo: "GitHub",
  origin_remote_status: "Origin",
  name_matches: "Name",
  remote_default_branch: "Remote",
  local_branch: "Local Branch",
  in_sync: "Sync",
  remote_protocol: "Protocol",
  origin_matches_canonical: "Canonical",
  worktree_dirty: "Dirty",
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

const repositoryTreeIconMap = Object.freeze({
  expanderCollapsed: "repo-tree-expander-icon repo-tree-expander-collapsed",
  expanderExpanded: "repo-tree-expander-icon repo-tree-expander-expanded",
  checkUnchecked: "repo-tree-checkbox-icon repo-tree-checkbox-unchecked",
  checkChecked: "repo-tree-checkbox-icon repo-tree-checkbox-checked",
  checkUnknown: "repo-tree-checkbox-icon repo-tree-checkbox-partial",
  folder: "repo-tree-node-icon repo-tree-folder-icon",
  folderOpen: "repo-tree-node-icon repo-tree-folder-open-icon",
  doc: "repo-tree-node-icon repo-tree-repository-icon",
  loading: "repo-tree-node-icon repo-tree-loading-icon",
  error: "repo-tree-node-icon repo-tree-error-icon",
});

/** @type {any | null} */
let repositoryTreeControl = null;
const pendingDirectoryLoads = new Map();

/** @type {{
 *   repositoryCatalog: RepositoryCatalog | null,
 *   repositories: RepositoryDescriptor[],
 *   directoryFolders: Record<string, FolderDescriptor[]>,
 *   checkedRepositoryIDs: string[],
 *   selectedRepositoryID: string,
 *   activeRepositoryTreeKey: string,
 *   selectedFolderPath: string,
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
 *   auditColumnFilters: Record<string, string>,
 *   auditQueue: AuditQueueEntry[],
 *   auditQueueVisible: boolean,
 *   auditQueueApplying: boolean,
 *   nextAuditChangeSequence: number,
 *   collapsedFolderPaths: string[],
 *   pollTimer: number | null,
 * }} */
const state = {
  repositoryCatalog: null,
  repositories: [],
  directoryFolders: {},
  checkedRepositoryIDs: [],
  selectedRepositoryID: "",
  activeRepositoryTreeKey: "",
  selectedFolderPath: "",
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
  auditColumnFilters: {},
  auditQueue: [],
  auditQueueVisible: false,
  auditQueueApplying: false,
  nextAuditChangeSequence: 1,
  collapsedFolderPaths: [],
  pollTimer: null,
};

const elements = {
  repoCount: document.querySelector("#repo-count"),
  repoLaunchSummary: document.querySelector("#repo-launch-summary"),
  repoFilter: document.querySelector("#repo-filter"),
  repoTree: document.querySelector("#repo-tree"),
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
  auditSelectionBadge: document.querySelector("#audit-selection-badge"),
  auditSelectionSummary: document.querySelector("#audit-selection-summary"),
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
  const visibleRepositories = (repositoryCatalog.repositories || []).slice().sort(compareRepositories);

  state.repositoryCatalog = repositoryCatalog;
  state.repositories = visibleRepositories;
  state.collapsedFolderPaths = [];
  state.allCommands = (commandCatalog.commands || []).slice().sort((left, right) => left.path.localeCompare(right.path));
  state.actionableCommands = state.allCommands.filter((command) => command.actionable);
  state.advancedCommands = state.actionableCommands.filter((command) => inferTaskForCommand(command) === taskAdvancedValue);

  const initialRepositoryID = state.repositories.some((repository) => repository.id === repositoryCatalog.selected_repository_id)
    ? repositoryCatalog.selected_repository_id || ""
    : "";
  state.activeRepositoryTreeKey = preferredInitialRepositoryTreeKey(initialRepositoryID);
  state.selectedFolderPath = preferredInitialRepositoryTreeFolderPath();
  const initialRepository = repositoryForFolderPath(state.selectedFolderPath);
  state.selectedRepositoryID = initialRepository?.id || "";
  state.checkedRepositoryIDs = initialRepository ? [initialRepository.id] : [];

  elements.repoCount.textContent = String(state.repositories.length);
  elements.taskCount.textContent = "6";
  elements.commandCount.textContent = String(state.advancedCommands.length);
  elements.targetRefMode.value = state.targetRefMode;
  elements.targetPathMode.value = state.targetPathMode;
  elements.fileTaskMode.value = state.fileTaskMode;

  renderRepositoryLaunchSummary();
  await renderRepositoryTree("");
  renderTargetState();
  renderTaskState();
  renderActionGroups("");

  if (state.selectedRepositoryID) {
    await selectRepository(state.selectedRepositoryID);
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
    void renderRepositoryTree(elements.repoFilter.value.trim().toLowerCase());
  });
  elements.repoTree.addEventListener("click", handleRepositoryTreeAuditRootSelection);

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

  elements.auditIncludeAll.addEventListener("change", handleAuditDraftChange);
  elements.auditResultsBody.addEventListener("click", handleAuditResultsClick);
  elements.auditResultsHead.addEventListener("change", handleAuditResultsHeadChange);
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
    syncAuditDraftFromArguments();
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

  elements.taskPanelInspect.hidden = false;
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
  const auditScopeRoots = workingFolderRoots();
  elements.auditRootsInput.value = formatAuditRootsInput(auditScopeRoots);
  elements.auditRootsInput.placeholder = auditScopeRoots.length > 0
    ? `${auditScopeRoots.join(", ")}`
    : "Select a folder in the tree to define the audit scope.";
  elements.taskInspectLoad.disabled = resolveAuditRoots().length === 0;
  renderAuditSelectionSummary();
}

function renderAuditSelectionSummary() {
  const selectedFolderPath = activeRepositoryTreeFolderPath();
  const launchRoots = configuredLaunchRoots();
  const scopeRepositories = repositoryScopeRepositories();
  const includeAllEnabled = Boolean(elements.auditIncludeAll?.checked);

  if (selectedFolderPath) {
    elements.auditSelectionBadge.textContent = "Selected folder";
    elements.auditSelectionSummary.textContent = `Audit ${selectedFolderPath}.${includeAllEnabled ? " Non-Git folders under this root will be included." : " Select another folder in the left tree when you want to move the audit target."}`;
    return;
  }

  if (launchRoots.length > 0) {
    elements.auditSelectionBadge.textContent = launchRoots.length === 1 ? "1 launch root" : `${launchRoots.length} launch roots`;
    elements.auditSelectionSummary.textContent = launchRoots.length === 1
      ? `Audit ${launchRoots[0]}.${includeAllEnabled ? " Non-Git folders under this root will be included." : " Select a folder in the left tree when you want to narrow the audit target."}`
      : `Audit ${summarizeAuditSelectionValues(launchRoots)}.${includeAllEnabled ? " Non-Git folders under those roots will be included." : " Select a folder in the left tree when you want to narrow the audit target."}`;
    return;
  }

  if (scopeRepositories.length === 0) {
    elements.auditSelectionBadge.textContent = "No target";
    elements.auditSelectionSummary.textContent = "Select a repository on the left or click a folder in the tree to prepare the next audit run.";
    return;
  }

  if (state.selectedScope === scopeSelectedValue && scopeRepositories.length === 1) {
    const repository = scopeRepositories[0];
    elements.auditSelectionBadge.textContent = "Selected repo";
    elements.auditSelectionSummary.textContent = `Audit ${repository.name} at ${repository.path}.${includeAllEnabled ? " Non-Git folders under this root will be included." : " Click a folder in the left tree to narrow the run."}`;
    return;
  }

  const scopeLabel = state.selectedScope === scopeCheckedValue ? "Checked scope" : "All repos";
  elements.auditSelectionBadge.textContent = scopeLabel;
  elements.auditSelectionSummary.textContent = `Audit ${scopeRepositories.length} repositories from the ${scopeLabel.toLowerCase()}: ${summarizeAuditSelectionValues(scopeRepositories.map((repository) => repository.name))}.${includeAllEnabled ? " Non-Git folders under those roots will be included." : " Click a folder in the left tree when you want to narrow the run."}`;
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
  const inspectionRequest = resolveAuditInspectionRequest();
  if ((inspectionRequest.roots || []).length === 0) {
    hideAuditResults();
    clearPolling();
    renderRunError("Select a folder in the tree to inspect.");
    setStatus("failed");
    return;
  }

  await inspectAuditRequest(inspectionRequest, clearOutput);
}

/**
 * @returns {AuditInspectionRequest}
 */
function resolveAuditInspectionRequest() {
  return {
    roots: resolveAuditRoots(),
    include_all: Boolean(elements.auditIncludeAll.checked),
  };
}

/**
 * @param {AuditInspectionRequest} inspectionRequest
 * @param {boolean} clearOutput
 */
async function inspectAuditRequest(inspectionRequest, clearOutput) {
  const auditRoots = (inspectionRequest.roots || []).slice();

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
        include_all: Boolean(inspectionRequest.include_all),
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

    state.auditInspectionRoots = auditRoots;
    state.auditInspectionIncludeAll = Boolean(inspectionRequest.include_all);
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

  const launchMode = catalog.launch_mode === currentRepoLaunchMode
    ? "Current repo mode"
    : catalog.launch_mode === configuredRootsLaunchMode
      ? "Explicit roots mode"
      : "Discovery mode";
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

  renderAuditTaskState();
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
async function renderRepositoryTree(query) {
  if (!(elements.repoTree instanceof HTMLElement)) {
    return;
  }

  elements.repoTree.classList.remove("wb-skeleton", "wb-initializing");

  const treeModel = buildRepositoryTreeModel(state.repositories);
  if (treeModel.length === 0) {
    elements.repoTree.innerHTML = "";
    repositoryTreeControl = null;
    appendEmptyState(elements.repoTree, state.repositoryCatalog?.error || "No repositories match the current filter.");
    return;
  }

  if (!repositoryTreeControl) {
    elements.repoTree.innerHTML = "";
  }

  const expandedKeys = new Set(repositoryTreeExpandedKeys());
  repositoryTreeControl = ensureRepositoryTreeControl();
  await repositoryTreeControl.load(buildRepositoryTreeSource(treeModel, expandedKeys));
  applyRepositoryTreeFilter(query);
  await focusActiveRepositoryTreeNode();
}

function ensureRepositoryTreeControl() {
  if (repositoryTreeControl) {
    return repositoryTreeControl;
  }

  repositoryTreeControl = new Wunderbaum({
    element: elements.repoTree,
    source: [],
    selectMode: "multi",
    checkbox: (event) => Boolean(String(event.node?.data?.repository_id || "")),
    iconMap: repositoryTreeIconMap,
    filter: {
      autoApply: true,
      autoExpand: true,
      mode: "hide",
      noData: "No repositories match the current filter.",
    },
    click: (event) => {
      const previousActiveRepositoryTreeKey = state.activeRepositoryTreeKey;
      const nodeKey = String(event.node?.key || "");
      const clickedActiveNode = Boolean(nodeKey) && nodeKey === previousActiveRepositoryTreeKey;
      if (nodeKey) {
        state.activeRepositoryTreeKey = nodeKey;
        if (typeof event.node?.setActive === "function") {
          void event.node.setActive(true, { noEvents: true });
        }
      }

      const nodeKind = String(event.node?.data?.kind || "");
      if (nodeKind !== "folder") {
        return;
      }

      state.selectedFolderPath = repositoryTreeNodeFolderPath(event.node?.data);
      void syncSelectedRepositoryFromTreeNode(event.node?.data);
      const expanded = typeof event.node?.isExpanded === "function"
        ? Boolean(event.node.isExpanded())
        : Boolean(event.node?.expanded);
      const nextExpanded = clickedActiveNode ? !expanded : true;
      setRepositoryTreeFolderCollapsed(state.selectedFolderPath, !nextExpanded);
      if (typeof event.node?.setExpanded === "function") {
        void event.node.setExpanded(nextExpanded);
      }

      if (typeof event.node?.setExpanded === "function") {
        return false;
      }
    },
    activate: (event) => {
      const nodeKey = String(event.node?.key || "");
      if (nodeKey) {
        state.activeRepositoryTreeKey = nodeKey;
      }

      if (String(event.node?.data?.kind || "") === "folder") {
        state.selectedFolderPath = repositoryTreeNodeFolderPath(event.node?.data);
        void syncSelectedRepositoryFromTreeNode(event.node?.data);
        syncAuditSelectionFromTree();
      }
    },
    select: (event) => {
      const repositoryID = String(event.node.data?.repository_id || "");
      if (!repositoryID) {
        return;
      }
      toggleCheckedRepository(repositoryID, Boolean(event.node.selected));
    },
    render: annotateRepositoryTreeNode,
  });

  return repositoryTreeControl;
}

function repositoryTreeExpandedKeys() {
  if (!repositoryTreeControl) {
    return [];
  }

  const treeState = repositoryTreeControl.getState({ expandedKeys: true, selectedKeys: false });
  if (!Array.isArray(treeState.expandedKeys)) {
    return [];
  }
  return treeState.expandedKeys.slice();
}

/**
 * @param {string} query
 */
function applyRepositoryTreeFilter(query) {
  if (!repositoryTreeControl) {
    return;
  }

  if (!query) {
    repositoryTreeControl.clearFilter();
    return;
  }

  repositoryTreeControl.filterNodes((node) => repositoryTreeNodeMatchesQuery(node, query), {
    autoExpand: true,
    mode: "hide",
    noData: "No repositories match the current filter.",
  });
}

async function focusActiveRepositoryTreeNode() {
  if (!repositoryTreeControl) {
    return;
  }

  const preferredKeys = [];
  if (state.activeRepositoryTreeKey) {
    preferredKeys.push(state.activeRepositoryTreeKey);
  }
  const selectedFolderPath = normalizeRepositoryTreePath(state.selectedFolderPath || "");
  if (selectedFolderPath) {
    preferredKeys.push(repositoryTreeFolderKey(selectedFolderPath));
  }
  const selectedRepositoryPath = normalizeRepositoryTreePath(selectedRepository()?.path || "");
  if (selectedRepositoryPath) {
    preferredKeys.push(repositoryTreeFolderKey(selectedRepositoryPath));
  }

  for (const preferredKey of preferredKeys) {
    const activeNode = repositoryTreeControl.findKey(preferredKey);
    if (!activeNode) {
      continue;
    }

    if (preferredKey !== state.activeRepositoryTreeKey) {
      state.activeRepositoryTreeKey = preferredKey;
    }

    await activeNode.setActive(true, { noEvents: true });
    return;
  }
}

/**
 * @param {any} node
 * @param {string} query
 * @returns {boolean | "branch"}
 */
function repositoryTreeNodeMatchesQuery(node, query) {
  const searchText = String(node.data?.search_text || "").toLowerCase();
  if (!searchText.includes(query)) {
    return false;
  }
  return String(node.data?.kind || "") === "folder" ? "branch" : true;
}

/**
 * @param {RepoTreeNodeModel[]} treeModel
 * @param {Set<string>} expandedKeys
 * @returns {object[]}
 */
function buildRepositoryTreeSource(treeModel, expandedKeys) {
  return treeModel.map((node) => {
    return {
      key: node.key,
      title: node.title,
      expanded: repositoryTreeShouldExpandFolder(node, expandedKeys),
      unselectable: true,
      selected: state.checkedRepositoryIDs.includes(node.repository?.id || ""),
      checkbox: Boolean(node.repository),
      configured_root: Boolean(node.configured_root),
      configured_root_ancestor: Boolean(node.configured_root_ancestor),
      kind: "folder",
      label: node.title,
      path: node.path,
      absolute_path: node.absolute_path || "",
      repository_id: node.repository?.id || "",
      repository_name: node.repository?.name || node.title,
      repository_path: node.repository?.path || node.path,
      search_text: node.search_text,
      children: buildRepositoryTreeSource(node.children, expandedKeys),
    };
  });
}

/**
 * @param {RepositoryDescriptor[]} repositories
 * @returns {RepoTreeNodeModel[]}
 */
function buildRepositoryTreeModel(repositories) {
  return buildFolderExplorerTreeModel(repositories, explorerRootPaths());
}

/**
 * @returns {string[]}
 */
function configuredLaunchRoots() {
  return (Array.isArray(state.repositoryCatalog?.launch_roots)
    ? state.repositoryCatalog.launch_roots
    : [])
    .map((rootPath) => normalizeRepositoryTreePath(rootPath))
    .filter(Boolean);
}

/**
 * @returns {string[]}
 */
function explorerRootPaths() {
  const launchRoots = configuredLaunchRoots();
  if (launchRoots.length > 0) {
    return launchRoots;
  }

  const explorerRoot = normalizeRepositoryTreePath(state.repositoryCatalog?.explorer_root || state.repositoryCatalog?.launch_path || "");
  return explorerRoot ? [explorerRoot] : [];
}

/**
 * @param {RepositoryDescriptor[]} repositories
 * @param {string[]} rootPaths
 * @returns {RepoTreeNodeModel[]}
 */
function buildFolderExplorerTreeModel(repositories, rootPaths) {
  const normalizedRootPaths = rootPaths
    .map((rootPath) => normalizeRepositoryTreePath(rootPath))
    .filter(Boolean);
  if (normalizedRootPaths.length === 0) {
    return [];
  }

  /** @type {Map<string, RepoTreeNodeModel>} */
  const nodeIndex = new Map();
  /** @type {RepoTreeNodeModel[]} */
  const treeModel = [];

  normalizedRootPaths.forEach((rootPath) => {
    ensureFolderExplorerRootNode(treeModel, nodeIndex, rootPath);
  });

  repositories.forEach((repository) => {
    if (!repositoryVisibleInTree(repository)) {
      return;
    }

    const repositoryPath = normalizeRepositoryTreePath(repository.path);
    const rootPath = configuredLaunchRootForRepository(repositoryPath, normalizedRootPaths);
    if (!rootPath) {
      return;
    }

    appendFolderExplorerRepository(treeModel, nodeIndex, rootPath, repository);
  });

  sortRepositoryTreeNodes(treeModel);
  return treeModel;
}

/**
 * @param {RepoTreeNodeModel[]} treeModel
 * @param {Map<string, RepoTreeNodeModel>} nodeIndex
 * @param {string} rootPath
 * @returns {RepoTreeNodeModel}
 */
function ensureFolderExplorerRootNode(treeModel, nodeIndex, rootPath) {
  let rootNode = nodeIndex.get(rootPath);
  if (rootNode) {
    return rootNode;
  }

  rootNode = newFolderExplorerNode(rootPath);
  rootNode.configured_root = configuredLaunchRoots().includes(rootPath);
  nodeIndex.set(rootPath, rootNode);
  treeModel.push(rootNode);
  return rootNode;
}

/**
 * @param {string} folderPath
 * @returns {RepoTreeNodeModel}
 */
function newFolderExplorerNode(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  const folderLabel = configuredLaunchRootLabel(normalizedFolderPath);
  return {
    key: repositoryTreeFolderKey(normalizedFolderPath),
    title: folderLabel,
    path: normalizedFolderPath,
    absolute_path: normalizedFolderPath,
    kind: "folder",
    search_text: `${folderLabel} ${normalizedFolderPath}`.toLowerCase(),
    children: [],
  };
}

/**
 * @param {RepoTreeNodeModel[]} treeModel
 * @param {Map<string, RepoTreeNodeModel>} nodeIndex
 * @param {string} rootPath
 * @param {RepositoryDescriptor} repository
 * @returns {void}
 */
function appendFolderExplorerRepository(treeModel, nodeIndex, rootPath, repository) {
  const normalizedRootPath = normalizeRepositoryTreePath(rootPath);
  const repositoryPath = normalizeRepositoryTreePath(repository.path);
  if (!normalizedRootPath || !repositoryPath) {
    return;
  }

  let currentNode = ensureFolderExplorerRootNode(treeModel, nodeIndex, normalizedRootPath);
  if (repositoryPath === normalizedRootPath) {
    attachRepositoryTreeMetadata(currentNode, repository);
    return;
  }

  const relativePath = repositoryPath.startsWith(`${normalizedRootPath}/`)
    ? repositoryPath.slice(normalizedRootPath.length + 1)
    : "";
  const relativeSegments = splitRepositoryTreePath(relativePath);
  let currentPath = normalizedRootPath;
  relativeSegments.forEach((segment) => {
    currentPath = `${currentPath}/${segment}`;
    currentNode = ensureFolderExplorerChildNode(currentNode, nodeIndex, currentPath);
  });

  attachRepositoryTreeMetadata(currentNode, repository);
}

/**
 * @param {RepoTreeNodeModel} parentNode
 * @param {Map<string, RepoTreeNodeModel>} nodeIndex
 * @param {string} folderPath
 * @returns {RepoTreeNodeModel}
 */
function ensureFolderExplorerChildNode(parentNode, nodeIndex, folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  let childNode = nodeIndex.get(normalizedFolderPath);
  if (childNode) {
    return childNode;
  }

  childNode = newFolderExplorerNode(normalizedFolderPath);
  nodeIndex.set(normalizedFolderPath, childNode);
  parentNode.children.push(childNode);
  return childNode;
}

/**
 * @param {RepoTreeNodeModel} node
 * @param {RepositoryDescriptor} repository
 * @returns {void}
 */
function attachRepositoryTreeMetadata(node, repository) {
  node.repository = repository;
  node.search_text = `${node.title} ${repositorySearchText(repository)}`;
}

/**
 * @returns {Set<string>}
 */
function collapsedRepositoryTreeFolderPaths() {
  return new Set((state.collapsedFolderPaths || [])
    .map((folderPath) => normalizeRepositoryTreePath(folderPath))
    .filter(Boolean));
}

/**
 * @param {string} folderPath
 * @param {boolean} collapsed
 * @returns {void}
 */
function setRepositoryTreeFolderCollapsed(folderPath, collapsed) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return;
  }

  if (collapsed) {
    if (collapsedRepositoryTreeFolderPaths().has(normalizedFolderPath)) {
      return;
    }
    state.collapsedFolderPaths = state.collapsedFolderPaths.concat([normalizedFolderPath]);
    return;
  }

  state.collapsedFolderPaths = state.collapsedFolderPaths.filter((path) => normalizeRepositoryTreePath(path) !== normalizedFolderPath);
}

/**
 * @param {string} repositoryPath
 * @param {string[]} launchRoots
 * @returns {string}
 */
function configuredLaunchRootForRepository(repositoryPath, launchRoots) {
  const matchingRoots = launchRoots.filter((launchRoot) => repositoryPath === launchRoot || repositoryPath.startsWith(`${launchRoot}/`));
  matchingRoots.sort((left, right) => right.length - left.length);
  return matchingRoots[0] || "";
}

/**
 * @param {string} launchRoot
 * @returns {string}
 */
function configuredLaunchRootLabel(launchRoot) {
  const rootSegments = splitRepositoryTreePath(launchRoot);
  if (rootSegments.length > 0) {
    return rootSegments[rootSegments.length - 1];
  }

  return launchRoot || ".";
}

/**
 * @param {RepositoryDescriptor} repository
 * @returns {boolean}
 */
function repositoryVisibleInTree(repository) {
  return Boolean(repository);
}

/**
 * @param {RepoTreeNodeModel[]} nodes
 */
function sortRepositoryTreeNodes(nodes) {
  nodes.sort((left, right) => {
    return left.title.localeCompare(right.title, undefined, { numeric: true, sensitivity: "base" });
  });
  nodes.forEach((node) => {
    if (node.children.length > 0) {
      sortRepositoryTreeNodes(node.children);
    }
  });
}

/**
 * @param {string} folderPath
 * @returns {string}
 */
function repositoryTreeFolderKey(folderPath) {
  return `folder:${normalizeRepositoryTreePath(folderPath)}`;
}

/**
 * @param {RepositoryDescriptor} repository
 * @returns {string}
 */
function repositorySearchText(repository) {
  return [repository.name, repository.path, repository.current_branch || "", repository.default_branch || ""]
    .join(" ")
    .toLowerCase();
}

/**
 * @param {RepoTreeNodeModel} node
 * @param {Set<string>} expandedKeys
 * @returns {boolean}
 */
function repositoryTreeShouldExpandFolder(node, expandedKeys) {
  const folderPath = normalizeRepositoryTreePath(node.absolute_path || node.path);
  if (folderPath && collapsedRepositoryTreeFolderPaths().has(folderPath)) {
    return false;
  }

  if (expandedKeys.has(node.key)) {
    return true;
  }

  if (!folderPath) {
    return false;
  }

  if (explorerRootPaths().includes(folderPath)) {
    return true;
  }

  const selectedFolderPath = normalizeRepositoryTreePath(state.selectedFolderPath || "");
  if (selectedFolderPath && repositoryTreePathWithin(folderPath, selectedFolderPath)) {
    return true;
  }

  return false;
}

/**
 * @param {string} folderPath
 * @returns {boolean}
 */
function revealConfiguredRootAncestor(folderPath) {
  if (state.repositoryCatalog?.launch_mode !== configuredRootsLaunchMode) {
    return false;
  }

  const launchRoot = singleConfiguredLaunchRoot();
  if (!launchRoot) {
    return false;
  }

  const anchorPath = singleConfiguredRootAnchorPath(launchRoot);
  const anchorSegments = splitRepositoryTreePath(anchorPath);
  const visiblePaths = singleConfiguredRootVisiblePaths(anchorPath);
  if (anchorSegments.length <= visiblePaths.length || visiblePaths.length < 1) {
    return false;
  }

  const topVisibleAncestorPath = visiblePaths[0];
  if (!topVisibleAncestorPath || normalizeRepositoryTreePath(folderPath) !== topVisibleAncestorPath) {
    return false;
  }

  state.selectedFolderPath = topVisibleAncestorPath;
  setRepositoryTreeFolderCollapsed(topVisibleAncestorPath, false);
  recordSingleConfiguredRootFolderPath(topVisibleAncestorPath);
  state.configuredRootAncestorDepth += 1;
  void ensureSingleConfiguredRootTreeDataLoaded().then(() => renderRepositoryTree(elements.repoFilter.value.trim().toLowerCase()));
  return true;
}

/**
 * @returns {boolean}
 */
function expandCurrentRepoExplorerFromLeaf() {
  if (state.repositoryCatalog?.launch_mode !== currentRepoLaunchMode || state.currentRepoTreeExpanded) {
    return false;
  }

  state.currentRepoTreeExpanded = true;
  state.currentRepoAncestorDepth = 1;
  void renderRepositoryTree(elements.repoFilter.value.trim().toLowerCase());
  return true;
}

/**
 * @param {string} folderPath
 * @returns {boolean}
 */
function expandCurrentRepoExplorerFromFolder(folderPath) {
  if (state.repositoryCatalog?.launch_mode !== currentRepoLaunchMode || state.currentRepoTreeExpanded) {
    return false;
  }

  const explorerRootPath = currentRepositoryExplorerRootPath();
  const absoluteFolderPath = currentRepositoryVisibleFolderAbsolutePath(folderPath);
  if (!explorerRootPath || !absoluteFolderPath || absoluteFolderPath !== explorerRootPath) {
    return false;
  }

  setRepositoryTreeFolderCollapsed(absoluteFolderPath, false);
  state.currentRepoTreeExpanded = true;
  state.currentRepoAncestorDepth = 1;
  void renderRepositoryTree(elements.repoFilter.value.trim().toLowerCase());
  return true;
}

/**
 * @param {string} folderPath
 * @returns {boolean}
 */
function revealCurrentRepoAncestor(folderPath) {
  if (state.repositoryCatalog?.launch_mode !== currentRepoLaunchMode) {
    return false;
  }

  const anchorSegments = splitRepositoryTreePath(currentRepositoryAnchorPath());
  const visibleSegments = currentRepositoryVisibleAncestorSegments();
  if (visibleSegments.length === 0 || anchorSegments.length <= visibleSegments.length) {
    return false;
  }

  const topVisibleFolderPath = visibleSegments.length <= 1 ? "" : visibleSegments[0];
  if (!topVisibleFolderPath || normalizeRepositoryTreePath(folderPath) !== topVisibleFolderPath) {
    return false;
  }

  setRepositoryTreeFolderCollapsed(topVisibleFolderPath, false);
  state.currentRepoAncestorDepth += 1;
  void renderRepositoryTree(elements.repoFilter.value.trim().toLowerCase());
  return true;
}

/**
 * @param {string} repositoryPath
 * @returns {string[]}
 */
function currentRepositoryModeTreeSegments(repositoryPath) {
  const normalizedRepositoryPath = normalizeRepositoryTreePath(repositoryPath);
  if (!normalizedRepositoryPath) {
    return [];
  }

  const visibleAncestorSegments = currentRepositoryVisibleAncestorSegments();
  if (visibleAncestorSegments.length === 0) {
    return [];
  }

  if (state.currentRepoTreeExpanded) {
    const explorerRootPath = currentRepositoryExplorerRootPath();
    if (explorerRootPath && (normalizedRepositoryPath === explorerRootPath || normalizedRepositoryPath.startsWith(`${explorerRootPath}/`))) {
      const relativePath = normalizedRepositoryPath === explorerRootPath
        ? ""
        : normalizedRepositoryPath.slice(explorerRootPath.length + 1);
      const relativeSegments = splitRepositoryTreePath(relativePath);
      return relativeSegments.length > 0
        ? visibleAncestorSegments.concat(relativeSegments)
        : visibleAncestorSegments.slice();
    }
    return [];
  }

  const currentRepositoryPath = currentRepositoryPathValue();
  if (normalizedRepositoryPath === currentRepositoryPath) {
    return visibleAncestorSegments;
  }

  return [];
}

/**
 * @returns {string}
 */
function currentRepositoryPathValue() {
  return normalizeRepositoryTreePath(selectedRepository()?.path || state.repositories[0]?.path || "");
}

/**
 * @returns {string}
 */
function currentRepositoryExplorerRootPath() {
  return normalizeRepositoryTreePath(state.repositoryCatalog?.explorer_root || "");
}

/**
 * @returns {string}
 */
function currentRepositoryAnchorPath() {
  return state.currentRepoTreeExpanded ? currentRepositoryExplorerRootPath() : currentRepositoryPathValue();
}

/**
 * @returns {string[]}
 */
function currentRepositoryVisibleAncestorSegments() {
  const anchorSegments = splitRepositoryTreePath(currentRepositoryAnchorPath());
  if (anchorSegments.length === 0) {
    return [];
  }

  const visibleDepth = Math.min(
    anchorSegments.length,
    Math.max(1, (state.currentRepoAncestorDepth || 0) + 1),
  );
  return anchorSegments.slice(anchorSegments.length - visibleDepth);
}

/**
 * @returns {Set<string>}
 */
function currentRepositoryAutoExpandFolderPaths() {
  const visibleAncestorSegments = currentRepositoryVisibleAncestorSegments();
  const autoExpandedPaths = new Set();
  visibleAncestorSegments.forEach((_, segmentIndex) => {
    autoExpandedPaths.add(visibleAncestorSegments.slice(0, segmentIndex + 1).join("/"));
  });
  return autoExpandedPaths;
}

/**
 * @param {string} folderPath
 * @returns {string}
 */
function currentRepositoryVisibleFolderAbsolutePath(folderPath) {
  const currentRepositoryPath = currentRepositoryPathValue();
  if (!currentRepositoryPath) {
    return "";
  }

  const visibleSegments = currentRepositoryVisibleAncestorSegments();
  const visibleFolderSegments = visibleSegments.slice(0, -1);
  if (visibleFolderSegments.length === 0) {
    return "";
  }

  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  for (let folderDepth = 1; folderDepth <= visibleFolderSegments.length; folderDepth += 1) {
    const relativeFolderPath = visibleFolderSegments.slice(0, folderDepth).join("/");
    if (relativeFolderPath !== normalizedFolderPath) {
      continue;
    }

    const suffixSegments = visibleSegments.slice(folderDepth);
    if (suffixSegments.length === 0) {
      return currentRepositoryPath;
    }

    const suffixPath = `/${suffixSegments.join("/")}`;
    if (!currentRepositoryPath.endsWith(suffixPath)) {
      return "";
    }

    return currentRepositoryPath.slice(0, currentRepositoryPath.length - suffixPath.length);
  }

  return "";
}

/**
 * @param {string} rawPath
 * @returns {string[]}
 */
function splitRepositoryTreePath(rawPath) {
  return normalizeRepositoryTreePath(rawPath)
    .split("/")
    .map((segment) => segment.trim())
    .filter(Boolean);
}

/**
 * @param {string} rawPath
 * @returns {string}
 */
function normalizeRepositoryTreePath(rawPath) {
  return String(rawPath || "")
    .replace(/\\/g, "/")
    .replace(/\/+/g, "/")
    .replace(/\/$/, "");
}

/**
 * @param {{ node: any, nodeElem: HTMLElement }} event
 */
function annotateRepositoryTreeNode(event) {
  const nodeElement = event.nodeElem;
  const rowElement = nodeElement.closest(".wb-row");
  const titleElement = nodeElement.querySelector(".wb-title");
  const checkboxElement = nodeElement.querySelector(".wb-checkbox");
  const expanderElement = nodeElement.querySelector(".wb-expander");
  const iconElement = nodeElement.querySelector(".wb-icon");
  const label = String(event.node.data?.label || event.node.title || "");
  const kind = String(event.node.data?.kind || "");
  const absolutePath = String(event.node.data?.absolute_path || "");
  let checkboxSpacer = nodeElement.querySelector(".repo-tree-checkbox-spacer");

  if (rowElement instanceof HTMLElement) {
    rowElement.dataset.repoTreeKind = kind;
    rowElement.dataset.repoTreeKey = String(event.node.key || "");
    if (absolutePath) {
      rowElement.dataset.repoTreeAbsolutePath = absolutePath;
    } else {
      delete rowElement.dataset.repoTreeAbsolutePath;
    }
  }
  if (titleElement instanceof HTMLElement && label) {
    titleElement.dataset.repoTreeNode = label;
    if (absolutePath) {
      titleElement.dataset.repoTreeAbsolutePath = absolutePath;
    } else {
      delete titleElement.dataset.repoTreeAbsolutePath;
    }
  }
  if (checkboxElement instanceof HTMLElement && label && String(event.node.data?.repository_id || "")) {
    checkboxElement.dataset.repoTreeCheckbox = label;
    checkboxElement.setAttribute("aria-label", `Include ${label} in checked repositories`);
  }
  if (expanderElement instanceof HTMLElement && label && kind === "folder") {
    expanderElement.dataset.repoTreeExpander = label;
    expanderElement.setAttribute("aria-label", `Toggle ${label} folder`);
  }
  if (kind === "folder") {
    if (checkboxElement instanceof HTMLElement && checkboxSpacer instanceof HTMLElement) {
      checkboxSpacer.remove();
      checkboxSpacer = null;
    } else if (!(checkboxElement instanceof HTMLElement) && !(checkboxSpacer instanceof HTMLElement) && iconElement instanceof HTMLElement) {
      checkboxSpacer = document.createElement("span");
      checkboxSpacer.className = "repo-tree-checkbox-spacer";
      checkboxSpacer.setAttribute("aria-hidden", "true");
      iconElement.parentNode?.insertBefore(checkboxSpacer, iconElement);
    }
  } else if (checkboxSpacer instanceof HTMLElement) {
    checkboxSpacer.remove();
  }
}

/**
 * @param {MouseEvent} event
 */
function handleRepositoryTreeAuditRootSelection(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLElement) || eventTarget.closest(".wb-expander")) {
    return;
  }

  const rowElement = eventTarget.closest(".wb-row");
  if (!(rowElement instanceof HTMLElement) || rowElement.dataset.repoTreeKind !== "folder") {
    return;
  }

  syncAuditSelectionFromTree();
}

/**
 * @returns {void}
 */
function syncAuditSelectionFromTree() {
  renderTaskState();
  if (state.selectedPath === commandPathAuditValue) {
    repopulateSelectedCommand();
  }
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

  await renderRepositoryTree(elements.repoFilter.value.trim().toLowerCase());
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
 * @param {any} nodeData
 * @returns {Promise<void>}
 */
async function syncSelectedRepositoryFromTreeNode(nodeData) {
  const repositoryID = String(nodeData?.repository_id || "").trim();
  if (!repositoryID) {
    clearSelectedRepositoryContext();
    return;
  }

  if (repositoryID === state.selectedRepositoryID) {
    renderTargetState();
    renderSelectedRepository();
    renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
    syncQuickActions();
    repopulateSelectedCommand();
    return;
  }

  await selectRepository(repositoryID);
}

/**
 * @returns {void}
 */
function clearSelectedRepositoryContext() {
  state.selectedRepositoryID = "";
  state.branches = [];
  renderTargetState();
  renderSelectedRepository();
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  syncQuickActions();
  repopulateSelectedCommand();
}

/**
 * @param {string} initialRepositoryID
 * @returns {string}
 */
function preferredInitialRepositoryTreeKey(initialRepositoryID) {
  const initialFolderPath = preferredInitialRepositoryTreeFolderPath();
  if (initialFolderPath) {
    return repositoryTreeFolderKey(initialFolderPath);
  }

  const initialRepository = findRepository(initialRepositoryID);
  return initialRepository ? repositoryTreeFolderKey(initialRepository.path) : "";
}

/**
 * @returns {string}
 */
function preferredInitialRepositoryTreeFolderPath() {
  return explorerRootPaths()[0] || "";
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

  void renderRepositoryTree(elements.repoFilter.value.trim().toLowerCase());
  renderTargetState();
  renderActionGroups(elements.commandFilter.value.trim().toLowerCase());
  syncQuickActions();
  repopulateSelectedCommand();
}

function renderSelectedRepository() {
  const repository = selectedRepository();
  if (!repository) {
    elements.repoTitle.textContent = state.repositoryCatalog?.error ? "Repository context unavailable" : "No repository selected";
    elements.repoPath.textContent = activeRepositoryTreeFolderPath() || state.repositoryCatalog?.launch_path || "";
    elements.repoSummary.textContent = state.repositoryCatalog?.error || "Select a repository folder to scope branch and repository actions.";
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
        elements.actionContext.textContent = "Audit is the primary flow above. Select a folder in the tree to inspect.";
        return;
      }
      elements.actionContext.textContent = `Audit is configured above for ${workingFolderRoots().length} ${workingFolderRoots().length === 1 ? "folder" : "folders"}. Run it there, then return here for follow-up commands.`;
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
      reason: "Select a folder in the tree to inspect.",
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
  return Array.from(new Set(workingFolderRoots()));
}

/**
 * @param {string[]} values
 * @returns {string}
 */
function summarizeAuditSelectionValues(values) {
  const filteredValues = values
    .map((value) => String(value || "").trim())
    .filter(Boolean);

  if (filteredValues.length <= 3) {
    return filteredValues.join(", ");
  }

  return `${filteredValues.slice(0, 3).join(", ")}, and ${filteredValues.length - 3} more`;
}

/**
 * @param {string[]} roots
 * @returns {string}
 */
function formatAuditRootsInput(roots) {
  return roots
    .map((root) => String(root || "").trim())
    .filter(Boolean)
    .join(", ");
}

function syncAuditDraftFromArguments() {
  if (state.selectedPath !== commandPathAuditValue) {
    return;
  }

  const parsedDraftRequest = parseAuditInspectionRequest(readArguments());
  if (!parsedDraftRequest) {
    return;
  }

  elements.auditIncludeAll.checked = parsedDraftRequest.include_all;
  renderTaskState();
  updateActionContext();
}

/**
 * @param {string[]} argumentsList
 * @returns {AuditInspectionRequest | null}
 */
function parseAuditInspectionRequest(argumentsList) {
  if (argumentsList.length === 0 || argumentsList[0] !== auditCommandNameValue) {
    return null;
  }

  const roots = [];
  let includeAll = false;

  for (let argumentIndex = 1; argumentIndex < argumentsList.length; argumentIndex += 1) {
    const argument = argumentsList[argumentIndex];
    if (argument === "--all") {
      includeAll = true;
      continue;
    }
    if (argument === "--roots") {
      const rootValue = argumentsList[argumentIndex + 1] || "";
      if (!rootValue) {
        return null;
      }
      roots.push(rootValue);
      argumentIndex += 1;
      continue;
    }
    if (argument.startsWith("--roots=")) {
      const rootValue = argument.slice("--roots=".length).trim();
      if (!rootValue) {
        return null;
      }
      roots.push(rootValue);
    }
  }

  return {
    roots: Array.from(new Set(roots)),
    include_all: includeAll,
  };
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
  renderRunButtonLabel();
  const argumentsList = readArguments();
  if (argumentsList.length === 0) {
    elements.commandPreview.textContent = "gix";
    return;
  }

  const quotedArguments = argumentsList.map((argument) => quoteShellArgument(argument));
  elements.commandPreview.textContent = `gix ${quotedArguments.join(" ")}`;
}

function renderRunButtonLabel() {
  const selectedCommand = findSelectedCommand();
  elements.runButton.textContent = selectedCommand?.path === commandPathAuditValue ? "Inspect audit table" : "Run";
}

async function submitRun() {
  if (elements.runButton.disabled) {
    return;
  }

  if (findSelectedCommand()?.path === commandPathAuditValue) {
    await runAuditTask();
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
  state.auditInspectionRows = (inspection.rows || []).slice();
  state.auditColumnFilters = {};
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
    const headerCellClassName = auditTableColumnClass(headerName);
    if (headerCellClassName) {
      headerCell.classList.add(headerCellClassName);
    }
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
        const headerName = headers[valueIndex] || "";
        const cellClassName = auditTableColumnClass(headerName);
        if (valueIndex === 0) {
          cell.scope = "row";
        }
        if (cellClassName) {
          cell.classList.add(cellClassName);
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
    const headerCellClassName = auditTableColumnClass(headerName);
    if (headerCellClassName) {
      headerCell.classList.add(headerCellClassName);
    }
    const headerStack = document.createElement("div");
    headerStack.className = "audit-header-stack";

    const headerLabel = document.createElement("span");
    headerLabel.className = "audit-header-label";
    headerLabel.textContent = auditColumnLabels[headerName] || headerName;
    headerStack.append(headerLabel);

    const filterControl = renderAuditColumnFilterControl(headerName, rows);
    if (filterControl) {
      headerStack.append(filterControl);
    }

    headerCell.append(headerStack);
    headerRow.append(headerCell);
  });

  const actionsHeaderCell = document.createElement("th");
  actionsHeaderCell.scope = "col";
  actionsHeaderCell.className = "audit-column-actions";
  actionsHeaderCell.textContent = "Actions";
  headerRow.append(actionsHeaderCell);
  elements.auditResultsHead.append(headerRow);

  const filteredRows = filterTypedAuditRows(rows);
  if (filteredRows.length === 0) {
    const emptyRow = document.createElement("tr");
    const emptyCell = document.createElement("td");
    emptyCell.colSpan = typedAuditHeaderColumns.length + 1;
    emptyCell.textContent = rows.length === 0
      ? "No repositories or folders matched the inspected roots."
      : "No audit rows match the current column filters.";
    emptyRow.append(emptyCell);
    elements.auditResultsBody.append(emptyRow);
  } else {
    filteredRows.forEach((row) => {
      const rowElement = document.createElement("tr");
      const record = typedAuditRecord(row);

      record.forEach((value, valueIndex) => {
        const cell = document.createElement(valueIndex === 0 ? "th" : "td");
        const headerName = typedAuditHeaderColumns[valueIndex] || "";
        const cellClassName = auditTableColumnClass(headerName);
        if (valueIndex === 0) {
          cell.scope = "row";
        }
        if (cellClassName) {
          cell.classList.add(cellClassName);
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
          if (action.queued) {
            button.classList.add("audit-action-button-queued");
          }
          button.dataset.auditAction = action.kind;
          button.dataset.auditPath = row.path;
          if (action.queuedChangeID) {
            button.dataset.auditQueuedChangeId = action.queuedChangeID;
          }
          button.textContent = action.label;
          actionsList.append(button);
        });
        actionsCell.append(actionsList);
      }

      rowElement.append(actionsCell);
      elements.auditResultsBody.append(rowElement);
    });
  }

  elements.auditResultsSummary.textContent = formatAuditResultsSummary(filteredRows.length, rows.length);
  elements.auditResultsPanel.hidden = false;
}

function hideAuditResults() {
  elements.auditResultsPanel.hidden = true;
  elements.auditResultsSummary.textContent = "";
  elements.auditResultsHead.innerHTML = "";
  elements.auditResultsBody.innerHTML = "";
  state.auditColumnFilters = {};
}

/**
 * @param {AuditInspectionRow} row
 * @returns {string[]}
 */
function typedAuditRecord(row) {
  return typedAuditHeaderColumns.map((headerName) => {
    return typedAuditColumnValue(row, headerName);
  });
}

/**
 * @param {string} headerName
 * @returns {string}
 */
function auditTableColumnClass(headerName) {
  if (headerName === "path") {
    return "audit-column-path";
  }
  if (headerName === "final_github_repo") {
    return "audit-column-repository";
  }
  if (headerName === "remote_default_branch" || headerName === "local_branch") {
    return "audit-column-branch";
  }
  if (headerName === "origin_remote_status"
    || headerName === "name_matches"
    || headerName === "in_sync"
    || headerName === "remote_protocol"
    || headerName === "origin_matches_canonical"
    || headerName === "worktree_dirty") {
    return "audit-column-status";
  }
  if (headerName === "dirty_files") {
    return "audit-column-dirty-files";
  }
  return "";
}

/**
 * @param {Event} event
 */
function handleAuditResultsHeadChange(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLSelectElement)) {
    return;
  }

  const headerName = eventTarget.dataset.auditColumnFilter || "";
  if (!headerName) {
    return;
  }

  const nextFilters = { ...state.auditColumnFilters };
  if (eventTarget.value) {
    nextFilters[headerName] = eventTarget.value;
  } else {
    delete nextFilters[headerName];
  }
  state.auditColumnFilters = nextFilters;
  renderTypedAuditTable(state.auditInspectionRows);
}

/**
 * @param {string} headerName
 * @param {AuditInspectionRow[]} rows
 * @returns {HTMLSelectElement | null}
 */
function renderAuditColumnFilterControl(headerName, rows) {
  const optionValues = auditColumnFilterValues(headerName, rows);
  if (optionValues.length <= 1) {
    return null;
  }

  const filterControl = document.createElement("select");
  filterControl.className = "select-input audit-column-filter";
  filterControl.dataset.auditColumnFilter = headerName;

  const allOption = document.createElement("option");
  allOption.value = "";
  allOption.textContent = "All";
  filterControl.append(allOption);

  optionValues.forEach((optionValue) => {
    const option = document.createElement("option");
    option.value = optionValue;
    option.textContent = optionValue;
    filterControl.append(option);
  });

  filterControl.value = state.auditColumnFilters[headerName] || "";
  return filterControl;
}

/**
 * @param {string} headerName
 * @param {AuditInspectionRow[]} rows
 * @returns {string[]}
 */
function auditColumnFilterValues(headerName, rows) {
  return Array.from(new Set(rows.map((row) => typedAuditColumnValue(row, headerName)).filter(Boolean)))
    .sort((left, right) => left.localeCompare(right, undefined, { numeric: true, sensitivity: "base" }));
}

/**
 * @param {AuditInspectionRow[]} rows
 * @returns {AuditInspectionRow[]}
 */
function filterTypedAuditRows(rows) {
  const activeFilters = Object.entries(state.auditColumnFilters).filter(([, filterValue]) => Boolean(filterValue));
  if (activeFilters.length === 0) {
    return rows;
  }

  return rows.filter((row) => activeFilters.every(([headerName, filterValue]) => typedAuditColumnValue(row, headerName) === filterValue));
}

/**
 * @param {AuditInspectionRow} row
 * @param {string} headerName
 * @returns {string}
 */
function typedAuditColumnValue(row, headerName) {
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
}

/**
 * @param {number} visibleCount
 * @param {number} totalCount
 * @returns {string}
 */
function formatAuditResultsSummary(visibleCount, totalCount) {
  if (visibleCount === totalCount) {
    return `${totalCount} ${totalCount === 1 ? "row" : "rows"}`;
  }
  return `${visibleCount} of ${totalCount} ${totalCount === 1 ? "row" : "rows"}`;
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

  const queuedChangeID = actionButton.dataset.auditQueuedChangeId || "";
  if (queuedChangeID) {
    removeQueuedAuditChange(queuedChangeID);
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

  removeQueuedAuditChange(changeID);
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
    const renameIncludeOwnerCheckbox = eventTarget.closest("[data-queue-include-owner]");
    if (renameIncludeOwnerCheckbox instanceof HTMLInputElement) {
      updateAuditQueueBoolean(renameIncludeOwnerCheckbox.dataset.queueIncludeOwner || "", "include_owner", renameIncludeOwnerCheckbox.checked);
      return;
    }

    const renameRequireCleanCheckbox = eventTarget.closest("[data-queue-require-clean]");
    if (renameRequireCleanCheckbox instanceof HTMLInputElement) {
      updateAuditQueueBoolean(renameRequireCleanCheckbox.dataset.queueRequireClean || "", "require_clean", renameRequireCleanCheckbox.checked);
      return;
    }

    const targetProtocolSelect = eventTarget.closest("[data-queue-target-protocol]");
    if (targetProtocolSelect instanceof HTMLSelectElement) {
      updateAuditQueueText(targetProtocolSelect.dataset.queueTargetProtocol || "", "target_protocol", targetProtocolSelect.value);
      return;
    }

    const syncStrategySelect = eventTarget.closest("[data-queue-sync-strategy]");
    if (syncStrategySelect instanceof HTMLSelectElement) {
      updateAuditQueueText(syncStrategySelect.dataset.queueSyncStrategy || "", "sync_strategy", syncStrategySelect.value);
    }
    return;
  }

  updateAuditQueueBoolean(deleteCheckbox.dataset.queueConfirmDelete || "", "confirm_delete", deleteCheckbox.checked);
}

/**
 * @param {string} changeID
 * @param {"confirm_delete" | "include_owner" | "require_clean"} fieldName
 * @param {boolean} fieldValue
 */
function updateAuditQueueBoolean(changeID, fieldName, fieldValue) {
  if (!changeID) {
    return;
  }

  state.auditQueue = state.auditQueue.map((change) => {
    if (change.id !== changeID) {
      return change;
    }
    return {
      ...change,
      [fieldName]: fieldValue,
    };
  });
  renderRunError("");
  renderAuditQueue();
}

/**
 * @param {string} changeID
 * @param {"target_protocol" | "sync_strategy"} fieldName
 * @param {string} fieldValue
 */
function updateAuditQueueText(changeID, fieldName, fieldValue) {
  if (!changeID) {
    return;
  }

  state.auditQueue = state.auditQueue.map((change) => {
    if (change.id !== changeID) {
      return change;
    }
    return {
      ...change,
      [fieldName]: fieldValue,
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

    if (protocolFixAvailable(row)) {
      actions.push({
        kind: auditChangeKindConvertProtocolValue,
        label: "Queue protocol fix",
        title: "Fix protocol",
        description: `Convert origin from ${row.remote_protocol} to a reviewed target protocol.`,
        buildChange: () => ({
          kind: auditChangeKindConvertProtocolValue,
          path: row.path,
          source_protocol: row.remote_protocol,
          target_protocol: defaultTargetProtocol(row.remote_protocol),
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

    if (row.origin_remote_status === "configured" && row.in_sync === "no" && row.remote_default_branch && row.remote_default_branch !== "n/a") {
      actions.push({
        kind: auditChangeKindSyncWithRemoteValue,
        label: "Queue sync",
        title: "Sync with remote",
        description: `Refresh the local repository state against ${row.remote_default_branch} using a reviewed dirty-worktree policy.`,
        buildChange: () => ({
          kind: auditChangeKindSyncWithRemoteValue,
          path: row.path,
          sync_strategy: auditSyncStrategyRequireCleanValue,
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

  return actions.map((action) => {
    const queuedChange = queuedAuditChangeForAction(row.path, action.kind);
    if (!queuedChange) {
      return action;
    }

    return {
      ...action,
      label: dequeueAuditActionLabel(action.label),
      queued: true,
      queuedChangeID: queuedChange.id,
    };
  });
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
  renderAuditQueueState();
}

function clearAuditQueue() {
  state.auditQueue = [];
  renderRunError("");
  renderAuditQueueState();
}

function renderAuditQueueState() {
  renderAuditQueue();
  if (!elements.auditResultsPanel.hidden) {
    renderTypedAuditTable(state.auditInspectionRows);
  }
}

/**
 * @param {string} changeID
 */
function removeQueuedAuditChange(changeID) {
  if (!changeID) {
    return;
  }

  state.auditQueue = state.auditQueue.filter((change) => change.id !== changeID);
  renderRunError("");
  renderAuditQueueState();
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
      appendToken(meta, formatAuditChangeKind(change.kind), "token-default");
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
        changes: sortAuditQueueForApply(state.auditQueue).map((change) => ({
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
      await inspectAuditRequest(
        {
          roots: state.auditInspectionRoots.slice(),
          include_all: state.auditInspectionIncludeAll,
        },
        false,
      );
    }

    const failedResults = results.filter((result) => result.status !== auditChangeStatusSucceededValue);
    if (failedResults.length > 0) {
      renderRunError(failedResults.map(formatAuditApplyIssue).join("\n"));
      setStatus("failed");
    } else {
      setStatus("succeeded");
    }
  } catch (error) {
    renderRunError(String(error));
    setStatus("failed");
  } finally {
    state.auditQueueApplying = false;
    renderAuditQueueState();
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
 * @param {AuditChangeApplyResult} result
 * @returns {string}
 */
function formatAuditApplyIssue(result) {
  if (result.error) {
    return result.error;
  }
  if (result.status === auditChangeStatusSkippedValue) {
    return `${formatAuditChangeKind(result.kind)} skipped for ${result.path}`;
  }
  return `${formatAuditChangeKind(result.kind)} failed for ${result.path}`;
}

/**
 * @param {number} count
 * @returns {string}
 */
function auditQueueSummary(count) {
  return `${count} pending ${count === 1 ? "change" : "changes"}`;
}

/**
 * @param {string} rowPath
 * @param {string} actionKind
 * @returns {AuditQueueEntry | undefined}
 */
function queuedAuditChangeForAction(rowPath, actionKind) {
  return state.auditQueue.find((change) => change.path === rowPath && change.kind === actionKind);
}

/**
 * @param {string} label
 * @returns {string}
 */
function dequeueAuditActionLabel(label) {
  if (label.startsWith("Queue ")) {
    return `Dequeue ${label.slice("Queue ".length)}`;
  }

  return `Dequeue ${label}`;
}

/**
 * @param {string} kind
 * @returns {string}
 */
function formatAuditChangeKind(kind) {
  switch (kind) {
    case auditChangeKindRenameFolderValue:
      return "Rename folder";
    case auditChangeKindConvertProtocolValue:
      return "Fix protocol";
    case auditChangeKindUpdateCanonicalValue:
      return "Fix canonical remote";
    case auditChangeKindSyncWithRemoteValue:
      return "Sync with remote";
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
  switch (change.kind) {
    case auditChangeKindRenameFolderValue:
      return renderRenameQueueOptions(change);
    case auditChangeKindConvertProtocolValue:
      return renderProtocolQueueOptions(change);
    case auditChangeKindSyncWithRemoteValue:
      return renderSyncQueueOptions(change);
    case auditChangeKindDeleteFolderValue:
      return renderDeleteQueueOptions(change);
    default:
      return null;
  }
}

/**
 * @returns {boolean}
 */
function auditQueueCanApply() {
  return state.auditQueue.every((change) => {
    if (change.kind === auditChangeKindDeleteFolderValue) {
      return Boolean(change.confirm_delete);
    }
    if (change.kind === auditChangeKindConvertProtocolValue) {
      const sourceProtocol = String(change.source_protocol || "").trim();
      const targetProtocol = String(change.target_protocol || "").trim();
      return protocolSourceValueAllowed(sourceProtocol) && protocolValueAllowed(targetProtocol) && sourceProtocol !== targetProtocol;
    }
    if (change.kind === auditChangeKindSyncWithRemoteValue) {
      return syncStrategyAllowed(String(change.sync_strategy || ""));
    }
    return true;
  });
}

/**
 * @param {AuditQueueEntry} change
 * @returns {HTMLElement}
 */
function renderDeleteQueueOptions(change) {
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
 * @param {AuditQueueEntry} change
 * @returns {HTMLElement}
 */
function renderRenameQueueOptions(change) {
  const container = document.createElement("div");
  container.className = "audit-queue-options";

  const heading = document.createElement("div");
  heading.className = "audit-queue-options-heading";
  heading.textContent = "Rename options";

  const includeOwnerLabel = document.createElement("label");
  includeOwnerLabel.className = "checkbox-row audit-queue-confirm";

  const includeOwnerCheckbox = document.createElement("input");
  includeOwnerCheckbox.type = "checkbox";
  includeOwnerCheckbox.checked = Boolean(change.include_owner);
  includeOwnerCheckbox.dataset.queueIncludeOwner = change.id;

  const includeOwnerCopy = document.createElement("span");
  includeOwnerCopy.textContent = "Include the owner in the destination folder name";

  includeOwnerLabel.append(includeOwnerCheckbox, includeOwnerCopy);

  const requireCleanLabel = document.createElement("label");
  requireCleanLabel.className = "checkbox-row audit-queue-confirm";

  const requireCleanCheckbox = document.createElement("input");
  requireCleanCheckbox.type = "checkbox";
  requireCleanCheckbox.checked = Boolean(change.require_clean);
  requireCleanCheckbox.dataset.queueRequireClean = change.id;

  const requireCleanCopy = document.createElement("span");
  requireCleanCopy.textContent = "Require a clean worktree before renaming";

  requireCleanLabel.append(requireCleanCheckbox, requireCleanCopy);

  container.append(heading, includeOwnerLabel, requireCleanLabel);
  return container;
}

/**
 * @param {AuditQueueEntry} change
 * @returns {HTMLElement}
 */
function renderProtocolQueueOptions(change) {
  const container = document.createElement("div");
  container.className = "audit-queue-options";

  const heading = document.createElement("div");
  heading.className = "audit-queue-options-heading";
  heading.textContent = "Protocol options";

  const source = document.createElement("p");
  source.className = "audit-queue-option-note";
  source.textContent = `Current protocol: ${protocolDisplayValue(change.source_protocol || "unknown")}`;

  const label = document.createElement("label");
  label.className = "audit-queue-option-row";
  label.textContent = "Target protocol";

  const select = document.createElement("select");
  select.className = "select-input audit-queue-select";
  select.dataset.queueTargetProtocol = change.id;

  protocolOptionValues().forEach((value) => {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = value;
    select.append(option);
  });

  if (change.target_protocol && protocolValueAllowed(change.target_protocol)) {
    select.value = change.target_protocol;
  }

  container.append(heading, source, label, select);
  return container;
}

/**
 * @param {AuditQueueEntry} change
 * @returns {HTMLElement}
 */
function renderSyncQueueOptions(change) {
  const container = document.createElement("div");
  container.className = "audit-queue-options";

  const heading = document.createElement("div");
  heading.className = "audit-queue-options-heading";
  heading.textContent = "Sync options";

  const label = document.createElement("label");
  label.className = "audit-queue-option-row";
  label.textContent = "Dirty-worktree policy";

  const select = document.createElement("select");
  select.className = "select-input audit-queue-select";
  select.dataset.queueSyncStrategy = change.id;

  syncStrategyOptions().forEach((optionValue) => {
    const option = document.createElement("option");
    option.value = optionValue.value;
    option.textContent = optionValue.label;
    select.append(option);
  });

  const syncStrategy = change.sync_strategy || auditSyncStrategyRequireCleanValue;
  if (syncStrategyAllowed(syncStrategy)) {
    select.value = syncStrategy;
  }

  container.append(heading, label, select);
  return container;
}

/**
 * @param {string} protocolValue
 * @returns {boolean}
 */
function protocolFixAvailableRowValue(protocolValue) {
  return protocolSourceValueAllowed(protocolValue);
}

/**
 * @param {AuditInspectionRow} row
 * @returns {boolean}
 */
function protocolFixAvailable(row) {
  return row.origin_remote_status === "configured" && protocolFixAvailableRowValue(row.remote_protocol);
}

/**
 * @param {string} currentProtocol
 * @returns {string}
 */
function defaultTargetProtocol(currentProtocol) {
  return currentProtocol === "https" ? "ssh" : "https";
}

/**
 * @returns {string[]}
 */
function protocolOptionValues() {
  return ["ssh", "https"];
}

/**
 * @param {string} protocolValue
 * @returns {boolean}
 */
function protocolValueAllowed(protocolValue) {
  return protocolOptionValues().includes(protocolValue);
}

/**
 * @param {string} protocolValue
 * @returns {boolean}
 */
function protocolSourceValueAllowed(protocolValue) {
  return protocolValueAllowed(protocolValue) || protocolValue === "git";
}

/**
 * @param {string} protocolValue
 * @returns {string}
 */
function protocolDisplayValue(protocolValue) {
  return protocolValue === "git" ? "ssh" : protocolValue;
}

/**
 * @returns {{ value: string, label: string }[]}
 */
function syncStrategyOptions() {
  return [
    { value: auditSyncStrategyRequireCleanValue, label: "Require clean worktree" },
    { value: auditSyncStrategyStashChangesValue, label: "Stash tracked changes" },
    { value: auditSyncStrategyCommitChangesValue, label: "Commit tracked changes" },
  ];
}

/**
 * @param {string} syncStrategy
 * @returns {boolean}
 */
function syncStrategyAllowed(syncStrategy) {
  return syncStrategyOptions().some((optionValue) => optionValue.value === syncStrategy);
}

/**
 * @param {AuditQueueEntry[]} changes
 * @returns {AuditQueueEntry[]}
 */
function sortAuditQueueForApply(changes) {
  return changes
    .slice()
    .sort((left, right) => auditApplyPriority(left.kind) - auditApplyPriority(right.kind));
}

/**
 * @param {string} kind
 * @returns {number}
 */
function auditApplyPriority(kind) {
  switch (kind) {
    case auditChangeKindUpdateCanonicalValue:
      return 10;
    case auditChangeKindConvertProtocolValue:
      return 20;
    case auditChangeKindSyncWithRemoteValue:
      return 30;
    case auditChangeKindRenameFolderValue:
      return 40;
    case auditChangeKindDeleteFolderValue:
      return 50;
    default:
      return 100;
  }
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
 * @param {string} folderPath
 * @returns {RepositoryDescriptor | null}
 */
function repositoryForFolderPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return null;
  }

  return state.repositories.find((repository) => normalizeRepositoryTreePath(repository.path) === normalizedFolderPath) || null;
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

async function ensureSingleConfiguredRootTreeDataLoaded() {
  const launchRoot = singleConfiguredLaunchRoot();
  if (!launchRoot) {
    return;
  }

  const anchorPath = singleConfiguredRootAnchorPath(launchRoot);
  const selectedRepositoryPath = normalizeRepositoryTreePath(selectedRepository()?.path || "");
  const pathsToLoad = new Set(singleConfiguredRootVisiblePaths(anchorPath));
  collectDirectoryPathChain(anchorPath, anchorPath).forEach((path) => {
    pathsToLoad.add(path);
  });

  const selectedRepositoryParentPath = parentDirectoryPath(selectedRepositoryPath);
  if (selectedRepositoryParentPath && repositoryTreePathWithin(anchorPath, selectedRepositoryParentPath)) {
    collectDirectoryPathChain(anchorPath, selectedRepositoryParentPath).forEach((path) => {
      pathsToLoad.add(path);
    });
  }

  for (const path of pathsToLoad) {
    // Keep the loaded subtree deterministic so sibling folders appear as soon as the user moves up.
    await ensureDirectoryFoldersLoaded(path);
  }
}

/**
 * @param {string} folderPath
 * @returns {Promise<void>}
 */
async function ensureDirectoryFoldersLoaded(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return;
  }

  if (Object.prototype.hasOwnProperty.call(state.directoryFolders, normalizedFolderPath)) {
    return;
  }

  const pendingLoad = pendingDirectoryLoads.get(normalizedFolderPath);
  if (pendingLoad) {
    await pendingLoad;
    return;
  }

  const loadPromise = (async () => {
    const response = await fetch(`${foldersEndpoint}?path=${encodeURIComponent(normalizedFolderPath)}`);
    if (!response.ok) {
      throw new Error(`Failed to browse folder ${normalizedFolderPath}: ${response.status}`);
    }

    /** @type {DirectoryListing} */
    const listing = await response.json();
    if (listing.error) {
      throw new Error(listing.error);
    }

    state.directoryFolders[normalizedFolderPath] = (listing.folders || [])
      .map((folder) => ({
        name: String(folder.name || "").trim(),
        path: normalizeRepositoryTreePath(folder.path),
      }))
      .filter((folder) => folder.name && folder.path);
  })().catch(() => {
    state.directoryFolders[normalizedFolderPath] = [];
  });

  pendingDirectoryLoads.set(normalizedFolderPath, loadPromise);
  try {
    await loadPromise;
  } finally {
    pendingDirectoryLoads.delete(normalizedFolderPath);
  }
}

/**
 * @param {string} basePath
 * @param {string} targetPath
 * @returns {string[]}
 */
function collectDirectoryPathChain(basePath, targetPath) {
  const normalizedBasePath = normalizeRepositoryTreePath(basePath);
  const normalizedTargetPath = normalizeRepositoryTreePath(targetPath);
  if (!normalizedBasePath || !normalizedTargetPath || !repositoryTreePathWithin(normalizedBasePath, normalizedTargetPath)) {
    return [];
  }

  const chain = [normalizedBasePath];
  if (normalizedBasePath === normalizedTargetPath) {
    return chain;
  }

  const relativeSegments = splitRepositoryTreePath(normalizedTargetPath.slice(normalizedBasePath.length + 1));
  let currentPath = normalizedBasePath;
  relativeSegments.forEach((segment) => {
    currentPath = `${currentPath}/${segment}`;
    chain.push(currentPath);
  });

  return chain;
}

/**
 * @param {string} folderPath
 * @returns {FolderDescriptor[]}
 */
function directoryFoldersForPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return [];
  }

  return state.directoryFolders[normalizedFolderPath] || [];
}

/**
 * @param {string} parentPath
 * @param {string} candidatePath
 * @returns {boolean}
 */
function repositoryTreePathWithin(parentPath, candidatePath) {
  const normalizedParentPath = normalizeRepositoryTreePath(parentPath);
  const normalizedCandidatePath = normalizeRepositoryTreePath(candidatePath);
  if (!normalizedParentPath || !normalizedCandidatePath) {
    return false;
  }

  return normalizedCandidatePath === normalizedParentPath || normalizedCandidatePath.startsWith(`${normalizedParentPath}/`);
}

/**
 * @param {string} folderPath
 * @returns {string}
 */
function parentDirectoryPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return "";
  }

  const lastSlashIndex = normalizedFolderPath.lastIndexOf("/");
  if (lastSlashIndex <= 0) {
    return "";
  }

  return normalizedFolderPath.slice(0, lastSlashIndex);
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
function activeRepositoryTreeFolderPath() {
  const selectedFolderPath = normalizeRepositoryTreePath(state.selectedFolderPath || "");
  if (selectedFolderPath) {
    return selectedFolderPath;
  }

  if (!repositoryTreeControl || !state.activeRepositoryTreeKey) {
    return "";
  }

  const activeNode = repositoryTreeControl.findKey(state.activeRepositoryTreeKey);
  if (!activeNode || String(activeNode.data?.kind || "") !== "folder") {
    return "";
  }

  return normalizeRepositoryTreePath(String(activeNode.data?.absolute_path || activeNode.data?.path || ""));
}

/**
 * @returns {string[]}
 */
function workingFolderRoots() {
  const selectedFolderPath = activeRepositoryTreeFolderPath();
  if (selectedFolderPath) {
    return [selectedFolderPath];
  }

  return explorerRootPaths();
}

/**
 * @param {any} nodeData
 * @returns {string}
 */
function repositoryTreeNodeFolderPath(nodeData) {
  return normalizeRepositoryTreePath(String(nodeData?.absolute_path || nodeData?.path || ""));
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
 * @param {RepositoryDescriptor[]} repositories
 * @returns {RepositoryDescriptor[]}
 */
function topLevelRepositories(repositories) {
  /** @type {RepositoryDescriptor[]} */
  const topLevel = [];

  repositories
    .slice()
    .sort((left, right) => {
      const leftPath = normalizeRepositoryTreePath(left.path);
      const rightPath = normalizeRepositoryTreePath(right.path);
      if (leftPath.length !== rightPath.length) {
        return leftPath.length - rightPath.length;
      }
      return compareRepositories(left, right);
    })
    .forEach((repository) => {
      const repositoryPath = normalizeRepositoryTreePath(repository.path);
      if (!repositoryPath) {
        topLevel.push(repository);
        return;
      }
      if (topLevel.some((candidate) => repositoryPathNestedWithinRepository(repositoryPath, candidate.path))) {
        return;
      }
      topLevel.push(repository);
    });

  return topLevel.sort(compareRepositories);
}

/**
 * @param {string} repositoryPath
 * @param {string} ancestorRepositoryPath
 * @returns {boolean}
 */
function repositoryPathNestedWithinRepository(repositoryPath, ancestorRepositoryPath) {
  const normalizedRepositoryPath = normalizeRepositoryTreePath(repositoryPath);
  const normalizedAncestorPath = normalizeRepositoryTreePath(ancestorRepositoryPath);
  if (!normalizedRepositoryPath || !normalizedAncestorPath || normalizedRepositoryPath === normalizedAncestorPath) {
    return false;
  }
  return normalizedRepositoryPath.startsWith(`${normalizedAncestorPath}/`);
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
