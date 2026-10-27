// Sections own their working copies and persistence. Navigation asks this one
// interface whether leaving is safe, without knowing how each section saves.
const sections = new Map();
let saving = null;

export function editorChanged() {
  document.body.dispatchEvent(new CustomEvent('exhibit:editor-changed'));
}

export function registerEditorSection(name, section) {
  sections.set(name, section);
}

export function hasEditorChanges() {
  return !!saving || [...sections.values()].some(section => section.isDirty());
}

export function waitForEditorSave() {
  return saving;
}

export function discardEditorChanges() {
  for (const section of sections.values()) section.discard();
  editorChanged();
}

export function saveEditorChanges() {
  if (saving) return saving;
  saving = Promise.resolve().then(async () => {
    try {
      for (const section of sections.values()) {
        if (section.isDirty() && await section.save() !== true) return false;
      }
      return ![...sections.values()].some(section => section.isDirty());
    } catch (error) {
      document.getElementById('status').textContent = 'Could not save: ' + error.message;
      return false;
    }
  }).finally(() => {
    saving = null;
    editorChanged();
  });
  return saving;
}
