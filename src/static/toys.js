function textElement(tag, text, className = "") {
  const element = document.createElement(tag);

  if (className) {
    element.className = className;
  }

  element.textContent = text;
  return element;
}

async function loadQotd() {
  const container = document.getElementById("qotd");
  if (!container) return;

  try {
    const response = await fetch("/api/toys/qotd", {
      cache: "no-store",
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    const quote = await response.json();

    container.replaceChildren(
      textElement(
        "div",
        `"${quote.text || "No quote available."}"`,
        "toy-quote"
      ),
      textElement(
        "div",
        `— ${quote.author || "Unknown"}${
          quote.source ? ` · ${quote.source}` : ""
        }`,
        "toy-meta"
      )
    );
  } catch (error) {
    console.error("QOTD:", error);

    container.replaceChildren(
      textElement(
        "span",
        "Quote of the Day is temporarily unavailable.",
        "warning"
      )
    );
  }
}

async function loadRandomQuote() {
  const container = document.getElementById("random-quote");
  const button = document.getElementById("random-quote-refresh");

  if (!container) return;

  if (button) {
    button.disabled = true;
  }

  container.replaceChildren(
    textElement("span", "Selecting wisdom at random...", "muted")
  );

  try {
    const response = await fetch("/api/toys/random-quote", {
      cache: "no-store",
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    const quote = await response.json();

    container.replaceChildren(
      textElement(
        "div",
        `"${quote.text || "No quote available."}"`,
        "toy-quote"
      ),
      textElement(
        "div",
        `— ${quote.author || "Unknown"}${
          quote.source ? ` · ${quote.source}` : ""
        }`,
        "toy-meta"
      )
    );
  } catch (error) {
    console.error("Random quote:", error);

    container.replaceChildren(
      textElement(
        "span",
        "Random quote is temporarily unavailable.",
        "warning"
      )
    );
  } finally {
    if (button) {
      button.disabled = false;
    }
  }
}

async function loadRecentGit() {
  const container = document.getElementById("recent-git");
  if (!container) return;

  try {
    const response = await fetch("/api/toys/git", {
      cache: "no-store",
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    const activity = await response.json();

    if (!Array.isArray(activity) || activity.length === 0) {
      throw new Error("No Git activity returned");
    }

    const fragment = document.createDocumentFragment();

    for (const commit of activity) {
      const item = document.createElement("div");
      item.className = "git-item";

      const sha =
        typeof commit.sha === "string"
          ? commit.sha.slice(0, 7)
          : "unknown";

      const repository =
        commit.repository || "unknown repository";

      const branch =
        commit.branch || "unknown branch";

      const message =
        commit.message || "(no commit message)";

      const author =
        commit.author ||
        commit.authorUsername ||
        "Unknown author";

      item.append(
        textElement(
          "div",
          `${sha} [${repository}:${branch}]`,
          "toy-code"
        ),
        textElement(
          "div",
          message,
          "toy-commit-message"
        ),
        textElement(
          "div",
          `${author}${
            commit.timestamp
              ? ` · ${commit.timestamp}`
              : ""
          }`,
          "toy-meta"
        )
      );

      fragment.append(item);
    }

    container.replaceChildren(fragment);
  } catch (error) {
    console.error("Git activity:", error);

    container.replaceChildren(
      textElement(
        "span",
        "Recent Git activity is temporarily unavailable.",
        "warning"
      )
    );
  }
}

async function loadWorkstation() {
  const container = document.getElementById("workstation");
  const button = document.getElementById("workstation-refresh");

  if (!container) return;

  if (button) {
    button.disabled = true;
  }

  container.replaceChildren(
    textElement("span", "Poking the workstation...", "muted")
  );

  try {
    const response = await fetch("/api/toys/workstation", {
      cache: "no-store",
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    const status = await response.json();

    const grid = document.createElement("dl");
    grid.className = "status-grid";

    const fields = [
      ["OS", `${status.os || "Unknown"} / ${status.architecture || "?"}`],
      ["Uptime", status.uptime || "Unknown"],
      ["CPU", `${status.cpuUsage || "?"} / ${status.cpuCores || "?"} cores`],
      ["RAM", status.ram || "Unknown"],
      ["Storage", status.storage || "Unknown"],
      ["GPU", status.gpu?.name || "Unknown"],
      [
        "GPU load",
        status.gpu?.loadPercentage != null
          ? `${status.gpu.loadPercentage}%`
          : "Unknown",
      ],
      [
        "VRAM",
        status.gpu?.vramUsedMB && status.gpu?.vramTotalMB
          ? `${status.gpu.vramUsedMB} / ${status.gpu.vramTotalMB} MB`
          : "Unknown",
      ],
      ["Processes", String(status.processCount ?? "Unknown")],
      ["Runtime", status.framework || "Unknown"],
    ];

    for (const [label, value] of fields) {
      grid.append(
        textElement("dt", label),
        textElement("dd", value)
      );
    }

    container.replaceChildren(grid);
  } catch (error) {
    console.error("Workstation status:", error);

    container.replaceChildren(
      textElement(
        "span",
        "Workstation telemetry is temporarily unavailable.",
        "warning"
      )
    );
  } finally {
    if (button) {
      button.disabled = false;
    }
  }
}

document
  .getElementById("workstation-refresh")
  ?.addEventListener("click", () => {
    void loadWorkstation();
  });

document
  .getElementById("random-quote-refresh")
  ?.addEventListener("click", () => {
    void loadRandomQuote();
  });

void loadWorkstation();
void loadRandomQuote();
void loadQotd();
void loadRecentGit();
