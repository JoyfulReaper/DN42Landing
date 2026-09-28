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

    const commit = activity[0];

    const sha = typeof commit.sha === "string"
      ? commit.sha.slice(0, 7)
      : "unknown";

    const repository = commit.repository || "unknown repository";
    const branch = commit.branch || "unknown branch";
    const message = commit.message || "(no commit message)";
    const author = commit.author || commit.authorUsername || "Unknown author";

    const header = textElement(
      "div",
      `${sha} [${repository}:${branch}]`,
      "toy-code"
    );

    const messageElement = textElement(
      "div",
      message,
      "toy-commit-message"
    );

    const metadata = textElement(
      "div",
      `${author}${commit.timestamp ? ` · ${commit.timestamp}` : ""}`,
      "toy-meta"
    );

    container.replaceChildren(
      header,
      messageElement,
      metadata
    );
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

void loadQotd();
void loadRecentGit();
