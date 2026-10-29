// Keep Clerk's Profile and Security intact. Exhibit settings have their own
// page, reached from this same account menu rather than a second header button.
export function clerkUserButtonOptions() {
  return {
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
