import { Controller } from '../stimulus/stimulus.js';
import { connectFloatingHeader } from './pip.js';

export default class extends Controller {
  connect() { this.dispose = connectFloatingHeader(this.element); }
  disconnect() { this.dispose?.(); }
}
