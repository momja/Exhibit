import { Controller } from '../stimulus/stimulus.js';
import { hasEditorChanges, saveEditorChanges, discardEditorChanges, waitForEditorSave } from './editor-session.js';

export default class UnsavedChangesController extends Controller {
  static targets = ['dialog', 'status'];

  connect() {
    this.restore();
  }

  restore() {
    // A browser Back can revive this same controller from the page cache.
    this.allowLeave = false;
    if (this.dialogTarget.open) this.dialogTarget.close();
    this.refresh();
  }

  refresh() {
    this.statusTarget.textContent = hasEditorChanges() ? 'Unsaved changes' : 'All changes saved';
  }

  navigate(event) {
    const link = event.target.closest('a[href]');
    if (!link || event.defaultPrevented || event.button > 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || link.hasAttribute('download') || (link.target && link.target !== '_self')) return;
    const destination = new URL(link.href, location.href);
    if (destination.pathname === location.pathname && destination.search === location.search && destination.hash) return;
    if (!hasEditorChanges()) return;
    event.preventDefault();
    this.destination = link.href;
    this.trigger = link;
    if (!this.dialogTarget.open) this.dialogTarget.showModal();
    this.dialogTarget.querySelector('[data-keep-editing]').focus();
  }

  beforeUnload(event) {
    if (this.allowLeave || !hasEditorChanges()) return;
    event.preventDefault();
    event.returnValue = '';
  }

  allowExit() {
    this.allowLeave = true;
  }

  keepEditing(event) {
    event?.preventDefault();
    if (this.saving) return;
    this.dialogTarget.close();
    this.trigger?.focus();
  }

  async discard() {
    this.setSaving(true);
    try {
      // A Save already submitted must settle before its confirmed snapshot
      // can be used to discard later edits. This never starts another save.
      await waitForEditorSave();
      discardEditorChanges();
      this.leave();
    } finally {
      this.setSaving(false);
    }
  }

  setSaving(value) {
    this.saving = value;
    this.dialogTarget.querySelectorAll('button').forEach(button => { button.disabled = value; });
  }

  async saveAndLeave() {
    this.setSaving(true);
    const saved = await saveEditorChanges();
    this.setSaving(false);
    if (saved) this.leave();
    else {
      this.keepEditing();
      document.getElementById('status').scrollIntoView({block: 'nearest'});
    }
  }

  leave() {
    this.dialogTarget.close();
    this.allowLeave = true;
    location.assign(this.destination);
  }

  disconnect() {
    if (this.dialogTarget.open) this.dialogTarget.close();
  }
}
