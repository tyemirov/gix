// @ts-check

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

const commandsEndpoint = "/api/commands";
const runsEndpoint = "/api/runs";
const pollIntervalMilliseconds = 800;

/** @type {{ catalog: CommandCatalog | null, commands: CommandDescriptor[], selectedPath: string, pollTimer: number | null }} */
const state = {
  catalog: null,
  commands: [],
  selectedPath: "",
  pollTimer: null,
};

const elements = {
  commandCount: document.querySelector("#command-count"),
  commandFilter: document.querySelector("#command-filter"),
  commandList: document.querySelector("#command-list"),
  commandTitle: document.querySelector("#command-title"),
  commandSummary: document.querySelector("#command-summary"),
  selectedPath: document.querySelector("#selected-path"),
  commandUsage: document.querySelector("#command-usage"),
  commandAliases: document.querySelector("#command-aliases"),
  commandFlags: document.querySelector("#command-flags"),
  argumentsInput: document.querySelector("#arguments-input"),
  stdinInput: document.querySelector("#stdin-input"),
  runButton: document.querySelector("#run-command"),
  runStatus: document.querySelector("#run-status"),
  runID: document.querySelector("#run-id"),
  stdoutOutput: document.querySelector("#stdout-output"),
  stderrOutput: document.querySelector("#stderr-output"),
  runError: document.querySelector("#run-error"),
};

initialize().catch((error) => {
  renderRunError(String(error));
  setStatus("failed");
});

async function initialize() {
  bindEvents();
  setStatus("loading");
  const response = await fetch(commandsEndpoint);
  if (!response.ok) {
    throw new Error(`Failed to load commands: ${response.status}`);
  }

  /** @type {CommandCatalog} */
  const catalog = await response.json();
  const runnableCommands = catalog.commands.filter((command) => command.runnable);
  runnableCommands.sort((left, right) => left.path.localeCompare(right.path));

  state.catalog = catalog;
  state.commands = runnableCommands;
  elements.commandCount.textContent = String(runnableCommands.length);
  renderCommandList("");

  const initialCommand = runnableCommands.find((command) => command.path === "gix version") || runnableCommands[0];
  if (initialCommand) {
    selectCommand(initialCommand.path);
  }

  setStatus("idle");
}

function bindEvents() {
  elements.commandFilter.addEventListener("input", () => {
    renderCommandList(elements.commandFilter.value.trim().toLowerCase());
  });

  elements.runButton.addEventListener("click", () => {
    void submitRun();
  });
}

/**
 * @param {string} query
 */
function renderCommandList(query) {
  const filteredCommands = state.commands.filter((command) => {
    if (!query) {
      return true;
    }
    const haystack = [command.path, command.short || "", ...(command.aliases || [])].join(" ").toLowerCase();
    return haystack.includes(query);
  });

  elements.commandList.innerHTML = "";
  if (filteredCommands.length === 0) {
    const emptyState = document.createElement("div");
    emptyState.className = "empty-state";
    emptyState.textContent = "No commands match the current filter.";
    elements.commandList.append(emptyState);
    return;
  }

  filteredCommands.forEach((command) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `command-button${command.path === state.selectedPath ? " active" : ""}`;
    button.innerHTML = `
      <span class="command-path">${escapeHTML(command.path)}</span>
      <span class="command-short">${escapeHTML(command.short || command.long || "No description available")}</span>
    `;
    button.addEventListener("click", () => {
      selectCommand(command.path);
    });
    elements.commandList.append(button);
  });
}

/**
 * @param {string} commandPath
 */
function selectCommand(commandPath) {
  const command = state.commands.find((candidate) => candidate.path === commandPath);
  if (!command) {
    return;
  }

  state.selectedPath = command.path;
  renderCommandList(elements.commandFilter.value.trim().toLowerCase());
  renderCommand(command);
  populateArguments(command);
}

/**
 * @param {CommandDescriptor} command
 */
function renderCommand(command) {
  elements.commandTitle.textContent = command.path;
  elements.commandSummary.textContent = command.long || command.short || "No description available.";
  elements.selectedPath.textContent = command.path;
  elements.commandUsage.textContent = command.use || command.path;
  renderAliases(command.aliases || []);
  renderFlags(command.flags || []);
}

/**
 * @param {CommandDescriptor} command
 */
function populateArguments(command) {
  const pathSegments = command.path.split(" ").slice(1);
  elements.argumentsInput.value = pathSegments.join("\n");
}

/**
 * @param {string[]} aliases
 */
function renderAliases(aliases) {
  elements.commandAliases.innerHTML = "";
  if (aliases.length === 0) {
    const emptyState = document.createElement("span");
    emptyState.className = "muted";
    emptyState.textContent = "No aliases";
    elements.commandAliases.append(emptyState);
    return;
  }

  aliases.forEach((alias) => {
    const token = document.createElement("span");
    token.className = "alias-token";
    token.textContent = alias;
    elements.commandAliases.append(token);
  });
}

/**
 * @param {FlagDescriptor[]} flags
 */
function renderFlags(flags) {
  elements.commandFlags.innerHTML = "";
  if (flags.length === 0) {
    const emptyState = document.createElement("div");
    emptyState.className = "empty-state";
    emptyState.textContent = "This command does not expose public flags.";
    elements.commandFlags.append(emptyState);
    return;
  }

  flags.forEach((flag) => {
    const container = document.createElement("div");
    container.className = "flag-item";

    const longName = `--${flag.name}`;
    const shortName = flag.shorthand ? `-${flag.shorthand}` : "";
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
        <strong>${escapeHTML(longName)}</strong>
        ${shortName ? `<span class="flag-token">${escapeHTML(shortName)}</span>` : ""}
        ${flag.required ? `<span class="flag-token">required</span>` : ""}
      </div>
      <div class="muted">${escapeHTML(flag.usage || "No usage text available")}</div>
      ${defaultTokens.length > 0 ? `<div class="muted">${escapeHTML(defaultTokens.join(" • "))}</div>` : ""}
    `;
    elements.commandFlags.append(container);
  });
}

async function submitRun() {
  clearPolling();
  renderRunError("");
  setStatus("running");
  elements.runButton.disabled = true;

  const argumentsList = readArguments();
  if (argumentsList.length === 0) {
    renderRunError("Add at least one argument before running a command.");
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
