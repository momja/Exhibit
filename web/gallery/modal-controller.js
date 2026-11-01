import { Controller } from '../stimulus/stimulus.js';
import { connectModal } from './modal.js';

// Stimulus owns connection/removal. The dialog module owns every listener and
// the stable feature handle, including when show() precedes this connection.
export default class extends Controller {
  connect() { this.disconnectModal = connectModal(this.element); }
  disconnect() { this.disconnectModal(); }
}
