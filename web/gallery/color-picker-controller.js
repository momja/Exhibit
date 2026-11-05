import { Controller } from '../stimulus/stimulus.js';

// Local working-copy state only. The tag feature still owns saving, while
// htmx and Stimulus own replacement and reconnection of the creation fields.
export default class extends Controller {
  static targets = ['picker', 'hex', 'swatch', 'selection', 'hint', 'error', 'empty'];

  connect() { this.refresh(); }

  choose(event) {
    event.stopPropagation();
    this.setColor(event.currentTarget.dataset.color);
  }

  input(event) {
    const value = event.target.value.trim();
    if (/^#[0-9a-f]{6}$/i.test(value)) this.setColor(value);
    else {
      this.showValidity(false);
      this.renderSelection(null);
      this.dispatch('change', { detail: { valid: false } });
    }
  }

  refresh() {
    const value = this.hexTarget.value.trim();
    if (/^#[0-9a-f]{6}$/i.test(value)) this.setColor(value);
    else {
      this.showValidity(false);
      this.renderSelection(null);
      this.dispatch('change', { detail: { valid: false } });
    }
  }

  setColor(value) {
    const hex = value.toLowerCase();
    this.hexTarget.value = hex;
    this.pickerTarget.value = hex;
    this.showValidity(true);
    this.renderSelection(hex);
    this.dispatch('change', { detail: { valid: true, color: hex } });
  }

  renderSelection(hex) {
    let selectedPreset = null;
    for (const swatch of this.swatchTargets) {
      const selected = swatch.dataset.color.toLowerCase() === hex;
      swatch.setAttribute('aria-checked', String(selected));
      swatch.tabIndex = -1;
      if (selected) selectedPreset = swatch;
    }
    (selectedPreset || this.swatchTargets[0]).tabIndex = 0;
    this.selectionTarget.textContent = selectedPreset?.dataset.colorName || 'Custom';
    this.selectionTarget.hidden = !hex;
  }

  showValidity(valid) {
    this.hexTarget.setAttribute('aria-invalid', String(!valid));
    this.errorTarget.textContent = valid ? '' : 'Enter a six-digit color, like #377a64.';
    this.errorTarget.hidden = valid;
    this.hintTarget.hidden = !valid;
    this.emptyTarget.hidden = valid;
  }

  navigate(event) {
    const index = this.swatchTargets.indexOf(event.target);
    if (index < 0) return;
    const movement = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -4, ArrowDown: 4 };
    let next;
    if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = this.swatchTargets.length - 1;
    else if (event.key in movement) next = (index + movement[event.key] + this.swatchTargets.length) % this.swatchTargets.length;
    else return;
    event.preventDefault();
    const swatch = this.swatchTargets[next];
    this.setColor(swatch.dataset.color);
    swatch.focus();
  }
}
