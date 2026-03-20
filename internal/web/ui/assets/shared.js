// @ts-check

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
 *   name: string,
 *   path: string,
 *   repository?: RepositoryDescriptor,
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
 *   include_all?: boolean,
 * }} AuditInspectionRequest
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
 *   status: string,
 *   file: string,
 * }} AuditDirtyFileEntry
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
 *   head_tagged: boolean,
 *   in_sync: string,
 *   remote_protocol: string,
 *   origin_matches_canonical: string,
 *   worktree_dirty: string,
 *   dirty_files: string,
 *   dirty_file_entries?: AuditDirtyFileEntry[],
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
 *   key: string,
 *   title: string,
 *   path: string,
 *   absolute_path?: string,
 *   configured_root?: boolean,
 *   kind: "folder",
 *   search_text: string,
 *   repository?: RepositoryDescriptor,
 *   children: RepoTreeNodeModel[],
 * }} RepoTreeNodeModel
 */

export const repositoriesEndpoint = "/api/repos";
export const foldersEndpoint = "/api/folders";
export const auditInspectEndpoint = "/api/audit/inspect";
export const auditApplyEndpoint = "/api/audit/apply";
export const currentRepositoryLaunchMode = "current_repo";
export const configuredRootsLaunchMode = "configured_roots";
export const auditChangeKindRenameFolderValue = "rename_folder";
export const auditChangeKindUpdateCanonicalValue = "update_remote_canonical";
export const auditChangeKindConvertProtocolValue = "convert_protocol";
export const auditChangeKindSyncWithRemoteValue = "sync_with_remote";
export const auditChangeKindUpdateChangelogValue = "update_changelog";
export const auditChangeKindCommitChangesValue = "commit_changes";
export const auditChangeKindDeleteFolderValue = "delete_folder";
export const auditSyncStrategyRequireCleanValue = "require_clean";
export const auditSyncStrategyStashChangesValue = "stash_changes";
export const auditSyncStrategyCommitChangesValue = "commit_changes";
export const auditChangeStatusSucceededValue = "succeeded";
export const auditChangeStatusSkippedValue = "skipped";
export const auditDirtyFilesPreviewLimit = 3;
export const typedAuditHeaderColumns = [
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

export const auditColumnLabels = Object.freeze({
  path: "Repo Path",
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

export const repositoryTreeIconMap = Object.freeze({
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

export const state = {
  /** @type {RepositoryCatalog | null} */
  repositoryCatalog: null,
  /** @type {RepositoryDescriptor[]} */
  repositories: [],
  /** @type {Record<string, FolderDescriptor[]>} */
  directoryFolders: {},
  /** @type {string[]} */
  checkedRepositoryIDs: [],
  /** @type {string} */
  selectedRepositoryID: "",
  /** @type {string} */
  activeRepositoryTreeKey: "",
  /** @type {string} */
  selectedFolderPath: "",
  /** @type {AuditInspectionRow[]} */
  auditInspectionRows: [],
  /** @type {string[]} */
  auditInspectionRoots: [],
  /** @type {boolean} */
  auditInspectionIncludeAll: false,
  /** @type {Record<string, string>} */
  auditColumnFilters: {},
  /** @type {string[]} */
  expandedAuditDirtyFilePaths: [],
  /** @type {AuditQueueEntry[]} */
  auditQueue: [],
  /** @type {boolean} */
  auditQueueVisible: false,
  /** @type {boolean} */
  auditQueueApplying: false,
  /** @type {number} */
  nextAuditChangeSequence: 1,
  /** @type {string[]} */
  collapsedFolderPaths: [],
  /** @type {string[]} */
  repositoryTreeRootPathsOverride: [],
};

export const elements = {
  repoCount: document.querySelector("#repo-count"),
  repoFilter: document.querySelector("#repo-filter"),
  repoTree: document.querySelector("#repo-tree"),
  auditSelectionBadge: document.querySelector("#audit-selection-badge"),
  auditSelectionSummary: document.querySelector("#audit-selection-summary"),
  auditRootsInput: document.querySelector("#audit-roots-input"),
  auditIncludeAll: document.querySelector("#audit-include-all"),
  taskInspectLoad: document.querySelector("#task-inspect-load"),
  runError: document.querySelector("#run-error"),
  auditResultsPanel: document.querySelector("#audit-results-panel"),
  auditResultsSummary: document.querySelector("#audit-results-summary"),
  auditTableShell: document.querySelector(".audit-table-shell"),
  auditResultsHead: document.querySelector("#audit-results-head"),
  auditResultsBody: document.querySelector("#audit-results-body"),
  auditQueuePanel: document.querySelector("#audit-queue-panel"),
  auditQueueSummary: document.querySelector("#audit-queue-summary"),
  auditQueueList: document.querySelector("#audit-queue-list"),
  auditQueueClear: document.querySelector("#audit-queue-clear"),
  auditQueueApply: document.querySelector("#audit-queue-apply"),
  runStatus: document.querySelector("#run-status"),
  stdoutOutput: document.querySelector("#stdout-output"),
  stderrOutput: document.querySelector("#stderr-output"),
};

export function normalizeDiscoveredRepository(repository) {
  if (!repository || typeof repository !== "object") {
    return null;
  }

  const repositoryPath = normalizeRepositoryTreePath(String(repository.path || ""));
  if (!repositoryPath) {
    return null;
  }

  const pathSegments = splitRepositoryTreePath(repositoryPath);
  const fallbackName = pathSegments[pathSegments.length - 1] || repositoryPath;
  return {
    id: String(repository.id || repositoryPath).trim(),
    name: String(repository.name || "").trim() || fallbackName,
    path: repositoryPath,
    current_branch: String(repository.current_branch || "").trim(),
    default_branch: String(repository.default_branch || "").trim(),
    dirty: Boolean(repository.dirty),
    context_current: Boolean(repository.context_current),
    error: String(repository.error || "").trim(),
  };
}

export function mergeKnownRepositories(repositories) {
  if (!Array.isArray(repositories) || repositories.length === 0) {
    return;
  }

  const repositoriesByPath = new Map(
    state.repositories.map((repository, repositoryIndex) => [normalizeRepositoryTreePath(repository.path), repositoryIndex]),
  );

  repositories.forEach((repository) => {
    const normalizedRepositoryPath = normalizeRepositoryTreePath(repository.path);
    if (!normalizedRepositoryPath) {
      return;
    }

    const existingRepositoryIndex = repositoriesByPath.get(normalizedRepositoryPath);
    if (typeof existingRepositoryIndex === "number") {
      const existingRepository = state.repositories[existingRepositoryIndex];
      state.repositories[existingRepositoryIndex] = {
        ...existingRepository,
        ...repository,
        id: existingRepository.id || repository.id,
        path: normalizedRepositoryPath,
        context_current: Boolean(existingRepository.context_current || repository.context_current),
      };
      return;
    }

    repositoriesByPath.set(normalizedRepositoryPath, state.repositories.length);
    state.repositories.push({
      ...repository,
      path: normalizedRepositoryPath,
    });
  });

  state.repositories = state.repositories.slice().sort(compareRepositories);
  elements.repoCount.textContent = String(state.repositories.length);
}

export function checkedRepositories() {
  return state.repositories.filter((repository) => state.checkedRepositoryIDs.includes(repository.id));
}

export function selectedRepository() {
  return findRepository(state.selectedRepositoryID) || state.repositories[0] || null;
}

export function findRepository(repositoryID) {
  return state.repositories.find((repository) => repository.id === repositoryID);
}

export function repositoryForFolderPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return null;
  }
  return state.repositories.find((repository) => normalizeRepositoryTreePath(repository.path) === normalizedFolderPath) || null;
}

export function compareRepositories(left, right) {
  if (left.context_current !== right.context_current) {
    return left.context_current ? -1 : 1;
  }
  return left.name.localeCompare(right.name) || left.path.localeCompare(right.path);
}

export function splitRepositoryTreePath(rawPath) {
  return normalizeRepositoryTreePath(rawPath)
    .split("/")
    .map((segment) => segment.trim())
    .filter(Boolean);
}

export function normalizeRepositoryTreePath(rawPath) {
  return String(rawPath || "")
    .replace(/\\/g, "/")
    .replace(/\/+/g, "/")
    .replace(/\/$/, "");
}

export function repositoryTreePathWithin(parentPath, candidatePath) {
  const normalizedParentPath = normalizeRepositoryTreePath(parentPath);
  const normalizedCandidatePath = normalizeRepositoryTreePath(candidatePath);
  if (!normalizedParentPath || !normalizedCandidatePath) {
    return false;
  }
  return normalizedCandidatePath === normalizedParentPath || normalizedCandidatePath.startsWith(`${normalizedParentPath}/`);
}

export function parentDirectoryPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return "";
  }

  const lastSlashIndex = normalizedFolderPath.lastIndexOf("/");
  if (lastSlashIndex < 0) {
    return "";
  }
  if (lastSlashIndex === 0) {
    return normalizedFolderPath === "/" ? "" : "/";
  }
  return normalizedFolderPath.slice(0, lastSlashIndex);
}

export function appendEmptyState(container, text) {
  const emptyState = document.createElement("div");
  emptyState.className = "empty-state";
  emptyState.textContent = text;
  container.append(emptyState);
}

export function appendToken(container, text, className) {
  const token = document.createElement("span");
  token.className = `context-token ${className}`;
  token.textContent = text;
  container.append(token);
}

export function auditQueueSummary(count) {
  return `${count} queued ${count === 1 ? "action" : "actions"}`;
}

export function summarizeAuditSelectionValues(values) {
  const filteredValues = values
    .map((value) => String(value || "").trim())
    .filter(Boolean);
  if (filteredValues.length <= 3) {
    return filteredValues.join(", ");
  }
  return `${filteredValues.slice(0, 3).join(", ")}, and ${filteredValues.length - 3} more`;
}

export function clearRunnerOutput() {
  elements.stdoutOutput.textContent = "";
  elements.stderrOutput.textContent = "";
}

export function renderRunError(value) {
  elements.runError.textContent = value;
}

export function setStatus(status) {
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
