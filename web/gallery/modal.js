// The feature interface: modalFor(dialog).show(trigger), close(), run(operation),
// and pending. No Stimulus lookup or startup ordering is needed by a caller.
const modals = new WeakMap();

export function modalFor(element) {
  return stateFor(element).handle;
}

// Lifecycle adapter used only by the registered Stimulus controller. show/run
// also connect synchronously, so an early user click never waits for Stimulus.
export function connectModal(element) {
  const modal = stateFor(element);
  modal.connect();
  return () => modal.disconnect();
}

function stateFor(element) {
  if (!modals.has(element)) modals.set(element, new Modal(element));
  return modals.get(element);
}

class Modal {
  constructor(element) {
    this.element = element;
    this.operation = null;
    const modal = this;
    this.handle = {
      show: trigger => this.show(trigger),
      close: () => this.close(),
      run: operation => this.run(operation),
      get pending() { return modal.operation !== null; },
      set pending(value) { value ? modal.begin() : modal.finish(); }
    };
    this.listeners = {
      cancel: event => { event.preventDefault(); this.close(); },
      click: event => this.click(event),
      keydown: event => this.wrapFocus(event)
    };
  }

  connect() {
    if (this.connected) return;
    for (const [name, listener] of Object.entries(this.listeners)) this.element.addEventListener(name, listener);
    this.connected = true;
  }

  disconnect() {
    if (!this.connected) return;
    for (const [name, listener] of Object.entries(this.listeners)) this.element.removeEventListener(name, listener);
    this.connected = false;
    // Removal releases the top layer, pending controls, and stale completions,
    // without focusing a removed region.
    if (this.element.open) this.element.close();
    this.returnFocus = null;
    this.restoreControls();
    this.operation = null;
    this.element.dispatchEvent(new CustomEvent('modal:disconnected', { bubbles: true }));
  }

  show(trigger) {
    if (!this.element.isConnected || this.element.open || this.operation) return false;
    this.connect();
    this.returnFocus = trigger || this.element.ownerDocument.activeElement;
    this.element.showModal();
    this.element.querySelector('[autofocus]')?.focus();
    return true;
  }

  close(reason = 'dismiss') {
    if (!this.element.open || this.operation) return false;
    const event = new CustomEvent('modal:before-close', { bubbles: true, cancelable: true, detail: { reason } });
    this.element.dispatchEvent(event);
    if (event.defaultPrevented) return false;
    this.element.close();
    // Restore synchronously. A queued native close event must not steal focus
    // from a reopened dialog or from a server-rendered replacement trigger.
    if (this.returnFocus?.isConnected) this.returnFocus.focus();
    this.returnFocus = null;
    this.element.dispatchEvent(new CustomEvent('modal:closed', { bubbles: true }));
    return true;
  }

  begin() {
    if (this.operation) return this.operation;
    this.connect();
    // Buttons lock by default; a native fieldset can lock a whole working copy.
    // Restore each control's own disabled state, including previously disabled
    // controls. Values and user input are never reset here.
    const operation = { controls: [...this.element.querySelectorAll('button, fieldset')]
      .map(control => [control, control.disabled]) };
    this.operation = operation;
    for (const [control] of operation.controls) control.disabled = true;
    return operation;
  }

  finish() {
    if (!this.operation) return;
    this.restoreControls();
    this.operation = null;
    // A pending form may have replaced its focused element.
    if (this.element.open && !this.element.contains(this.element.ownerDocument.activeElement)) {
      this.element.querySelector('[autofocus]')?.focus();
      if (!this.element.contains(this.element.ownerDocument.activeElement)) this.element.focus();
    }
  }

  async run(callback) {
    if (this.operation) return;
    const operation = this.begin();
    try {
      // Invoke immediately, before awaiting: user activation for clipboard or
      // popup work must remain in the original click's call stack.
      const result = await callback();
      if (this.operation === operation) return result;
    } finally {
      if (this.operation === operation) this.finish();
    }
  }

  restoreControls() {
    for (const [control, disabled] of this.operation?.controls || []) control.disabled = disabled;
  }

  click(event) {
    if (event.target.closest('[data-modal-dismiss]')) return this.close();
    if (event.target !== this.element) return;
    const bounds = this.element.getBoundingClientRect();
    if (event.clientX < bounds.left || event.clientX > bounds.right ||
        event.clientY < bounds.top || event.clientY > bounds.bottom) this.close('backdrop');
  }

  wrapFocus(event) {
    if (event.key !== 'Tab') return;
    // Chromium otherwise tabs out to browser chrome at the last field.
    const fields = [...this.element.querySelectorAll('a[href], button, input, select, textarea, [tabindex]')]
      .filter(field => field.tabIndex >= 0 && !field.matches(':disabled') && field.getClientRects().length);
    const first = fields[0];
    const last = fields.at(-1);
    if (!first) {
      event.preventDefault();
      this.element.focus();
    } else if (event.shiftKey && document.activeElement === first || !event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      (event.shiftKey ? last : first).focus();
    }
  }
}
