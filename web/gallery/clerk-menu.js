// Keep Clerk's Profile and Security intact. Exhibit settings have their own
// page, reached from this same account menu rather than a second header button.
export function clerkUserButtonOptions() {
  return {
    appearance: { elements: {
      userButtonTrigger: { width: 'var(--header-control-size)', height: 'var(--header-control-size)', padding: 0, boxSizing: 'border-box' },
    } },
    customMenuItems: [{
      label: 'Settings',
      href: '/settings',
      mountIcon(element) {
        const glyph = document.createElement('i');
        glyph.className = 'ph ph-gear';
        glyph.setAttribute('aria-hidden', 'true');
        element.append(glyph);
      },
      unmountIcon(element) { element.replaceChildren(); },
    }],
  };
}
