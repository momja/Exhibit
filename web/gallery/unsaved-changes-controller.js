import { Controller } from '../stimulus/stimulus.js';
import { modalFor } from './modal.js';
import { hasEditorChanges, saveEditorChanges, discardEditorChanges, waitForEditorSave, editorSaveStatus } from './editor-session.js';

export default class UnsavedChangesController extends Controller {
  static targets = ['dialog', 'status', 'saveButton', 'saveMessage', 'saveMessageText'];

  connect() {
    this.modal = modalFor(this.dialogTarget);
    this.restore();
  }

  restore() {
    // A browser Back can revive this same controller from the page cache.
    this.allowLeave = false;
    this.modal.pending = false;
    this.modal.close();
    this.refresh();
  }

  refresh() {
    const dirty = hasEditorChanges();
    const { saving, issue } = editorSaveStatus();
    this.statusTarget.dataset.state = saving ? 'saving' : dirty ? (issue ? 'attention' : 'dirty') : 'saved';
    this.statusTarget.textContent = saving ? 'Saving…' : dirty ? (issue ? 'Review' : 'Unsaved') : 'Saved';
    const button = this.saveButtonTarget;
    button.classList.toggle('btn-sec', !dirty);
    button.disabled = saving || !dirty;
    button.setAttribute('aria-busy', String(saving));
    // Legacy footer statuses include a text glyph. The shared notice supplies
    // its own icon, so keep just the explanation and its recovery instruction.
    this.saveMessageTextTarget.textContent = dirty && !saving ? issue.replace(/^✗\s*(?:Error:\s*)?/, '') : '';
    this.saveMessageTarget.hidden = !dirty || saving || !issue;
  }

  async save() {
    await saveEditorChanges();
  }

  navigate(event) {
    const link = event.target.closest('a[href]');
    if (!link || event.defaultPrevented || event.button > 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || link.hasAttribute('download') || (link.target && link.target !== '_self')) return;
    const destination = new URL(link.href, location.href);
    if (destination.pathname === location.pathname && destination.search === location.search && destination.hash) return;
    if (!hasEditorChanges()) return;
    event.preventDefault();
    this.destination = link.href;
    this.modal.show(link);
  }

  beforeUnload(event) {
    if (this.allowLeave || !hasEditorChanges()) return;
    event.preventDefault();
    event.returnValue = '';
  }

  allowExit() {
    this.allowLeave = true;
  }

  beforeClose(event) {
    if (event.detail.reason === 'backdrop') event.preventDefault();
  }

  closed() {
    this.destination = null;
  }

  async discard() {
    if (this.modal.pending) return;
    const discarded = await this.modal.run(async () => {
      // A Save already submitted must settle before its confirmed snapshot
      // can be used to discard later edits. This never starts another save.
      await waitForEditorSave();
      discardEditorChanges();
      return true;
    });
    if (discarded) this.leave();
  }

  async saveAndLeave() {
    if (this.modal.pending) return;
    const saved = await this.modal.run(saveEditorChanges);
    if (saved === undefined) return; // The dialog was removed during this save.
    if (saved) this.leave();
    else {
      this.modal.close();
      document.getElementById('status').scrollIntoView({block: 'nearest'});
    }
  }

  leave() {
    const destination = this.destination;
    this.modal.close();
    this.allowLeave = true;
    location.assign(destination);
  }

}
