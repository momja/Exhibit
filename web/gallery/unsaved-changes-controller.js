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
    this.setSaving(false);
    this.closeDialog();
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
    this.dialogTarget.dispatchEvent(new CustomEvent('exhibit:modal-open', { detail: { trigger: link } }));
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
    this.closeDialog();
  }

  beforeClose(event) {
    if (this.saving) event.preventDefault();
  }

  closed() {
    this.destination = null;
    this.trigger = null;
  }

  closeDialog() {
    this.dialogTarget.dispatchEvent(new CustomEvent('exhibit:modal-close'));
  }

  async discard() {
    if (this.saving) return;
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
    if (this.saving) return;
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
    const destination = this.destination;
    this.setSaving(false);
    this.closeDialog();
    this.allowLeave = true;
    location.assign(destination);
  }

}
