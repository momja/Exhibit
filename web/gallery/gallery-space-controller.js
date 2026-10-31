import { Controller } from '../stimulus/stimulus.js';
import { initializeGalleryWidgets } from './gallery-widgets.js';
import { initializeDuplication } from './duplicate.js';
import { initializeGalleryReturn } from './gallery-return.js';

// Everything here belongs to the current space. Owner dialogs, widget
// observers and edit-return links are released even during history traversal.
export default class extends Controller {
  connect() {
    this.cleanups = [initializeGalleryWidgets, initializeDuplication, initializeGalleryReturn]
      .map(initialize => initialize(this.element));
  }

  disconnect() {
    this.cleanups.forEach(cleanup => cleanup?.());
  }
}
