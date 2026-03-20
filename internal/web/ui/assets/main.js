// @ts-check

import {
  elements,
  setStatus,
} from "./shared.js";
import {
  handleRepositoryTreeCheckboxClick,
  loadInitialState,
  renderRepositoryTree,
  setRepositoryTreeScopeChangeHandler,
} from "./repo_tree.js";
import {
  applyAuditQueue,
  clearAuditQueue,
  handleAuditQueueListChange,
  handleAuditQueueListClick,
  handleAuditResultsClick,
  handleAuditResultsHeadChange,
  inspectAuditRoots,
  renderAuditQueue,
  renderAuditTaskState,
} from "./audit.js";

export function reportBootstrapFailure(message) {
  const failureMessage = String(message || "").trim();
  if (!failureMessage) {
    return;
  }

  if (elements.runError instanceof HTMLElement) {
    elements.runError.textContent = failureMessage;
  }

  if (elements.runStatus instanceof HTMLElement) {
    elements.runStatus.textContent = "failed";
    elements.runStatus.className = "status-pill status-failed";
  }
}

export async function initializeApp() {
  bindEvents();
  setRepositoryTreeScopeChangeHandler(renderAuditTaskState);
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
