(function () {
  const root = document.documentElement;
  const shell = document.querySelector("[data-app-shell]");
  const reader = document.querySelector(".reader");
  const helpModal = document.querySelector("[data-help-modal]");
  const articleList = document.querySelector("[data-article-list]");
  const viewMenuTrigger = document.querySelector("[data-view-menu-trigger]");
  const viewPopover = document.querySelector("[data-view-popover]");
  let gPrefix = false;
  const detail = {
    source: document.querySelector("[data-detail-source]"),
    time: document.querySelector("[data-detail-time]"),
    tag: document.querySelector("[data-detail-tag]"),
    title: document.querySelector("[data-detail-title]"),
    link: document.querySelector("[data-detail-link]"),
    summary: document.querySelector("[data-detail-summary]"),
    body: document.querySelector("[data-detail-body]"),
    image: document.querySelector("[data-detail-image]"),
  };

  const storedTheme = localStorage.getItem("readress-theme");
  if (storedTheme) {
    root.dataset.theme = storedTheme;
  }
  if (shell && sessionStorage.getItem("readress-sidebar-open") === "1") {
    shell.classList.add("sidebar-open");
  }
  syncThemeLogos();
  applyArticleView(localStorage.getItem("readress-view") || "magazine");

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
    sessionStorage.setItem("readress-sidebar-open", "1");
  }

  function closeSidebar() {
    shell?.classList.remove("sidebar-open");
    sessionStorage.setItem("readress-sidebar-open", "0");
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

  function openViewMenu() {
    if (!viewPopover) {
      return;
    }
    viewPopover.hidden = false;
    viewMenuTrigger?.setAttribute("aria-expanded", "true");
  }

  function closeViewMenu() {
    if (!viewPopover) {
      return;
    }
    viewPopover.hidden = true;
    viewMenuTrigger?.setAttribute("aria-expanded", "false");
  }

  function closeBoardPopovers(except) {
    document.querySelectorAll("[data-board-save-popover]").forEach((popover) => {
      if (popover !== except) {
        popover.hidden = true;
        popover.closest("[data-board-save]")?.querySelector("[data-board-save-trigger]")?.setAttribute("aria-expanded", "false");
      }
    });
  }

  function applyArticleView(view) {
    const allowed = ["magazine", "cards", "article", "title"];
    const next = allowed.includes(view) ? view : "magazine";
    if (articleList) {
      articleList.dataset.view = next;
    }
    document.querySelectorAll("[data-view-option]").forEach((option) => {
      option.classList.toggle("selected", option.dataset.viewOption === next);
    });
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
    const image = row.dataset.image || "";
    const url = row.dataset.url || "#";
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
    if (detail.link) {
      detail.link.href = url || "#";
    }
    detail.summary.textContent = summary;
    if (detail.image) {
      if (image) {
        detail.image.src = image;
        detail.image.hidden = false;
      } else {
        detail.image.removeAttribute("src");
        detail.image.hidden = true;
      }
    }
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

  function readCookie(name) {
    return document.cookie
      .split(";")
      .map((part) => part.trim())
      .find((part) => part.startsWith(name + "="))
      ?.slice(name.length + 1) || "";
  }

  function ensureCSRFField(form) {
    if ((form.method || "").toLowerCase() !== "post") {
      return;
    }
    const token = decodeURIComponent(readCookie("readress_csrf"));
    if (!token) {
      return;
    }
    let field = form.querySelector("input[name='csrf_token']");
    if (!field) {
      field = document.createElement("input");
      field.type = "hidden";
      field.name = "csrf_token";
      form.appendChild(field);
    }
    field.value = token;
  }

  document.querySelector("[data-open-sidebar]")?.addEventListener("click", openSidebar);
  document.querySelectorAll("[data-close-sidebar]").forEach((item) => item.addEventListener("click", closeSidebar));
  document.querySelector("[data-close-detail]")?.addEventListener("click", function () {
    reader?.classList.remove("detail-open");
  });
  document.querySelector("[data-open-help]")?.addEventListener("click", openHelp);
  document.querySelector("[data-close-help]")?.addEventListener("click", closeHelp);
  document.querySelector("[data-toggle-category-form]")?.addEventListener("click", function (event) {
    const form = document.querySelector("[data-category-form]");
    if (!form) {
      return;
    }
    const willOpen = form.hidden;
    form.hidden = !willOpen;
    event.currentTarget.setAttribute("aria-expanded", String(willOpen));
    if (willOpen) {
      form.querySelector("input")?.focus();
    }
  });
  viewMenuTrigger?.addEventListener("click", function (event) {
    event.stopPropagation();
    viewMenuTrigger.blur();
    if (viewPopover?.hidden) {
      openViewMenu();
    } else {
      closeViewMenu();
    }
  });
  document.querySelectorAll("[data-view-option]").forEach((option) => {
    option.addEventListener("click", function () {
      const next = option.dataset.viewOption || "magazine";
      localStorage.setItem("readress-view", next);
      applyArticleView(next);
      closeViewMenu();
    });
  });
  document.addEventListener("click", function (event) {
    if (!event.target.closest("[data-view-menu]")) {
      closeViewMenu();
    }
    if (!event.target.closest("[data-board-save]")) {
      closeBoardPopovers();
    }
  });

  document.querySelectorAll("[data-board-save-trigger]").forEach((trigger) => {
    trigger.addEventListener("click", function (event) {
      event.stopPropagation();
      const wrapper = trigger.closest("[data-board-save]");
      const popover = wrapper?.querySelector("[data-board-save-popover]");
      if (!popover) {
        return;
      }
      const willOpen = popover.hidden;
      closeBoardPopovers(popover);
      popover.hidden = !willOpen;
      trigger.setAttribute("aria-expanded", String(willOpen));
    });
  });

  document.querySelectorAll("form[data-confirm]").forEach((form) => {
    form.addEventListener("submit", function (event) {
      ensureCSRFField(form);
      if (!window.confirm(form.dataset.confirm || "Continue?")) {
        event.preventDefault();
      }
    });
  });

  document.querySelectorAll("form[data-loading-label]").forEach((form) => {
    form.addEventListener("submit", function () {
      ensureCSRFField(form);
      const button = form.querySelector("button[type='submit']");
      if (!button) {
        return;
      }
      button.disabled = true;
      button.classList.add("is-loading");
      const label = form.dataset.loadingLabel || "Working";
      const textSpan = button.querySelector("span");
      if (textSpan) {
        textSpan.textContent = label;
      } else {
        button.setAttribute("aria-label", label);
      }
    });
  });

  document.querySelectorAll("form[method='post']").forEach((form) => {
    form.addEventListener("submit", function () {
      ensureCSRFField(form);
    });
  });

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

    if (event.key === "l") {
      event.preventDefault();
      activeArticle()?.querySelector("[data-read-later-action]")?.click();
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
      closeViewMenu();
      closeBoardPopovers();
    }

    if (event.key === "/") {
      event.preventDefault();
      document.getElementById("search")?.focus();
    }
  });
})();
