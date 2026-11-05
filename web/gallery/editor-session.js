// Sections own their working copies and persistence. Navigation asks this one
// interface whether leaving is safe, without knowing how each section saves.
const sections = new Map();
let saving = null;
let saveIssue = '';

export function editorSaveStatus() {
  return { saving: !!saving, issue: saveIssue };
}

export function editorChanged() {
  // Approval or discard can resolve an issue outside the Save button. Do not
  // carry that old message into the next, unrelated working copy.
  if (!hasEditorChanges()) saveIssue = '';
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
  saveIssue = '';
  for (const section of sections.values()) section.discard();
  editorChanged();
}

export function saveEditorChanges() {
  if (saving) return saving;
  saveIssue = '';
  saving = Promise.resolve().then(async () => {
    try {
      for (const section of sections.values()) {
        if (section.isDirty() && await section.save() !== true) {
          saveIssue = section.saveMessage?.() || 'Some changes need your attention. Review the messages in the editor before leaving.';
          return false;
        }
      }
      return ![...sections.values()].some(section => section.isDirty());
    } catch (error) {
      saveIssue = 'Could not save: ' + error.message;
      document.getElementById('status').textContent = saveIssue;
      return false;
    }
  }).finally(() => {
    saving = null;
    editorChanged();
  });
  editorChanged();
  return saving;
}
