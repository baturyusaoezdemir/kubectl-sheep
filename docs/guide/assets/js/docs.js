(function () {
  const PAGES = {
    introduction: "introduction.md",
    install: "install.md",
    quickstart: "quickstart.md",
    "rancher-instances": "rancher-instances.md",
    authentication: "authentication.md",
    kubeconfigs: "kubeconfigs.md",
    interactive: "interactive.md",
    "exec-kubeconfigs": "exec-kubeconfigs.md",
    connect: "connect.md",
    "wsl-setup": "wsl-setup.md",
    configuration: "configuration.md",
  };

  const contentEl = document.getElementById("docs-content");
  const navEl = document.getElementById("docs-nav");
  const searchEl = document.getElementById("docs-search");

  marked.setOptions({ gfm: true });

  function fixLinks(html) {
    return html.replace(/href="([^"#][^"]*?)\.md"/g, (_, path) => {
      const page = path.replace(/^\.\//, "");
      return `href="#/${page}"`;
    });
  }

  function currentPage() {
    const raw = (location.hash || "#/introduction").slice(2);
    return raw || "introduction";
  }

  function setActiveNav(page) {
    navEl.querySelectorAll("a[data-page]").forEach((link) => {
      link.classList.toggle("active", link.dataset.page === page);
    });
  }

  function addCopyButtons() {
    contentEl.querySelectorAll("pre").forEach((pre) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "copy-btn";
      btn.textContent = "Copy";
      btn.addEventListener("click", async () => {
        const code = pre.querySelector("code");
        const text = code ? code.textContent : pre.textContent;
        try {
          await navigator.clipboard.writeText(text.trim());
          btn.textContent = "Copied";
          setTimeout(() => {
            btn.textContent = "Copy";
          }, 1500);
        } catch {
          btn.textContent = "Failed";
        }
      });
      pre.appendChild(btn);
    });
  }

  async function loadPage(page) {
    const file = PAGES[page];
    if (!file) {
      contentEl.innerHTML =
        '<p class="docs-error">Page not found. <a href="#/introduction">Go to introduction</a>.</p>';
      setActiveNav("");
      return;
    }

    contentEl.innerHTML = '<p class="docs-loading">Loading…</p>';
    setActiveNav(page);

    try {
      const res = await fetch(file);
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const md = await res.text();
      contentEl.innerHTML = fixLinks(marked.parse(md));
      addCopyButtons();
      document.title = `${extractTitle(md)} — kubectl-sheep`;
      window.scrollTo(0, 0);
    } catch (err) {
      contentEl.innerHTML = `<p class="docs-error">Failed to load <code>${file}</code>: ${err.message}</p>`;
    }
  }

  function extractTitle(md) {
    const match = md.match(/^#\s+(.+)$/m);
    return match ? match[1].trim() : "Documentation";
  }

  function route() {
    loadPage(currentPage());
  }

  searchEl.addEventListener("input", () => {
    const q = searchEl.value.trim().toLowerCase();
    navEl.querySelectorAll("a[data-page]").forEach((link) => {
      const match = link.textContent.toLowerCase().includes(q);
      link.classList.toggle("hidden", !match);
    });
    navEl.querySelectorAll(".nav-group").forEach((group) => {
      const visible = group.querySelectorAll("a:not(.hidden)").length > 0;
      group.style.display = visible ? "" : "none";
    });
  });

  window.addEventListener("hashchange", route);
  route();
})();
