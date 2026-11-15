import { Controller } from '../stimulus/stimulus.js';

// On phones the bar is fixed, so this slot reserves the height it gives up (Exh-b6r6).
export default class extends Controller {
  connect() {
    const header = this.element.querySelector('.app-header');
    this.observer = new ResizeObserver(([entry]) => {
      this.element.style.setProperty('--app-header-height', `${entry.borderBoxSize[0].blockSize}px`);
    });
    this.observer.observe(header, { box: 'border-box' });
  }

  disconnect() {
    this.observer.disconnect();
  }
}
