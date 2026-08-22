document.querySelectorAll(".markdown-preview .math-inline, .markdown-preview .math-display").forEach((element) => {
  if (typeof katex === "undefined") {
    return;
  }

  katex.render(element.textContent, element, {
    displayMode: element.classList.contains("math-display"),
    throwOnError: false,
    trust: false,
    maxSize: 50,
    maxExpand: 1000,
  });
});
