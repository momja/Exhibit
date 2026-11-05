import { Controller } from '../stimulus/stimulus.js';

// Local working-copy state only. The tag feature still owns saving, while
// htmx and Stimulus own replacement and reconnection of the creation fields.
export default class extends Controller {
  static targets = ['picker', 'hex', 'swatch'];

  connect() { this.refresh(); }

  choose(event) {
    event.stopPropagation();
    this.setColor(event.currentTarget.dataset.color);
  }

  input(event) {
    const value = event.target.value.trim();
    if (/^#[0-9a-f]{6}$/i.test(value)) this.setColor(value);
  }

  refresh() {
    const value = this.hexTarget.value.trim();
    if (/^#[0-9a-f]{6}$/i.test(value)) this.setColor(value);
  }

  setColor(value) {
    const hex = value.toLowerCase();
    this.hexTarget.value = hex;
    this.pickerTarget.value = hex;
    let preset = false;
    for (const swatch of this.swatchTargets) {
      const selected = swatch.dataset.color.toLowerCase() === hex;
      swatch.setAttribute('aria-pressed', String(selected));
      preset ||= selected;
    }
    this.element.querySelector('.color-custom-row')?.classList.toggle('is-custom', !preset);
  }
}
