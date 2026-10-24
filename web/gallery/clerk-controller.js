import { Controller } from '../stimulus/stimulus.js';
import { clerkClient } from './clerk-client.js';
import { clerkAuthAppearance } from './clerk-appearance.js';

export default class extends Controller {
  static targets = ['mount', 'status'];
  static values = { mode: String, next: { type: String, default: '/' } };

  async connect() {
    try {
      const clerk = await clerkClient();
      if (!this.element.isConnected) return;
      this.clerk = clerk;
      this.statusTarget.textContent = '';
      if (this.modeValue === 'logout') {
        await clerk.signOut({ redirectUrl: '/auth/login' });
        window.location.replace('/auth/login');
      } else if (this.modeValue === 'user') {
        if (clerk.isSignedIn) clerk.mountUserButton(this.mountTarget);
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
      if (this.element.isConnected) this.statusTarget.textContent = error.message;
    }
  }

  disconnect() {
    if (!this.clerk) return;
    if (this.modeValue === 'user') this.clerk.unmountUserButton(this.mountTarget);
    else if (this.modeValue === 'signup') this.clerk.unmountSignUp(this.mountTarget);
    else if (this.modeValue === 'signin') this.clerk.unmountSignIn(this.mountTarget);
  }
}
