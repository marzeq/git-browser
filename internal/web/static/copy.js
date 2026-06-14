document.querySelectorAll(".copy-link").forEach((element) => {
  element.classList.add("has-js");
});

document.addEventListener("click", async (event) => {
  const link = event.target.closest("[data-copy-text]");
  if (!link) {
    return;
  }

  const original = link.textContent;

  try {
    await navigator.clipboard.writeText(link.dataset.copyText || "");
    link.textContent = "copied";
    link.dataset.copyState = "done";
  } catch {
    link.textContent = "failed";
    link.dataset.copyState = "failed";
  }

  window.setTimeout(() => {
    link.textContent = original;
    delete link.dataset.copyState;
  }, 1200);
});
