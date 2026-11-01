// Local configuration belongs to the runner's app chrome, outside the artifact.
for (const form of document.querySelectorAll("[data-connection-form]")) {
  const kind = form.querySelector("[name=kind]");
  function updateFields() {
    for (const group of form.querySelectorAll("[data-kinds]")) {
      const active = group.dataset.kinds.split(" ").includes(kind.value);
      group.hidden = !active;
      for (const input of group.querySelectorAll("input")) input.disabled = !active;
    }
  }
  kind.addEventListener("change", updateFields);
  updateFields();
}
