import { Controller } from '../stimulus/stimulus.js';
import { consumeOutsidePress } from './outside-press.js';

// Existing explicit activation owns open state; this controller keeps the
// server-rendered receipt within the viewport and releases its layout hooks.
export default class extends Controller {
  connect() {
    this.trigger = this.element.querySelector('[data-capability-trigger]');
    this.panel = this.element.querySelector('.capability-popover');
    this.reposition = event => {
      if (event?.target instanceof Node && this.panel.contains(event.target)) return;
      cancelAnimationFrame(this.frame);
      this.frame = requestAnimationFrame(() => this.position());
    };
    this.releaseOutsidePress = consumeOutsidePress({
      isOpen: () => this.element.classList.contains('is-open'),
      contains: target => this.element.contains(target),
      dismiss: () => this.close(),
    });
    this.sync();
  }

  disconnect() {
    this.close();
    this.releaseOutsidePress();
  }

  sync() {
    const open = this.element.classList.contains('is-open');
    if (open !== this.listening) {
      const method = open ? 'addEventListener' : 'removeEventListener';
      window[method]('resize', this.reposition);
      window[method]('scroll', this.reposition, true);
      window.visualViewport?.[method]('resize', this.reposition);
      window.visualViewport?.[method]('scroll', this.reposition);
      this.listening = open;
    }
    if (open) this.position();
    else cancelAnimationFrame(this.frame);
  }

  close() {
    this.element.classList.remove('is-open');
    this.trigger.setAttribute('aria-expanded', 'false');
    this.sync();
  }

  position() {
    if (!this.element.classList.contains('is-open')) return;
    const viewport = window.visualViewport;
    const leftEdge = (viewport?.offsetLeft ?? 0) + 16;
    const topEdge = (viewport?.offsetTop ?? 0) + 16;
    const rightEdge = leftEdge + (viewport?.width ?? innerWidth) - 32;
    const bottomEdge = topEdge + (viewport?.height ?? innerHeight) - 32;
    const trigger = this.trigger.getBoundingClientRect();
    if (trigger.bottom < topEdge || trigger.top > bottomEdge) return this.close();
    const above = Math.max(0, trigger.top - topEdge - 8);
    const below = Math.max(0, bottomEdge - trigger.bottom - 8);
    this.panel.style.maxHeight = `${bottomEdge - topEdge}px`;
    const natural = this.panel.getBoundingClientRect();
    const upward = natural.height > below && above > below;
    const room = upward ? above : below;
    this.panel.style.maxHeight = `${room >= 96 ? room : bottomEdge - topEdge}px`;
    const bounds = this.panel.getBoundingClientRect();
    const anchor = (this.element.closest('.card') ?? this.trigger).getBoundingClientRect();
    const left = Math.max(leftEdge, Math.min(anchor.left, rightEdge - bounds.width));
    const preferredTop = upward ? trigger.top - 8 - bounds.height : trigger.bottom + 8;
    const top = Math.max(topEdge, Math.min(preferredTop, bottomEdge - bounds.height));
    this.panel.style.left = `${left}px`;
    this.panel.style.top = `${top}px`;
  }
}
