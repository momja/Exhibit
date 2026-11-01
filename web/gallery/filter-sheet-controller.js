import { Controller } from '../stimulus/stimulus.js';
import { modalFor } from './modal.js';

// The gallery owns which sheet its trigger opens; modal owns its lifecycle.
// htmx replaces the options while both controller roots remain connected.
export default class FilterSheetController extends Controller {
  static targets = ['dialog', 'trigger'];

  open() {
    modalFor(this.dialogTarget).show(this.triggerTarget);
  }
}
