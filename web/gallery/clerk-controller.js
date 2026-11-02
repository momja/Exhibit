import { Controller } from '../stimulus/stimulus.js';
import { clerkClient, clerkClientAfterLoad } from './clerk-client.js';
import { clerkAuthAppearance } from './clerk-appearance.js';
import { clerkUserButtonOptions } from './clerk-menu.js';

export default class extends Controller {
  static targets = ['mount', 'status', 'recovery'];
  static values = { mode: String, next: { type: String, default: '/' } };

  async connect() {
    if (this.modeValue === 'manage') return;
    this.connection = new AbortController();
    const { signal } = this.connection;
    if (this.modeValue === 'user') this.setAccountState('loading', 'Loading your account…');
    try {
      // The account button rides on an ordinary page; the other modes are
      // login pages, where nothing else is waiting.
      const clerk = await (this.modeValue === 'user' ? clerkClientAfterLoad() : clerkClient());
      if (signal.aborted) return;
      this.clerk = clerk;
      if (this.modeValue !== 'user') this.statusTarget.textContent = '';
      if (this.hasRecoveryTarget) this.recoveryTarget.hidden = true;
      if (this.modeValue === 'logout') {
        await clerk.signOut({ redirectUrl: '/auth/login' });
        window.location.replace('/auth/login');
      } else if (this.modeValue === 'user') {
        if (!clerk.isSignedIn) throw new Error('Your account session is unavailable. Reload this page to sign in again.');
        await this.loadUserButton(clerk, signal);
        if (!signal.aborted) this.setAccountState('ready', 'Account menu ready.');
      } else if (clerk.isSignedIn) {
        await clerk.session.getToken();
        const response = await fetch('/auth/clerk/ready', { credentials: 'same-origin' });
        if (!response.ok) throw new Error('Your account cannot access Exhibit. Contact the administrator.');
        window.location.replace(this.nextValue);
      } else {
        const options = { routing: 'hash', forceRedirectUrl: this.nextValue, appearance: clerkAuthAppearance };
        if (this.modeValue === 'signup') clerk.mountSignUp(this.mountTarget, options);
        else clerk.mountSignIn(this.mountTarget, options);
      }
    } catch (error) {
      if (!signal.aborted) {
        if (this.modeValue === 'user') {
          this.unmountUserButton();
          this.setAccountState('error', error.message);
        } else this.statusTarget.textContent = error.message;
        if (this.hasRecoveryTarget) this.recoveryTarget.hidden = false;
      }
    }
  }

  setAccountState(state, message) {
    this.element.dataset.clerkState = state;
    this.element.setAttribute('aria-busy', String(state === 'loading'));
    this.mountTarget.inert = state !== 'ready';
    this.mountTarget.setAttribute('aria-hidden', String(state !== 'ready'));
    this.statusTarget.classList.toggle('sr-only', state !== 'error');
    this.statusTarget.textContent = message;
    this.recoveryTarget.hidden = state !== 'error';
  }

  loadUserButton(clerk, signal) {
    const mount = this.mountTarget;
    // mountUserButton returns void; React can render the trigger later. Watch
    // the supported Clerk element class, not its wrappers or loading content.
    return new Promise((resolve, reject) => {
      let finished = false;
      const finish = error => {
        if (finished) return;
        finished = true;
        observer.disconnect();
        clearTimeout(timer);
        signal.removeEventListener('abort', cancel);
        if (error) reject(error);
        else resolve();
      };
      const cancel = () => finish();
      const rendered = () => {
        if (mount.querySelector('button.cl-userButtonTrigger:not(:disabled)')) finish();
      };
      const observer = new MutationObserver(rendered);
      const timer = setTimeout(() => finish(new Error('Your account menu did not finish loading. Reload this page to try again.')), 15000);
      observer.observe(mount, { childList: true, subtree: true, attributes: true, attributeFilter: ['disabled'] });
      signal.addEventListener('abort', cancel, { once: true });
      // The server enforces Exhibit-owned deletion. Refresh persisted client
      // permissions before mounting, within the same bounded loading state.
      Promise.resolve().then(() => clerk.user.reload()).then(() => {
        if (finished) return;
        this.userButtonMounted = true;
        clerk.mountUserButton(mount, clerkUserButtonOptions());
        rendered();
      }).catch(finish);
    });
  }

  unmountUserButton() {
    if (this.userButtonMounted) this.clerk.unmountUserButton(this.mountTarget);
    this.userButtonMounted = false;
  }

  async openProfile() {
    this.statusTarget.textContent = 'Loading your profile…';
    try {
      const clerk = await clerkClient();
      if (!this.element.isConnected) return;
      this.statusTarget.textContent = '';
      clerk.openUserProfile();
    } catch (error) {
      if (this.element.isConnected) this.statusTarget.textContent = error.message;
    }
  }

  retry() {
    window.location.reload();
  }

  disconnect() {
    this.connection?.abort();
    if (!this.clerk) return;
    if (this.modeValue === 'user') this.unmountUserButton();
    else if (this.modeValue === 'signup') this.clerk.unmountSignUp(this.mountTarget);
    else if (this.modeValue === 'signin') this.clerk.unmountSignIn(this.mountTarget);
  }
}
