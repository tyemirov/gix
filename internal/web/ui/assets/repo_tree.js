// @ts-check

import { Wunderbaum } from "https://cdn.jsdelivr.net/npm/wunderbaum@0/+esm";

import {
  currentRepositoryLaunchMode,
  elements,
  foldersEndpoint,
  repositoryTreeIconMap,
  repositoriesEndpoint,
  state,
  appendEmptyState,
  checkedRepositories,
  compareRepositories,
  findRepository,
  mergeKnownRepositories,
  normalizeDiscoveredRepository,
  normalizeRepositoryTreePath,
  parentDirectoryPath,
  repositoryForFolderPath,
  repositoryTreePathWithin,
  selectedRepository,
  splitRepositoryTreePath,
} from "./shared.js";

/** @type {any | null} */
let repositoryTreeControl = null;
const pendingDirectoryLoads = new Map();
let repositoryTreeScopeChangeHandler = () => {};
let repositoryTreeSelectionSequence = 0;

export function setRepositoryTreeScopeChangeHandler(handler) {
  repositoryTreeScopeChangeHandler = typeof handler === "function" ? handler : () => {};
}

export async function loadInitialState() {
  const response = await fetch(repositoriesEndpoint);
  if (!response.ok) {
    throw new Error(`Failed to load repositories: ${response.status}`);
  }

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

export async function renderRepositoryTree(query, options = {}) {
  if (!(elements.repoTree instanceof HTMLElement)) {
    return;
  }

  elements.repoTree.classList.remove("wb-skeleton", "wb-initializing");
  if (options.awaitDirectoryData !== false) {
    await ensureRepositoryTreeDirectoryDataLoaded();
  }

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

export function handleRepositoryTreeCheckboxClick(event) {
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

export function configuredLaunchRoots() {
  return (Array.isArray(state.repositoryCatalog?.launch_roots) ? state.repositoryCatalog.launch_roots : [])
    .map((rootPath) => normalizeRepositoryTreePath(rootPath))
    .filter(Boolean);
}

export function explorerRootPaths() {
  const launchRoots = configuredLaunchRoots();
  if (launchRoots.length > 0) {
    return launchRoots;
  }
  const explorerRoot = normalizeRepositoryTreePath(state.repositoryCatalog?.explorer_root || state.repositoryCatalog?.launch_path || "");
  return explorerRoot ? [explorerRoot] : [];
}

export function activeRepositoryTreeFolderPath() {
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

export function workingFolderRoots() {
  const checkedRoots = checkedRepositories().map((repository) => repository.path);
  if (checkedRoots.length > 0) {
    return checkedRoots;
  }

  const selectedFolderPath = activeRepositoryTreeFolderPath();
  if (selectedFolderPath) {
    return [selectedFolderPath];
  }

  return explorerRootPaths();
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
        repositoryTreeScopeChangeHandler();
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

async function selectFolderPath(folderPath) {
  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return;
  }
  const selectionSequence = ++repositoryTreeSelectionSequence;

  const currentRootPaths = repositoryTreeRootPaths();
  if (configuredLaunchRoots().length === 0
    && currentRootPaths.length === 1
    && currentRootPaths[0] === normalizedFolderPath) {
    const parentPath = parentDirectoryPath(normalizedFolderPath);
    if (parentPath) {
      state.repositoryTreeRootPathsOverride = [parentPath];
    }
  }

  state.selectedFolderPath = normalizedFolderPath;
  state.activeRepositoryTreeKey = repositoryTreeFolderKey(normalizedFolderPath);
  const repository = repositoryForFolderPath(normalizedFolderPath);
  state.selectedRepositoryID = repository?.id || state.selectedRepositoryID;

  repositoryTreeScopeChangeHandler();
  await renderRepositoryTree(currentRepositoryTreeQuery(), { awaitDirectoryData: false });
  await ensureRepositoryTreeDirectoryDataLoaded();
  if (selectionSequence !== repositoryTreeSelectionSequence) {
    return;
  }
  await renderRepositoryTree(currentRepositoryTreeQuery(), { awaitDirectoryData: false });
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
    return launchRoots;
  }

  if (launchRoots.length === 1) {
    const parentRootPath = parentDirectoryPath(launchRoots[0]);
    return parentRootPath ? [parentRootPath] : launchRoots;
  }

  if (String(state.repositoryCatalog?.launch_mode || "") === currentRepositoryLaunchMode) {
    const currentRepository = selectedRepository();
    const currentRepositoryPath = normalizeRepositoryTreePath(currentRepository?.path || "");
    if (currentRepositoryPath) {
      const parentRootPath = parentDirectoryPath(currentRepositoryPath);
      return parentRootPath ? [parentRootPath] : [currentRepositoryPath];
    }
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

function buildFolderExplorerTreeModel(repositories, rootPaths) {
  const normalizedRootPaths = rootPaths
    .map((rootPath) => normalizeRepositoryTreePath(rootPath))
    .filter(Boolean);
  if (normalizedRootPaths.length === 0) {
    return [];
  }

  const nodeIndex = new Map();
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

function populateLoadedFolderExplorerChildren(parentNode, nodeIndex) {
  directoryFoldersForPath(parentNode.absolute_path || parentNode.path).forEach((folder) => {
    if (!repositoryTreePathRelevantToConfiguredRoots(folder.path)) {
      return;
    }
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
  return Boolean(repository) && repositoryTreePathRelevantToConfiguredRoots(repository.path);
}

function repositoryTreePathRelevantToConfiguredRoots(folderPath) {
  const launchRoots = configuredLaunchRoots();
  if (launchRoots.length === 0) {
    return true;
  }

  const normalizedFolderPath = normalizeRepositoryTreePath(folderPath);
  if (!normalizedFolderPath) {
    return false;
  }

  return launchRoots.some((launchRoot) => (
    repositoryTreePathWithin(normalizedFolderPath, launchRoot)
    || repositoryTreePathWithin(launchRoot, normalizedFolderPath)
  ));
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

function toggleCheckedRepository(repositoryID, checked) {
  const checkedRepositoryIDs = new Set(state.checkedRepositoryIDs);
  if (checked) {
    checkedRepositoryIDs.add(repositoryID);
  } else {
    checkedRepositoryIDs.delete(repositoryID);
  }
  state.checkedRepositoryIDs = Array.from(checkedRepositoryIDs);

  void renderRepositoryTree((elements.repoFilter?.value || "").trim().toLowerCase());
  repositoryTreeScopeChangeHandler();
}

function preferredInitialRepositoryTreeFolderPath() {
  const launchRoots = configuredLaunchRoots();
  if (launchRoots.length > 0) {
    return launchRoots[0];
  }

  const selectedRepositoryDescriptor = findRepository(state.repositoryCatalog?.selected_repository_id || "");
  if (selectedRepositoryDescriptor) {
    return selectedRepositoryDescriptor.path;
  }

  return explorerRootPaths()[0] || "";
}

function repositoryTreeNodeFolderPath(nodeData) {
  return normalizeRepositoryTreePath(String(nodeData?.absolute_path || nodeData?.path || ""));
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

  await Promise.all(Array.from(pathsToLoad, (path) => ensureDirectoryFoldersLoaded(path)));
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

function currentRepositoryTreeQuery() {
  return (elements.repoFilter?.value || "").trim().toLowerCase();
}
