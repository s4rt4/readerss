(function () {
  const root = document.documentElement;
  const shell = document.querySelector("[data-app-shell]");
  const reader = document.querySelector(".reader");
  const helpModal = document.querySelector("[data-help-modal]");
  let gPrefix = false;
  const detail = {
    source: document.querySelector("[data-detail-source]"),
    time: document.querySelector("[data-detail-time]"),
    tag: document.querySelector("[data-detail-tag]"),
    title: document.querySelector("[data-detail-title]"),
    summary: document.querySelector("[data-detail-summary]"),
    body: document.querySelector("[data-detail-body]"),
  };

  const storedTheme = localStorage.getItem("readress-theme");
  if (storedTheme) {
    root.dataset.theme = storedTheme;
  }
  syncThemeLogos();

  if ("serviceWorker" in navigator) {
    window.addEventListener("load", () => {
      navigator.serviceWorker.register("/static/sw.js").catch(() => {});
    });
  }

  function syncThemeLogos() {
    const isDark = root.dataset.theme === "dark";
    document.querySelectorAll("[data-logo-light][data-logo-dark]").forEach((logo) => {
      logo.src = isDark ? logo.dataset.logoDark : logo.dataset.logoLight;
    });
  }

  function openSidebar() {
    shell?.classList.add("sidebar-open");
  }

  function closeSidebar() {
    shell?.classList.remove("sidebar-open");
  }

  function openHelp() {
    if (helpModal) {
      helpModal.hidden = false;
    }
  }

  function closeHelp() {
    if (helpModal) {
      helpModal.hidden = true;
    }
  }

  function selectArticle(row) {
    if (!row || !detail.title) {
      return;
    }

    document.querySelectorAll("[data-article-id]").forEach((item) => {
      item.classList.toggle("active", item === row);
    });

    const title = row.dataset.title || "";
    const source = row.dataset.source || "";
    const time = row.dataset.time || "";
    const tag = row.dataset.tag || "";
    const summary = row.dataset.summary || "";
    let paragraphs = [summary];
    try {
      const parsed = JSON.parse(row.dataset.content || "[]");
      if (Array.isArray(parsed) && parsed.length) {
        paragraphs = parsed;
      }
    } catch (_) {
      paragraphs = [summary];
    }

    detail.source.textContent = source;
    detail.time.textContent = time;
    detail.tag.textContent = tag;
    detail.title.textContent = title;
    detail.summary.textContent = summary;
    detail.body.innerHTML = paragraphs.map((paragraph) => "<p>" + escapeHTML(String(paragraph)) + "</p>").join("");
    reader?.classList.add("detail-open");
  }

  function escapeHTML(value) {
    return value.replace(/[&<>"']/g, function (char) {
      return {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#039;",
      }[char];
    });
  }

  document.querySelector("[data-open-sidebar]")?.addEventListener("click", openSidebar);
  document.querySelectorAll("[data-close-sidebar]").forEach((item) => item.addEventListener("click", closeSidebar));
  document.querySelector("[data-close-detail]")?.addEventListener("click", function () {
    reader?.classList.remove("detail-open");
  });
  document.querySelector("[data-open-help]")?.addEventListener("click", openHelp);
  document.querySelector("[data-close-help]")?.addEventListener("click", closeHelp);

  document.querySelector("[data-toggle-theme]")?.addEventListener("click", function () {
    const next = root.dataset.theme === "dark" ? "" : "dark";
    if (next) {
      root.dataset.theme = next;
      localStorage.setItem("readress-theme", next);
    } else {
      delete root.dataset.theme;
      localStorage.removeItem("readress-theme");
    }
    syncThemeLogos();
  });

  document.querySelector("[data-theme-setting]")?.addEventListener("change", function (event) {
    const next = event.currentTarget.value;
    if (next === "dark" || next === "light") {
      root.dataset.theme = next;
      localStorage.setItem("readress-theme", next);
    } else {
      delete root.dataset.theme;
      localStorage.removeItem("readress-theme");
    }
    syncThemeLogos();
  });

  document.querySelectorAll("[data-article-id]").forEach((row) => {
    row.querySelector(".article-select")?.addEventListener("click", () => selectArticle(row));
  });

  document.querySelector("[data-search-input]")?.addEventListener("keydown", function (event) {
    if (event.key === "Enter") {
      event.preventDefault();
      const query = event.currentTarget.value.trim();
      window.location.href = query ? "/search?q=" + encodeURIComponent(query) : "/search";
    }
  });

  if (window.EventSource && document.querySelector("[data-unread-count]")) {
    const events = new EventSource("/events");
    events.addEventListener("unread", (event) => {
      document.querySelectorAll("[data-unread-count]").forEach((item) => {
        item.textContent = event.data;
      });
    });
  }

  function activeArticle() {
    return document.querySelector("[data-article-id].active") || document.querySelector("[data-article-id]");
  }

  function moveArticle(delta) {
    const rows = Array.from(document.querySelectorAll("[data-article-id]"));
    if (!rows.length) {
      return;
    }
    const current = activeArticle();
    const index = Math.max(0, rows.indexOf(current));
    const next = rows[Math.min(rows.length - 1, Math.max(0, index + delta))];
    next.focus();
    selectArticle(next);
  }

  function toggleActiveClass(name) {
    const row = activeArticle();
    if (!row) {
      return;
    }
    row.classList.toggle(name);
    if (name === "unread") {
      row.classList.remove("active");
      row.classList.add("active");
    }
  }

  document.addEventListener("keydown", function (event) {
    if (event.target.matches("input, textarea")) {
      return;
    }

    if (gPrefix) {
      gPrefix = false;
      if (event.key === "g") {
        event.preventDefault();
        const first = document.querySelector("[data-article-id]");
        first?.scrollIntoView({ block: "nearest" });
        first?.querySelector(".article-select")?.click();
      }
      if (event.key === "h") {
        window.location.href = "/feed-health";
      }
      if (event.key === "s") {
        window.location.href = "/settings";
      }
      return;
    }

    if (event.key === "g") {
      gPrefix = true;
      window.setTimeout(() => {
        gPrefix = false;
      }, 900);
      return;
    }

    if (event.key === "j") {
      event.preventDefault();
      moveArticle(1);
    }

    if (event.key === "k") {
      event.preventDefault();
      moveArticle(-1);
    }

    if (event.key === "o") {
      activeArticle()?.querySelector(".article-select")?.click();
    }

    if (event.key === "s") {
      event.preventDefault();
      activeArticle()?.querySelector("[data-star-action]")?.click();
    }

    if (event.key === "m") {
      event.preventDefault();
      activeArticle()?.querySelector("[data-read-action]")?.click();
    }

    if (event.key === "r") {
      document.querySelector("[data-refresh-form]")?.requestSubmit();
    }

    if (event.key === "G") {
      event.preventDefault();
      const rows = Array.from(document.querySelectorAll("[data-article-id]"));
      const last = rows[rows.length - 1];
      last?.scrollIntoView({ block: "nearest" });
      last?.querySelector(".article-select")?.click();
    }

    if (event.key === "a") {
      window.location.href = "/feeds/manage";
    }

    if (event.key === "?") {
      openHelp();
    }

    if (event.key === "Escape") {
      closeHelp();
      closeSidebar();
    }

    if (event.key === "/") {
      event.preventDefault();
      document.getElementById("search")?.focus();
    }
  });
})();
