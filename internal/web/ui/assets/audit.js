// @ts-check

import {
  auditChangeKindCommitChangesValue,
  auditApplyEndpoint,
  auditChangeKindConvertProtocolValue,
  auditChangeKindDeleteFolderValue,
  auditChangeKindRenameFolderValue,
  auditChangeKindSyncWithRemoteValue,
  auditChangeKindUpdateChangelogValue,
  auditChangeKindUpdateCanonicalValue,
  auditChangeStatusSkippedValue,
  auditChangeStatusSucceededValue,
  auditColumnLabels,
  auditDirtyFilesPreviewLimit,
  auditInspectEndpoint,
  auditQueueSummary,
  auditSyncStrategyCommitChangesValue,
  auditSyncStrategyRequireCleanValue,
  auditSyncStrategyStashChangesValue,
  elements,
  state,
  appendEmptyState,
  appendToken,
  checkedRepositories,
  clearRunnerOutput,
  renderRunError,
  setStatus,
  summarizeAuditSelectionValues,
  typedAuditHeaderColumns,
} from "./shared.js";
import {
  activeRepositoryTreeFolderPath,
  workingFolderRoots,
} from "./repo_tree.js";

const remoteProtocolSSHValue = "ssh";
const remoteProtocolHTTPSValue = "https";

export function renderAuditTaskState() {
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

  if (checkedRoots.length === 1) {
    elements.auditSelectionBadge.textContent = "Checked repo";
    elements.auditSelectionSummary.textContent = `Audit ${checkedRoots[0]}.${includeAllEnabled ? " Non-Git folders under this root will be included." : " Uncheck the repository to return to folder selection."}`;
    return;
  }

  if (selectedFolderPath) {
    elements.auditSelectionBadge.textContent = "Selected folder";
    elements.auditSelectionSummary.textContent = `Audit ${selectedFolderPath}.${includeAllEnabled ? " Non-Git folders under this root will be included." : " Select another folder when you want to move the audit target."}`;
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

function resolveAuditRoots() {
  return Array.from(new Set(workingFolderRoots()));
}

function currentAuditInspectionRequest() {
  return {
    roots: resolveAuditRoots(),
    include_all: Boolean(elements.auditIncludeAll?.checked),
  };
}

async function fetchAuditInspection(inspectionRequest) {
  const response = await fetch(auditInspectEndpoint, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(inspectionRequest),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
    throw new Error(payload.error || `Failed to inspect audit roots: ${response.status}`);
  }

  const inspection = await response.json();
  if (inspection.error) {
    throw new Error(inspection.error);
  }
  return inspection;
}

function replaceAuditInspectionSnapshot(inspectionRequest, inspection, resetFilters) {
  state.auditInspectionRoots = (inspectionRequest.roots || []).slice();
  state.auditInspectionIncludeAll = Boolean(inspectionRequest.include_all);
  state.auditInspectionRows = (inspection.rows || []).slice();
  if (resetFilters) {
    state.auditColumnFilters = {};
    state.expandedAuditDirtyFilePaths = [];
  } else {
    state.expandedAuditDirtyFilePaths = state.expandedAuditDirtyFilePaths.filter((rowPath) => (
      state.auditInspectionRows.some((row) => row.path === rowPath && auditDirtyFileEntries(row).length > 0)
    ));
  }
}

function reconcileAuditQueueToInspection() {
  const nextQueue = [];
  let removedQueuedActions = 0;

  state.auditQueue.forEach((change) => {
    const row = state.auditInspectionRows.find((candidate) => candidate.path === change.path);
    if (!row) {
      removedQueuedActions += 1;
      return;
    }

    const actionStillAvailable = availableAuditRowActions(row).some((action) => auditQueuedChangeMatchesAction(change, row, action));
    if (!actionStillAvailable) {
      removedQueuedActions += 1;
      return;
    }

    nextQueue.push(change);
  });

  state.auditQueue = nextQueue;
  return { removedQueuedActions };
}

function auditQueuedChangeMatchesAction(change, row, action) {
  if (change.kind !== action.kind) {
    return false;
  }

  if (change.kind === auditChangeKindConvertProtocolValue) {
    const expectedChange = action.buildChange(row);
    return change.source_protocol === expectedChange.source_protocol
      && change.target_protocol === expectedChange.target_protocol;
  }

  return true;
}

async function refreshCurrentAuditInspection(resetFilters) {
  const inspectionRequest = {
    roots: state.auditInspectionRoots.slice(),
    include_all: state.auditInspectionIncludeAll,
  };
  if (inspectionRequest.roots.length === 0) {
    return { removedQueuedActions: 0 };
  }

  const inspection = await fetchAuditInspection(inspectionRequest);
  replaceAuditInspectionSnapshot(inspectionRequest, inspection, resetFilters);
  const refreshResult = reconcileAuditQueueToInspection();
  renderTypedAuditTable(state.auditInspectionRows);
  renderAuditQueue();
  return refreshResult;
}

export async function inspectAuditRoots() {
  const inspectionRequest = currentAuditInspectionRequest();
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
    const inspection = await fetchAuditInspection(inspectionRequest);
    replaceAuditInspectionSnapshot(inspectionRequest, inspection, true);
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
        renderAuditTableCell(cell, row, headerName, value);
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
  elements.auditTableShell?.classList.toggle("audit-table-shell-dirty-expanded", state.expandedAuditDirtyFilePaths.length > 0);
  elements.auditResultsPanel.hidden = false;
}

function hideAuditResults() {
  elements.auditResultsPanel.hidden = true;
  elements.auditResultsSummary.textContent = "";
  elements.auditResultsHead.innerHTML = "";
  elements.auditResultsBody.innerHTML = "";
  state.auditColumnFilters = {};
  state.expandedAuditDirtyFilePaths = [];
  elements.auditTableShell?.classList.remove("audit-table-shell-dirty-expanded");
}

function renderAuditTableCell(cell, row, headerName, value) {
  if (headerName === "dirty_files") {
    renderAuditDirtyFilesCell(cell, row);
    return;
  }

  cell.textContent = value || " ";
}

function renderAuditDirtyFilesCell(cell, row) {
  const dirtyFileEntries = auditDirtyFileEntries(row);
  if (dirtyFileEntries.length === 0) {
    cell.textContent = auditDirtyFilesText(row) || " ";
    return;
  }

  const expanded = state.expandedAuditDirtyFilePaths.includes(row.path);
  const visibleEntries = expanded ? dirtyFileEntries : dirtyFileEntries.slice(0, auditDirtyFilesPreviewLimit);

  const shell = document.createElement("div");
  shell.className = `audit-dirty-files-shell${expanded ? " audit-dirty-files-shell-expanded" : ""}`;

  const nestedTable = document.createElement("table");
  nestedTable.className = "audit-dirty-files-table";

  const nestedHead = document.createElement("thead");
  const headRow = document.createElement("tr");
  const statusHead = document.createElement("th");
  statusHead.scope = "col";
  statusHead.textContent = "Status";
  const fileHead = document.createElement("th");
  fileHead.scope = "col";
  fileHead.textContent = "File";
  headRow.append(statusHead, fileHead);
  nestedHead.append(headRow);

  const nestedBody = document.createElement("tbody");
  visibleEntries.forEach((dirtyFileEntry) => {
    const entryRow = document.createElement("tr");

    const statusCell = document.createElement("td");
    statusCell.className = "audit-dirty-files-status";
    statusCell.textContent = dirtyFileEntry.status;

    const fileCell = document.createElement("td");
    fileCell.className = "audit-dirty-files-path";
    fileCell.textContent = dirtyFileEntry.file;

    entryRow.append(statusCell, fileCell);
    nestedBody.append(entryRow);
  });

  nestedTable.append(nestedHead, nestedBody);
  shell.append(nestedTable);

  if (dirtyFileEntries.length > auditDirtyFilesPreviewLimit) {
    const summary = document.createElement("p");
    summary.className = "audit-dirty-files-summary";
    summary.textContent = expanded
      ? `Showing all ${dirtyFileEntries.length} dirty files.`
      : `Showing ${visibleEntries.length} of ${dirtyFileEntries.length} dirty files.`;
    shell.append(summary);

    const toggleButton = document.createElement("button");
    toggleButton.type = "button";
    toggleButton.className = "secondary-button audit-dirty-files-toggle";
    toggleButton.dataset.auditDirtyToggle = row.path;
    toggleButton.textContent = expanded ? "Show less" : "Show more";
    shell.append(toggleButton);
  }

  cell.append(shell);
}

function availableAuditRowActions(row) {
  const correctiveActions = [];
  const operatorActions = [];
  if (row.is_git_repository) {
    if (row.name_matches === "no") {
      correctiveActions.push({
        kind: auditChangeKindRenameFolderValue,
        label: "Rename folder",
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
      correctiveActions.push({
        kind: auditChangeKindUpdateCanonicalValue,
        label: "Fix canonical remote",
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

    const protocolAction = buildProtocolAuditRowAction(row);
    if (protocolAction) {
      operatorActions.push(protocolAction);
    }

    if (row.origin_remote_status === "configured"
      && row.remote_default_branch
      && row.remote_default_branch !== "n/a"
      && row.in_sync === "no") {
      operatorActions.push({
        kind: auditChangeKindSyncWithRemoteValue,
        label: "Sync with remote",
        title: "Sync with remote",
        description: `Refresh the local repository state against ${row.remote_default_branch} using a reviewed dirty-worktree policy.`,
        buildChange: () => ({
          kind: auditChangeKindSyncWithRemoteValue,
          path: row.path,
          sync_strategy: auditSyncStrategyRequireCleanValue,
        }),
      });
    }

    if (row.worktree_dirty === "yes" && changelogAuditRowActionAvailable(row)) {
      operatorActions.push({
        kind: auditChangeKindUpdateChangelogValue,
        label: "Update changelog",
        title: "Update changelog",
        description: "Generate the next changelog section from recent changes and insert it into CHANGELOG.md.",
        buildChange: () => ({
          kind: auditChangeKindUpdateChangelogValue,
          path: row.path,
        }),
      });
    }

    if (row.worktree_dirty === "yes") {
      operatorActions.push({
        kind: auditChangeKindCommitChangesValue,
        label: "Commit changes",
        title: "Commit changes",
        description: "Generate a commit message from the pending changes and commit the full worktree.",
        buildChange: () => ({
          kind: auditChangeKindCommitChangesValue,
          path: row.path,
        }),
      });
    }
  } else if (row.path) {
    correctiveActions.push({
      kind: auditChangeKindDeleteFolderValue,
      label: "Delete folder",
      title: "Delete folder",
      description: `Delete ${row.path} from disk after explicit confirmation.`,
      buildChange: () => ({
        kind: auditChangeKindDeleteFolderValue,
        path: row.path,
        confirm_delete: false,
      }),
    });
  }

  return correctiveActions.concat(operatorActions);
}

function changelogAuditRowActionAvailable(row) {
  const localBranch = String(row.local_branch || "").trim();
  const defaultBranch = String(row.remote_default_branch || "").trim();
  if (!localBranch) {
    return false;
  }
  if (Boolean(row.head_tagged)) {
    return false;
  }
  if (defaultBranch && defaultBranch !== "n/a") {
    return localBranch === defaultBranch;
  }
  return localBranch === "master";
}

function buildProtocolAuditRowAction(row) {
  if (row.origin_remote_status !== "configured") {
    return null;
  }

  const currentProtocol = normalizedRemoteProtocol(row.remote_protocol);
  if (currentProtocol !== remoteProtocolSSHValue && currentProtocol !== remoteProtocolHTTPSValue) {
    return null;
  }

  const targetProtocol = currentProtocol === remoteProtocolSSHValue
    ? remoteProtocolHTTPSValue
    : remoteProtocolSSHValue;
  const targetProtocolLabel = targetProtocol.toUpperCase();
  const currentProtocolLabel = currentProtocol.toUpperCase();

  return {
    kind: auditChangeKindConvertProtocolValue,
    label: `Switch to ${targetProtocolLabel}`,
    title: `Switch to ${targetProtocolLabel}`,
    description: `Convert origin from ${currentProtocolLabel} to ${targetProtocolLabel}.`,
    buildChange: () => ({
      kind: auditChangeKindConvertProtocolValue,
      path: row.path,
      source_protocol: currentProtocol,
      target_protocol: targetProtocol,
    }),
  };
}

function normalizedRemoteProtocol(rawProtocol) {
  const protocolValue = String(rawProtocol || "").trim().toLowerCase();
  if (protocolValue === "git" || protocolValue === remoteProtocolSSHValue) {
    return remoteProtocolSSHValue;
  }
  if (protocolValue === remoteProtocolHTTPSValue) {
    return remoteProtocolHTTPSValue;
  }
  return "";
}

function buildAuditRowActions(row) {
  return availableAuditRowActions(row).map((action) => {
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

export function handleAuditResultsClick(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLElement)) {
    return;
  }

  const dirtyFilesToggle = eventTarget.closest("[data-audit-dirty-toggle]");
  if (dirtyFilesToggle instanceof HTMLButtonElement) {
    toggleAuditDirtyFilesExpanded(dirtyFilesToggle.dataset.auditDirtyToggle || "");
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

function toggleAuditDirtyFilesExpanded(rowPath) {
  const normalizedRowPath = String(rowPath || "").trim();
  if (!normalizedRowPath) {
    return;
  }

  if (state.expandedAuditDirtyFilePaths.includes(normalizedRowPath)) {
    state.expandedAuditDirtyFilePaths = state.expandedAuditDirtyFilePaths.filter((path) => path !== normalizedRowPath);
  } else {
    state.expandedAuditDirtyFilePaths = state.expandedAuditDirtyFilePaths.concat([normalizedRowPath]);
  }

  renderTypedAuditTable(state.auditInspectionRows);
}

function queueAuditChange(row, action) {
  const nextChangeID = `audit-change-${state.nextAuditChangeSequence}`;
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

export function renderAuditQueue() {
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

export function clearAuditQueue() {
  state.auditQueue = [];
  renderRunError("");
  renderAuditQueueState();
}

export function handleAuditQueueListClick(event) {
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

export function handleAuditQueueListChange(event) {
  const eventTarget = event.target;
  if (!(eventTarget instanceof HTMLElement)) {
    return;
  }

  const confirmDeleteCheckbox = eventTarget.closest("[data-queue-confirm-delete]");
  if (confirmDeleteCheckbox instanceof HTMLInputElement) {
    updateAuditQueueBoolean(confirmDeleteCheckbox.dataset.queueConfirmDelete || "", "confirm_delete", confirmDeleteCheckbox.checked);
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

function removeQueuedAuditChange(changeID) {
  if (!changeID) {
    return;
  }

  state.auditQueue = state.auditQueue.filter((change) => change.id !== changeID);
  renderRunError("");
  renderAuditQueueState();
}

export async function applyAuditQueue() {
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
          confirm_delete: Boolean(change.confirm_delete),
          include_owner: Boolean(change.include_owner),
          require_clean: Boolean(change.require_clean),
          source_protocol: change.source_protocol || "",
          target_protocol: change.target_protocol || "",
          sync_strategy: change.sync_strategy || "",
        })),
      }),
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
      throw new Error(payload.error || `Failed to apply queued audit changes: ${response.status}`);
    }

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
    const messages = [];

    if (state.auditInspectionRoots.length > 0) {
      try {
        const refreshResult = await refreshCurrentAuditInspection(false);
        if (refreshResult.removedQueuedActions > 0) {
          messages.push(`Audit refreshed. Cleared ${refreshResult.removedQueuedActions} queued ${refreshResult.removedQueuedActions === 1 ? "action" : "actions"} that no longer match the latest findings.`);
        } else {
          messages.push("Audit refreshed to match the current workspace state.");
        }
      } catch (refreshError) {
        messages.push(`Audit refresh failed: ${String(refreshError)}`);
      }
    }

    if (failedResults.length > 0) {
      messages.unshift(failedResults.map(formatAuditApplyIssue).join("\n"));
    }

    renderRunError(messages.join("\n"));
    if (failedResults.length > 0 || messages.some((message) => message.startsWith("Audit refresh failed:"))) {
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

function formatAuditApplyIssue(result) {
  if (result.error) {
    return result.error;
  }
  if (result.status === auditChangeStatusSkippedValue) {
    return `${formatAuditChangeKind(result.kind)} skipped for ${result.path}`;
  }
  return `${formatAuditChangeKind(result.kind)} failed for ${result.path}`;
}

function formatAuditChangeKind(kind) {
  switch (kind) {
    case auditChangeKindCommitChangesValue:
      return "Commit changes";
    case auditChangeKindConvertProtocolValue:
      return "Switch remote protocol";
    case auditChangeKindDeleteFolderValue:
      return "Delete folder";
    case auditChangeKindUpdateChangelogValue:
      return "Update changelog";
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

function dequeueAuditActionLabel(label) {
  return `Remove ${label}`;
}

function queuedAuditChangeForAction(rowPath, actionKind) {
  return state.auditQueue.find((change) => change.path === rowPath && change.kind === actionKind);
}

function renderAuditQueueOptions(change) {
  switch (change.kind) {
    case auditChangeKindDeleteFolderValue:
      return renderDeleteQueueOptions(change);
    case auditChangeKindRenameFolderValue:
      return renderRenameQueueOptions(change);
    case auditChangeKindConvertProtocolValue:
      return renderProtocolQueueOptions(change);
    case auditChangeKindSyncWithRemoteValue:
      return renderSyncQueueOptions(change);
    default:
      return null;
  }
}

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

function renderProtocolQueueOptions(change) {
  const container = document.createElement("div");
  container.className = "audit-queue-options";

  const heading = document.createElement("div");
  heading.className = "audit-queue-options-heading";
  heading.textContent = "Protocol change";

  const summary = document.createElement("p");
  summary.className = "audit-queue-description";
  summary.textContent = `${String(change.source_protocol || "").toUpperCase()} -> ${String(change.target_protocol || "").toUpperCase()}`;

  container.append(heading, summary);
  return container;
}

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
    if (change.kind === auditChangeKindDeleteFolderValue) {
      return Boolean(change.confirm_delete);
    }
    if (change.kind === auditChangeKindSyncWithRemoteValue) {
      return syncStrategyAllowed(String(change.sync_strategy || ""));
    }
    return true;
  });
}

function sortAuditQueueForApply(changes) {
  return changes
    .slice()
    .sort((left, right) => auditApplyPriority(left.kind) - auditApplyPriority(right.kind));
}

function auditApplyPriority(kind) {
  switch (kind) {
    case auditChangeKindUpdateCanonicalValue:
      return 10;
    case auditChangeKindConvertProtocolValue:
      return 20;
    case auditChangeKindSyncWithRemoteValue:
      return 30;
    case auditChangeKindUpdateChangelogValue:
      return 35;
    case auditChangeKindCommitChangesValue:
      return 36;
    case auditChangeKindDeleteFolderValue:
      return 50;
    case auditChangeKindRenameFolderValue:
      return 60;
    default:
      return 100;
  }
}

function syncStrategyOptions() {
  return [
    { value: auditSyncStrategyRequireCleanValue, label: "Require clean worktree" },
    { value: auditSyncStrategyStashChangesValue, label: "Stash tracked changes" },
    { value: auditSyncStrategyCommitChangesValue, label: "Commit tracked changes" },
  ];
}

function syncStrategyAllowed(syncStrategy) {
  return syncStrategyOptions().some((optionValue) => optionValue.value === syncStrategy);
}

function typedAuditRecord(row) {
  return typedAuditHeaderColumns.map((headerName) => typedAuditColumnValue(row, headerName));
}

function auditDirtyFileEntries(row) {
  const entries = Array.isArray(row.dirty_file_entries) ? row.dirty_file_entries : [];
  if (entries.length > 0) {
    return entries
      .map((entry) => ({
        status: String(entry?.status || "").trim(),
        file: String(entry?.file || "").trim(),
      }))
      .filter((entry) => entry.status && entry.file);
  }

  return auditDirtyFilesText(row)
    .split(";")
    .map((entry) => parseAuditDirtyFileDisplayEntry(entry))
    .filter(Boolean);
}

function auditDirtyFilesText(row) {
  const dirtyFilesValue = String(row.dirty_files || "").trim();
  if (dirtyFilesValue) {
    return dirtyFilesValue;
  }

  const entries = Array.isArray(row.dirty_file_entries) ? row.dirty_file_entries : [];
  return entries
    .map((entry) => {
      const statusValue = String(entry?.status || "").trim();
      const fileValue = String(entry?.file || "").trim();
      if (!statusValue || !fileValue) {
        return "";
      }
      return `${statusValue} ${fileValue}`;
    })
    .filter(Boolean)
    .join("; ");
}

function parseAuditDirtyFileDisplayEntry(rawEntry) {
  const trimmedEntry = String(rawEntry || "").trim();
  if (!trimmedEntry) {
    return null;
  }

  if (trimmedEntry.length >= 2) {
    const statusValue = trimmedEntry.slice(0, 2).trim();
    const fileValue = trimmedEntry.slice(2).trim();
    if (statusValue && fileValue) {
      return { status: statusValue, file: fileValue };
    }
  }

  const statusValue = trimmedEntry.slice(0, 1).trim();
  const fileValue = trimmedEntry.slice(1).trim();
  if (!statusValue || !fileValue) {
    return null;
  }

  return { status: statusValue, file: fileValue };
}

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

export function handleAuditResultsHeadChange(event) {
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

function auditColumnFilterValues(headerName, rows) {
  return Array.from(new Set(rows.map((row) => typedAuditColumnValue(row, headerName)).filter(Boolean)))
    .sort((left, right) => left.localeCompare(right, undefined, { numeric: true, sensitivity: "base" }));
}

function filterTypedAuditRows(rows) {
  const activeFilters = Object.entries(state.auditColumnFilters).filter(([, filterValue]) => Boolean(filterValue));
  if (activeFilters.length === 0) {
    return rows;
  }
  return rows.filter((row) => activeFilters.every(([headerName, filterValue]) => typedAuditColumnValue(row, headerName) === filterValue));
}

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
      return auditDirtyFilesText(row);
    default:
      return "";
  }
}

function formatAuditResultsSummary(visibleCount, totalCount) {
  if (visibleCount === totalCount) {
    return `${totalCount} ${totalCount === 1 ? "row" : "rows"}`;
  }
  return `${visibleCount} of ${totalCount} ${totalCount === 1 ? "row" : "rows"}`;
}
