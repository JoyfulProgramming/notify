(function () {
  const ORDER = [
    "overview", "scope", "architecture", "repo", "contracts", "auth",
    "invariants", "ingestor", "filter", "rules", "delivery",
    "system-properties", "implementation-order", "dev-env", "live-evals",
    "dod", "open-questions", "appendix",
  ];

  const LABELS = {
    overview: "The MVP in one picture",
    scope: "Out of scope",
    architecture: "Bounded contexts & pace layers",
    repo: "Repository structure",
    contracts: "Contracts (schemas)",
    auth: "Authentication",
    invariants: "System invariants",
    ingestor: "notification-ingestor",
    filter: "filter-service",
    rules: "rule-api",
    delivery: "delivery-service",
    "system-properties": "System-wide properties",
    "implementation-order": "Implementation order",
    "dev-env": "Dev environment",
    "live-evals": "Live evaluations",
    dod: "Definition of done",
    "open-questions": "Open questions",
    appendix: "Appendix",
  };

  const navLinks = document.querySelectorAll(".nav-link");
  const panes = document.querySelectorAll(".pane");
  const prevBtn = document.getElementById("prev-btn");
  const nextBtn = document.getElementById("next-btn");

  function show(target) {
    if (!ORDER.includes(target)) target = ORDER[0];

    panes.forEach((p) => p.classList.toggle("active", p.id === "pane-" + target));
    navLinks.forEach((l) => l.classList.toggle("active", l.dataset.target === target));

    const idx = ORDER.indexOf(target);
    const prev = ORDER[idx - 1];
    const next = ORDER[idx + 1];

    prevBtn.style.visibility = prev ? "visible" : "hidden";
    nextBtn.style.visibility = next ? "visible" : "hidden";
    if (prev) { prevBtn.textContent = "← " + LABELS[prev]; prevBtn.onclick = () => navigate(prev); }
    if (next) { nextBtn.textContent = LABELS[next] + " →"; nextBtn.onclick = () => navigate(next); }

    window.scrollTo(0, 0);
  }

  function navigate(target) {
    history.pushState(null, "", "#" + target);
    show(target);
  }

  navLinks.forEach((link) => {
    link.addEventListener("click", () => navigate(link.dataset.target));
  });

  document.querySelectorAll("[data-target].inline-link").forEach((el) => {
    el.addEventListener("click", () => navigate(el.dataset.target));
  });

  window.addEventListener("popstate", () => {
    show(location.hash.replace("#", ""));
  });

  show(location.hash.replace("#", "") || ORDER[0]);

  if (window.hljs) {
    hljs.highlightAll();
  }
})();
