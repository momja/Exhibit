// Replace only documented icon slots. Retain the SVG's dimensions and ARIA
// attributes; a currentColor mask follows light/dark themes and button states.
function phosphorIcon(name) {
  return {
    backgroundColor: 'currentColor',
    mask: `url("/assets/phosphor/${name}.svg") center / contain no-repeat`,
    WebkitMask: `url("/assets/phosphor/${name}.svg") center / contain no-repeat`,
    '& > *': { visibility: 'hidden' },
  };
}

// Map Clerk's supported appearance API to Exhibit tokens. CSS variables keep
// open account dialogs in sync when the system changes light/dark mode.
export const clerkAppearance = {
  variables: {
    colorPrimary: 'var(--color-accent)',
    colorPrimaryForeground: 'var(--color-on-accent)',
    colorBackground: 'var(--color-panel)',
    colorForeground: 'var(--color-ink)',
    colorMuted: 'var(--color-subtle)',
    colorMutedForeground: 'var(--color-ink-mid)',
    colorInput: 'var(--color-panel)',
    colorInputForeground: 'var(--color-ink)',
    colorBorder: 'var(--color-border)',
    colorDanger: 'var(--color-danger)',
    colorSuccess: 'var(--color-success)',
    colorWarning: 'var(--color-warn-strong)',
    colorRing: 'var(--color-accent-tint-border)',
    fontFamily: 'system-ui, sans-serif',
    fontFamilyButtons: 'system-ui, sans-serif',
    fontSize: '14px',
    borderRadius: 'var(--radius-md)',
  },
  elements: {
    navbarButtonIcon__account: phosphorIcon('user-circle'),
    navbarButtonIcon__security: phosphorIcon('shield-check'),
    userButtonPopoverActionButtonIcon__manageAccount: phosphorIcon('gear'),
    userButtonPopoverActionButtonIcon__signOut: phosphorIcon('sign-out'),
    userButtonPopoverActionButtonIcon__signOutAll: phosphorIcon('sign-out'),
    userButtonPopoverActionButtonIcon__addAccount: phosphorIcon('plus'),
    passkeyIcon: phosphorIcon('fingerprint'),
    buttonArrowIcon: phosphorIcon('arrow-right'),
    identityPreviewEditButtonIcon: phosphorIcon('pencil-simple'),
    formFieldInputShowPasswordIcon: phosphorIcon('eye'),
    formFieldInputGroup: {
      '&:has(input[type="text"]) .cl-formFieldInputShowPasswordIcon': phosphorIcon('eye-slash'),
    },
    cardBox: { borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-card)', border: '1px solid var(--color-hairline)' },
    card: { background: 'var(--color-panel)', boxShadow: 'none', padding: 'var(--space-8)' },
    headerTitle: { color: 'var(--color-ink)', fontSize: '20px', fontWeight: 600, letterSpacing: '-0.01em' },
    headerSubtitle: { color: 'var(--color-ink-mid)', fontSize: '14px' },
    formFieldLabel: { color: 'var(--color-ink-soft)', fontSize: '13px', fontWeight: 500 },
    formFieldInput: { border: '1px solid var(--color-border)', fontSize: 'var(--field-font-size)', padding: 'var(--space-3) var(--space-4)', boxShadow: 'none' },
    formButtonPrimary: {
      background: 'var(--brand-blue)', color: 'var(--color-on-accent)',
      boxShadow: 'none', border: 'none', fontWeight: 500,
      '&:hover': { background: 'var(--brand-blue-hover)' },
    },
    socialButtonsIconButton: { boxShadow: 'none', border: '1px solid var(--color-border)', background: 'var(--color-panel)' },
    socialButtonsBlockButton: { boxShadow: 'none', border: '1px solid var(--color-border)', background: 'var(--color-panel)' },
    footer: { background: 'var(--color-subtle)', backgroundImage: 'none', borderTop: '1px solid var(--color-hairline)' },
    footerActionLink: { color: 'var(--brand-blue)', fontWeight: 500 },
    userButtonPopoverCard: { borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-modal)', border: '1px solid var(--color-border)' },
  },
};

// Width belongs to the standalone login form, not the navigation's user button.
export const clerkAuthAppearance = {
  elements: {
    rootBox: { width: '100%', maxWidth: '100%' },
    cardBox: { width: '100%', maxWidth: '100%' },
  },
};
