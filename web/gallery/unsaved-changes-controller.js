import { Controller } from '../stimulus/stimulus.js';
import { modalFor } from './modal.js';
import { hasEditorChanges, saveEditorChanges, discardEditorChanges, waitForEditorSave } from './editor-session.js';

export default class UnsavedChangesController extends Controller {
  static targets = ['dialog', 'status'];

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
