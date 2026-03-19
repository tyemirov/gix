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
 *   sync_strategy?: string,
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

const repositoriesEndpoint = "/api/repos";
const foldersEndpoint = "/api/folders";
const auditInspectEndpoint = "/api/audit/inspect";
const auditApplyEndpoint = "/api/audit/apply";
const configuredRootsLaunchMode = "configured_roots";
const auditChangeKindRenameFolderValue = "rename_folder";
const auditChangeKindUpdateCanonicalValue = "update_remote_canonical";
const auditChangeKindSyncWithRemoteValue = "sync_with_remote";
const auditSyncStrategyRequireCleanValue = "require_clean";
const auditSyncStrategyStashChangesValue = "stash_changes";
const auditSyncStrategyCommitChangesValue = "commit_changes";
const auditChangeStatusSucceededValue = "succeeded";
const auditChangeStatusSkippedValue = "skipped";
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

const state = {
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

const elements = {
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

initialize().catch((error) => {
  reportBootstrapFailure(String(error));
});

async function initialize() {
  bindEvents();
  await loadInitialState();
  renderAuditTaskState();
  await renderRepositoryTree("");
  renderAuditQueue();
  setStatus("idle");
}

function bindEvents() {
  elements.repoFilter?.addEventListener("input", () => {
    void renderRepositoryTree((elements.repoFilter?.value || "").trim().toLowerCase());
  });
  elements.repoTree?.addEventListener("click", handleRepositoryTreeCheckboxClick);
  elements.auditIncludeAll?.addEventListener("change", renderAuditTaskState);
  elements.taskInspectLoad?.addEventListener("click", () => {
    void inspectAuditRoots();
  });
  elements.auditResultsHead?.addEventListener("change", handleAuditResultsHeadChange);
  elements.auditResultsBody?.addEventListener("click", handleAuditResultsClick);
  elements.auditQueueList?.addEventListener("click", handleAuditQueueListClick);
  elements.auditQueueList?.addEventListener("change", handleAuditQueueListChange);
  elements.auditQueueClear?.addEventListener("click", clearAuditQueue);
  elements.auditQueueApply?.addEventListener("click", () => {
    void applyAuditQueue();
  });
}

async function loadInitialState() {
  const response = await fetch(repositoriesEndpoint);
  if (!response.ok) {
    throw new Error(`Failed to load repositories: ${response.status}`);
  }

  /** @type {RepositoryCatalog} */
  const repositoryCatalog = await response.json();
  if (repositoryCatalog.error) {
    throw new Error(repositoryCatalog.error);
  }

  state.repositoryCatalog = repositoryCatalog;
  state.repositories = (repositoryCatalog.repositories || [])
    .map((repository) => normalizeDiscoveredRepository(repository))
    .filter(Boolean)
    .sort(compareRepositories);
  elements.repoCount.textContent = String(state.repositories.length);

  const initialRepositoryID = state.repositories.some((repository) => repository.id === repositoryCatalog.selected_repository_id)
    ? repositoryCatalog.selected_repository_id || ""
    : state.repositories[0]?.id || "";
  state.selectedRepositoryID = initialRepositoryID;
  state.selectedFolderPath = preferredInitialRepositoryTreeFolderPath();
  state.activeRepositoryTreeKey = state.selectedFolderPath ? repositoryTreeFolderKey(state.selectedFolderPath) : "";
}

function renderAuditTaskState() {
  const auditScopeRoots = workingFolderRoots();
  elements.auditRootsInput.value = auditScopeRoots.join(", ");
  elements.auditRootsInput.placeholder = auditScopeRoots.length > 0
    ? auditScopeRoots.join(", ")
    : "Select a folder in the tree to define the audit scope.";
  elements.taskInspectLoad.disabled = auditScopeRoots.length === 0;
  renderAuditSelectionSummary();
}

function renderAuditSelectionSummary() {
  const auditRoots = workingFolderRoots();
  const selectedFolderPath = activeRepositoryTreeFolderPath();
  const checkedRoots = checkedRepositories().map((repository) => repository.path);
  const includeAllEnabled = Boolean(elements.auditIncludeAll?.checked);

  if (checkedRoots.length > 1) {
    elements.auditSelectionBadge.textContent = `${checkedRoots.length} checked repos`;
    elements.auditSelectionSummary.textContent = `Audit ${summarizeAuditSelectionValues(checkedRoots)}.${includeAllEnabled ? " Non-Git folders under those roots will be included." : " Uncheck repositories to narrow the next run."}`;
    return;
  }

  if (selectedFolderPath) {
    elements.auditSelectionBadge.textContent = "Selected folder";
    elements.auditSelectionSummary.textContent = `Audit ${selectedFolderPath}.${includeAllEnabled ? " Non-Git folders under this root will be included." : " Select another folder when you want to move the audit target."}`;
    return;
  }

  if (checkedRoots.length === 1) {
    elements.auditSelectionBadge.textContent = "Checked repo";
    elements.auditSelectionSummary.textContent = `Audit ${checkedRoots[0]}.${includeAllEnabled ? " Non-Git folders under this root will be included." : " Check more repositories or click a parent folder to broaden the run."}`;
    return;
  }

  if (auditRoots.length === 0) {
    elements.auditSelectionBadge.textContent = "No target";
    elements.auditSelectionSummary.textContent = "Select a folder in the tree or check repositories to prepare the next audit run.";
    return;
  }

  elements.auditSelectionBadge.textContent = auditRoots.length === 1 ? "1 audit root" : `${auditRoots.length} audit roots`;
  elements.auditSelectionSummary.textContent = `Audit ${summarizeAuditSelectionValues(auditRoots)}.${includeAllEnabled ? " Non-Git folders under those roots will be included." : ""}`;
}

async function inspectAuditRoots() {
  const inspectionRequest = {
    roots: resolveAuditRoots(),
    include_all: Boolean(elements.auditIncludeAll?.checked),
  };
  if ((inspectionRequest.roots || []).length === 0) {
    hideAuditResults();
    renderRunError("Select a folder in the tree to inspect.");
    setStatus("failed");
    return;
  }

  const queuedActionCount = state.auditQueue.length;
  clearRunnerOutput();
  renderRunError("");
  hideAuditResults();
  setStatus("loading");
  elements.taskInspectLoad.disabled = true;

  try {
    const response = await fetch(auditInspectEndpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(inspectionRequest),
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

    state.auditInspectionRoots = (inspectionRequest.roots || []).slice();
    state.auditInspectionIncludeAll = Boolean(inspectionRequest.include_all);
    state.auditInspectionRows = (inspection.rows || []).slice();
    state.auditColumnFilters = {};
    state.auditQueueVisible = true;
    state.auditQueue = [];
    renderTypedAuditTable(state.auditInspectionRows);
    renderAuditQueue();
    if (queuedActionCount > 0) {
      renderRunError(`Audit refreshed. Cleared ${queuedActionCount} queued ${queuedActionCount === 1 ? "action" : "actions"} to match the latest findings.`);
    }
    setStatus("succeeded");
  } catch (error) {
    hideAuditResults();
    renderRunError(String(error));
    setStatus("failed");
  } finally {
    renderAuditTaskState();
  }
}

function renderTypedAuditTable(rows) {
  elements.auditResultsHead.innerHTML = "";
  elements.auditResultsBody.innerHTML = "";

  const headerRow = document.createElement("tr");
  typedAuditHeaderColumns.forEach((headerName) => {
    const headerCell = document.createElement("th");
    headerCell.scope = "col";
    const columnClassName = auditTableColumnClass(headerName);
    if (columnClassName) {
      headerCell.classList.add(columnClassName);
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
      typedAuditRecord(row).forEach((value, valueIndex) => {
        const cell = document.createElement(valueIndex === 0 ? "th" : "td");
        const headerName = typedAuditHeaderColumns[valueIndex] || "";
        const columnClassName = auditTableColumnClass(headerName);
        if (valueIndex === 0) {
          cell.scope = "row";
        }
        if (columnClassName) {
          cell.classList.add(columnClassName);
        }
        cell.textContent = value || " ";
        rowElement.append(cell);
      });

      const actionsCell = document.createElement("td");
      actionsCell.className = "audit-actions-cell";
      const actions = buildAuditRowActions(row);
      if (actions.length === 0) {
        const emptyLabel = document.createElement("span");
        emptyLabel.className = "panel-note";
        emptyLabel.textContent = "No actions";
        actionsCell.append(emptyLabel);
      } else {
        const actionsList = document.createElement("div");
        actionsList.className = "audit-actions-list";
        actions.forEach((action) => {
          const button = document.createElement("button");
          button.type = "button";
          button.className = `secondary-button audit-action-button${action.queued ? " audit-action-button-queued" : ""}`;
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
 * @returns {AuditRowActionDefinition[]}
 */
function buildAuditRowActions(row) {
  if (!row.is_git_repository) {
    return [];
  }

  /** @type {AuditRowActionDefinition[]} */
  const actions = [];
  if (row.name_matches === "no") {
    actions.push({
      kind: auditChangeKindRenameFolderValue,
      label: "Queue rename folder",
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
      label: "Queue fix canonical remote",
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

  if (row.origin_remote_status === "configured"
    && row.in_sync === "no"
    && row.remote_default_branch
    && row.remote_default_branch !== "n/a") {
    actions.push({
      kind: auditChangeKindSyncWithRemoteValue,
      label: "Queue sync with remote",
      title: "Sync with remote",
      description: `Refresh the local repository state against ${row.remote_default_branch} using a reviewed dirty-worktree policy.`,
      buildChange: () => ({
        kind: auditChangeKindSyncWithRemoteValue,
        path: row.path,
        sync_strategy: auditSyncStrategyRequireCleanValue,
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
 * @param {MouseEvent} event
 * @returns {void}
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
 * @param {AuditInspectionRow} row
 * @param {AuditRowActionDefinition} action
 * @returns {void}
 */
function queueAuditChange(row, action) {
  const nextChangeID = `audit-change-${state.nextAuditChangeSequence}`;
  /** @type {AuditQueueEntry} */
  const candidate = {
    id: nextChangeID,
    kind: action.kind,
    path: row.path,
    title: action.title,
    description: action.description,
    ...action.buildChange(row),
  };

  const existingIndex = state.auditQueue.findIndex((change) => change.path === candidate.path && change.kind === candidate.kind);
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

function renderAuditQueueState() {
  renderAuditQueue();
  if (!elements.auditResultsPanel.hidden) {
    renderTypedAuditTable(state.auditInspectionRows);
  }
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
    appendEmptyState(elements.auditQueueList, "No actions queued from the latest audit snapshot.");
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

function clearAuditQueue() {
  state.auditQueue = [];
  renderRunError("");
  renderAuditQueueState();
}

/**
 * @param {MouseEvent} event
 * @returns {void}
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
 * @returns {void}
 */
function handleAuditQueueListChange(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLElement)) {
    return;
  }

  const includeOwnerCheckbox = eventTarget.closest("[data-queue-include-owner]");
  if (includeOwnerCheckbox instanceof HTMLInputElement) {
    updateAuditQueueBoolean(includeOwnerCheckbox.dataset.queueIncludeOwner || "", "include_owner", includeOwnerCheckbox.checked);
    return;
  }

  const requireCleanCheckbox = eventTarget.closest("[data-queue-require-clean]");
  if (requireCleanCheckbox instanceof HTMLInputElement) {
    updateAuditQueueBoolean(requireCleanCheckbox.dataset.queueRequireClean || "", "require_clean", requireCleanCheckbox.checked);
    return;
  }

  const syncStrategySelect = eventTarget.closest("[data-queue-sync-strategy]");
  if (syncStrategySelect instanceof HTMLSelectElement) {
    updateAuditQueueText(syncStrategySelect.dataset.queueSyncStrategy || "", "sync_strategy", syncStrategySelect.value);
  }
}

/**
 * @param {string} changeID
 * @param {"include_owner" | "require_clean"} fieldName
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
 * @param {"sync_strategy"} fieldName
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
 * @param {string} changeID
 * @returns {void}
 */
function removeQueuedAuditChange(changeID) {
  if (!changeID) {
    return;
  }

  state.auditQueue = state.auditQueue.filter((change) => change.id !== changeID);
  renderRunError("");
  renderAuditQueueState();
}

async function applyAuditQueue() {
  if (state.auditQueue.length === 0 || state.auditQueueApplying) {
    return;
  }
  if (!auditQueueCanApply()) {
    renderRunError("Review the queued actions and complete all required options before applying the queue.");
    return;
  }

  state.auditQueueApplying = true;
  renderAuditQueue();
  clearRunnerOutput();
  renderRunError("");
  setStatus("loading");

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
          sync_strategy: change.sync_strategy || "",
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

    const failedResults = results.filter((result) => result.status !== auditChangeStatusSucceededValue);
    if (failedResults.length > 0) {
      renderRunError(`${failedResults.map(formatAuditApplyIssue).join("\n")}\nRun the audit again when you want a refreshed snapshot.`);
      setStatus("failed");
    } else {
      renderRunError("Apply finished. Run the audit again when you want a refreshed snapshot.");
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
    const heading = [formatAuditChangeKind(result.kind), result.path].filter(Boolean).join(" · ");

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
 * @param {string} kind
 * @returns {string}
 */
function formatAuditChangeKind(kind) {
  switch (kind) {
    case auditChangeKindRenameFolderValue:
      return "Rename folder";
    case auditChangeKindUpdateCanonicalValue:
      return "Fix canonical remote";
    case auditChangeKindSyncWithRemoteValue:
      return "Sync with remote";
    default:
      return kind;
  }
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
 * @param {string} rowPath
 * @param {string} actionKind
 * @returns {AuditQueueEntry | undefined}
 */
function queuedAuditChangeForAction(rowPath, actionKind) {
  return state.auditQueue.find((change) => change.path === rowPath && change.kind === actionKind);
}

/**
 * @param {AuditQueueEntry} change
 * @returns {HTMLElement | null}
 */
function renderAuditQueueOptions(change) {
  switch (change.kind) {
    case auditChangeKindRenameFolderValue:
      return renderRenameQueueOptions(change);
    case auditChangeKindSyncWithRemoteValue:
      return renderSyncQueueOptions(change);
    default:
      return null;
  }
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

function auditQueueCanApply() {
  return state.auditQueue.every((change) => {
    if (change.kind === auditChangeKindSyncWithRemoteValue) {
      return syncStrategyAllowed(String(change.sync_strategy || ""));
    }
    return true;
  });
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
    case auditChangeKindSyncWithRemoteValue:
      return 20;
    case auditChangeKindRenameFolderValue:
      return 30;
    default:
      return 100;
  }
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
 * @param {AuditInspectionRow} row
 * @returns {string[]}
 */
function typedAuditRecord(row) {
  return typedAuditHeaderColumns.map((headerName) => typedAuditColumnValue(row, headerName));
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
 * @returns {void}
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
 * @param {string} query
 * @returns {Promise<void>}
 */
async function renderRepositoryTree(query) {
  if (!(elements.repoTree instanceof HTMLElement)) {
    return;
  }

  elements.repoTree.classList.remove("wb-skeleton", "wb-initializing");
  await ensureRepositoryTreeDirectoryDataLoaded();

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
      if (nodeKey) {
        state.activeRepositoryTreeKey = nodeKey;
        if (typeof event.node?.setActive === "function") {
          void event.node.setActive(true, { noEvents: true });
        }
      }

      if (String(event.node?.data?.kind || "") !== "folder") {
        return;
      }

      const folderPath = repositoryTreeNodeFolderPath(event.node?.data);
      const wasActiveNode = Boolean(nodeKey) && nodeKey === previousActiveRepositoryTreeKey;
      const expanded = typeof event.node?.isExpanded === "function"
        ? Boolean(event.node.isExpanded())
        : Boolean(event.node?.expanded);
      const nextExpanded = wasActiveNode ? !expanded : true;
      setRepositoryTreeFolderCollapsed(folderPath, !nextExpanded);
      if (typeof event.node?.setExpanded === "function") {
        void event.node.setExpanded(nextExpanded);
      }
      void selectFolderPath(folderPath);

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
        const folderPath = repositoryTreeNodeFolderPath(event.node?.data);
        state.selectedFolderPath = folderPath;
        const repository = repositoryForFolderPath(folderPath);
        state.selectedRepositoryID = repository?.id || state.selectedRepositoryID;
        renderAuditTaskState();
      }
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

function repositoryTreeExpandedFolderPaths() {
  return repositoryTreeExpandedKeys()
    .map((key) => String(key || ""))
    .filter((key) => key.startsWith("folder:"))
    .map((key) => normalizeRepositoryTreePath(key.slice("folder:".length)))
    .filter(Boolean);
}

/**
 * @param {string} query
 * @returns {void}
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
  return "branch";
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

  for (const preferredKey of preferredKeys) {
    const activeNode = repositoryTreeControl.findKey(preferredKey);
    if (!activeNode) {
      continue;
    }
    state.activeRepositoryTreeKey = preferredKey;
    await activeNode.setActive(true, { noEvents: true });
    return;
  }
}

/**
 * @param {string} folderPath
 * @returns {Promise<void>}
 */
async function selectFolderPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return;
  }

  const currentRootPaths = repositoryTreeRootPaths();
  if (currentRootPaths.length === 1 && currentRootPaths[0] === normalizedFolderPath) {
    const parentPath = parentDirectoryPath(normalizedFolderPath);
    if (parentPath) {
      state.repositoryTreeRootPathsOverride = [parentPath];
    }
  }

  state.selectedFolderPath = normalizedFolderPath;
  state.activeRepositoryTreeKey = repositoryTreeFolderKey(normalizedFolderPath);
  const repository = repositoryForFolderPath(normalizedFolderPath);
  state.selectedRepositoryID = repository?.id || state.selectedRepositoryID;

  await ensureDirectoryFoldersLoaded(normalizedFolderPath);
  await renderRepositoryTree((elements.repoFilter?.value || "").trim().toLowerCase());
  renderAuditTaskState();
}

function buildRepositoryTreeSource(treeModel, expandedKeys) {
  return treeModel.map((node) => ({
    key: node.key,
    title: node.title,
    expanded: repositoryTreeShouldExpandFolder(node, expandedKeys),
    unselectable: true,
    selected: state.checkedRepositoryIDs.includes(node.repository?.id || ""),
    checkbox: Boolean(node.repository),
    configured_root: Boolean(node.configured_root),
    kind: "folder",
    label: node.title,
    path: node.path,
    absolute_path: node.absolute_path || "",
    repository_id: node.repository?.id || "",
    repository_name: node.repository?.name || node.title,
    repository_path: node.repository?.path || node.path,
    search_text: node.search_text,
    children: buildRepositoryTreeSource(node.children, expandedKeys),
  }));
}

function buildRepositoryTreeModel(repositories) {
  return buildFolderExplorerTreeModel(repositories, repositoryTreeRootPaths());
}

function defaultRepositoryTreeRootPaths() {
  const launchRoots = configuredLaunchRoots();
  if (launchRoots.length > 1) {
    const commonRootPath = normalizeRepositoryTreePath(state.repositoryCatalog?.explorer_root || state.repositoryCatalog?.launch_path || "");
    return commonRootPath ? [commonRootPath] : launchRoots;
  }
  if (launchRoots.length === 1) {
    const parentRootPath = parentDirectoryPath(launchRoots[0]);
    return parentRootPath ? [parentRootPath] : launchRoots;
  }
  const explorerRoot = normalizeRepositoryTreePath(state.repositoryCatalog?.explorer_root || state.repositoryCatalog?.launch_path || "");
  return explorerRoot ? [explorerRoot] : [];
}

function configuredLaunchRoots() {
  return (Array.isArray(state.repositoryCatalog?.launch_roots) ? state.repositoryCatalog.launch_roots : [])
    .map((rootPath) => normalizeRepositoryTreePath(rootPath))
    .filter(Boolean);
}

function explorerRootPaths() {
  const launchRoots = configuredLaunchRoots();
  if (launchRoots.length > 0) {
    return launchRoots;
  }
  const explorerRoot = normalizeRepositoryTreePath(state.repositoryCatalog?.explorer_root || state.repositoryCatalog?.launch_path || "");
  return explorerRoot ? [explorerRoot] : [];
}

function repositoryTreeRootPaths() {
  const overridePaths = (Array.isArray(state.repositoryTreeRootPathsOverride) ? state.repositoryTreeRootPathsOverride : [])
    .map((rootPath) => normalizeRepositoryTreePath(rootPath))
    .filter(Boolean);
  if (overridePaths.length > 0) {
    return overridePaths;
  }
  return defaultRepositoryTreeRootPaths();
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
    const rootNode = ensureFolderExplorerRootNode(treeModel, nodeIndex, rootPath);
    populateLoadedFolderExplorerChildren(rootNode, nodeIndex);
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
 * @param {RepoTreeNodeModel} parentNode
 * @param {Map<string, RepoTreeNodeModel>} nodeIndex
 * @returns {void}
 */
function populateLoadedFolderExplorerChildren(parentNode, nodeIndex) {
  directoryFoldersForPath(parentNode.absolute_path || parentNode.path).forEach((folder) => {
    const childNode = ensureFolderExplorerChildNode(parentNode, nodeIndex, folder.path);
    populateLoadedFolderExplorerChildren(childNode, nodeIndex);
  });
}

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

function attachRepositoryTreeMetadata(node, repository) {
  node.repository = repository;
  node.search_text = `${node.title} ${repositorySearchText(repository)}`;
}

function collapsedRepositoryTreeFolderPaths() {
  return new Set((state.collapsedFolderPaths || [])
    .map((folderPath) => normalizeRepositoryTreePath(folderPath))
    .filter(Boolean));
}

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

function configuredLaunchRootForRepository(repositoryPath, launchRoots) {
  const matchingRoots = launchRoots.filter((launchRoot) => repositoryPath === launchRoot || repositoryPath.startsWith(`${launchRoot}/`));
  matchingRoots.sort((left, right) => right.length - left.length);
  return matchingRoots[0] || "";
}

function configuredLaunchRootLabel(launchRoot) {
  const rootSegments = splitRepositoryTreePath(launchRoot);
  if (rootSegments.length > 0) {
    return rootSegments[rootSegments.length - 1];
  }
  return launchRoot || ".";
}

function repositoryVisibleInTree(repository) {
  return Boolean(repository);
}

function sortRepositoryTreeNodes(nodes) {
  nodes.sort((left, right) => left.title.localeCompare(right.title, undefined, { numeric: true, sensitivity: "base" }));
  nodes.forEach((node) => {
    if (node.children.length > 0) {
      sortRepositoryTreeNodes(node.children);
    }
  });
}

function repositoryTreeFolderKey(folderPath) {
  return `folder:${normalizeRepositoryTreePath(folderPath)}`;
}

function repositorySearchText(repository) {
  return [repository.name, repository.path, repository.current_branch || "", repository.default_branch || ""]
    .join(" ")
    .toLowerCase();
}

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
  if (repositoryTreeRootPaths().includes(folderPath) || configuredLaunchRoots().includes(folderPath)) {
    return true;
  }
  const selectedFolderPath = normalizeRepositoryTreePath(state.selectedFolderPath || "");
  return Boolean(selectedFolderPath && repositoryTreePathWithin(folderPath, selectedFolderPath));
}

/**
 * @param {{ node: any, nodeElem: HTMLElement }} event
 * @returns {void}
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
    rowElement.dataset.repoTreeRepositoryId = String(event.node.data?.repository_id || "");
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
 * @returns {void}
 */
function handleRepositoryTreeCheckboxClick(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLElement)) {
    return;
  }

  const checkboxElement = eventTarget.closest(".wb-checkbox");
  if (!(checkboxElement instanceof HTMLElement)) {
    return;
  }

  const rowElement = checkboxElement.closest(".wb-row");
  if (!(rowElement instanceof HTMLElement)) {
    return;
  }

  const repositoryID = String(rowElement.dataset.repoTreeRepositoryId || "").trim();
  if (!repositoryID) {
    return;
  }

  event.preventDefault();
  event.stopPropagation();
  toggleCheckedRepository(repositoryID, !state.checkedRepositoryIDs.includes(repositoryID));
}

function toggleCheckedRepository(repositoryID, checked) {
  const checkedRepositoryIDs = new Set(state.checkedRepositoryIDs);
  if (checked) {
    checkedRepositoryIDs.add(repositoryID);
  } else {
    checkedRepositoryIDs.delete(repositoryID);
  }
  state.checkedRepositoryIDs = Array.from(checkedRepositoryIDs);

  void renderRepositoryTree((elements.repoFilter?.value || "").trim().toLowerCase());
  renderAuditTaskState();
}

function preferredInitialRepositoryTreeFolderPath() {
  const launchRoots = configuredLaunchRoots();
  if (launchRoots.length > 0) {
    return launchRoots[0];
  }

  const selectedRepository = findRepository(state.repositoryCatalog?.selected_repository_id || "");
  if (selectedRepository) {
    return selectedRepository.path;
  }

  return explorerRootPaths()[0] || "";
}

function repositoryTreeNodeFolderPath(nodeData) {
  return normalizeRepositoryTreePath(String(nodeData?.absolute_path || nodeData?.path || ""));
}

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

function resolveAuditRoots() {
  return Array.from(new Set(workingFolderRoots()));
}

function workingFolderRoots() {
  const checkedRoots = checkedRepositories().map((repository) => repository.path);
  if (checkedRoots.length > 1) {
    return checkedRoots;
  }

  const selectedFolderPath = activeRepositoryTreeFolderPath();
  if (selectedFolderPath) {
    return [selectedFolderPath];
  }

  if (checkedRoots.length === 1) {
    return checkedRoots;
  }

  return explorerRootPaths();
}

async function ensureRepositoryTreeDirectoryDataLoaded() {
  const rootPaths = repositoryTreeRootPaths();
  if (rootPaths.length === 0) {
    return;
  }

  const pathsToLoad = new Set(rootPaths);
  const selectedFolderPath = normalizeRepositoryTreePath(state.selectedFolderPath || "");
  if (selectedFolderPath) {
    rootPaths.forEach((rootPath) => {
      collectDirectoryPathChain(rootPath, selectedFolderPath).forEach((path) => {
        pathsToLoad.add(path);
      });
    });
  }

  repositoryTreeExpandedFolderPaths().forEach((folderPath) => {
    rootPaths.forEach((rootPath) => {
      collectDirectoryPathChain(rootPath, folderPath).forEach((path) => {
        pathsToLoad.add(path);
      });
    });
  });

  for (const path of pathsToLoad) {
    await ensureDirectoryFoldersLoaded(path);
  }
}

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

    const discoveredRepositories = (listing.folders || [])
      .map((folder) => normalizeDiscoveredRepository(folder.repository))
      .filter(Boolean);
    mergeKnownRepositories(discoveredRepositories);

    state.directoryFolders[normalizedFolderPath] = (listing.folders || [])
      .map((folder) => ({
        name: String(folder.name || "").trim(),
        path: normalizeRepositoryTreePath(folder.path),
        repository: normalizeDiscoveredRepository(folder.repository),
      }))
      .filter((folder) => folder.name && folder.path)
      .map((folder) => ({
        ...folder,
        repository: folder.repository ? (repositoryForFolderPath(folder.repository.path) || folder.repository) : undefined,
      }));
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

function directoryFoldersForPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return [];
  }
  return state.directoryFolders[normalizedFolderPath] || [];
}

function repositoryTreePathWithin(parentPath, candidatePath) {
  const normalizedParentPath = normalizeRepositoryTreePath(parentPath);
  const normalizedCandidatePath = normalizeRepositoryTreePath(candidatePath);
  if (!normalizedParentPath || !normalizedCandidatePath) {
    return false;
  }
  return normalizedCandidatePath === normalizedParentPath || normalizedCandidatePath.startsWith(`${normalizedParentPath}/`);
}

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
 * @param {any} repository
 * @returns {RepositoryDescriptor | null}
 */
function normalizeDiscoveredRepository(repository) {
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

/**
 * @param {RepositoryDescriptor[]} repositories
 * @returns {void}
 */
function mergeKnownRepositories(repositories) {
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

function checkedRepositories() {
  return state.repositories.filter((repository) => state.checkedRepositoryIDs.includes(repository.id));
}

function selectedRepository() {
  return findRepository(state.selectedRepositoryID) || state.repositories[0] || null;
}

function findRepository(repositoryID) {
  return state.repositories.find((repository) => repository.id === repositoryID);
}

function repositoryForFolderPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return null;
  }
  return state.repositories.find((repository) => normalizeRepositoryTreePath(repository.path) === normalizedFolderPath) || null;
}

function compareRepositories(left, right) {
  if (left.context_current !== right.context_current) {
    return left.context_current ? -1 : 1;
  }
  return left.name.localeCompare(right.name) || left.path.localeCompare(right.path);
}

function splitRepositoryTreePath(rawPath) {
  return normalizeRepositoryTreePath(rawPath)
    .split("/")
    .map((segment) => segment.trim())
    .filter(Boolean);
}

function normalizeRepositoryTreePath(rawPath) {
  return String(rawPath || "")
    .replace(/\\/g, "/")
    .replace(/\/+/g, "/")
    .replace(/\/$/, "");
}

function appendEmptyState(container, text) {
  const emptyState = document.createElement("div");
  emptyState.className = "empty-state";
  emptyState.textContent = text;
  container.append(emptyState);
}

function appendToken(container, text, className) {
  const token = document.createElement("span");
  token.className = `context-token ${className}`;
  token.textContent = text;
  container.append(token);
}

/**
 * @param {number} count
 * @returns {string}
 */
function auditQueueSummary(count) {
  return `${count} queued ${count === 1 ? "action" : "actions"}`;
}

function summarizeAuditSelectionValues(values) {
  const filteredValues = values
    .map((value) => String(value || "").trim())
    .filter(Boolean);
  if (filteredValues.length <= 3) {
    return filteredValues.join(", ");
  }
  return `${filteredValues.slice(0, 3).join(", ")}, and ${filteredValues.length - 3} more`;
}

function clearRunnerOutput() {
  elements.stdoutOutput.textContent = "";
  elements.stderrOutput.textContent = "";
}

/**
 * @param {string} value
 * @returns {void}
 */
function renderRunError(value) {
  elements.runError.textContent = value;
}

/**
 * @param {string} status
 * @returns {void}
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
