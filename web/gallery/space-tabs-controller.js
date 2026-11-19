import { Controller } from '../stimulus/stimulus.js';

// Phone space tabs are fixed to the viewport. A sticky bar cannot leave its
// container, so when iOS rubber-bands past the end of the page it rides up with
// the content and leaves an empty band beneath it. Being fixed takes the bar out
// of document flow, so this publishes its rendered height (wrapped or enlarged
// labels, the home-indicator inset) for the layout to reserve as clearance.
export default class extends Controller {
  connect() {
    const layout = this.element.parentElement;
    this.observer = new ResizeObserver(([entry]) => {
      layout.style.setProperty('--space-tabs-height', `${entry.borderBoxSize[0].blockSize}px`);
    });
    // The border box: the home-indicator inset is padding, and the default
    // content box would not report it changing.
    this.observer.observe(this.element, { box: 'border-box' });
  }

  disconnect() {
    this.observer.disconnect();
  }
}
