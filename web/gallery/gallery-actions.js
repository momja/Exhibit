// Delegation keeps menus working after gallery searches and appended pages.
// Resolve the open menu each time: htmx can replace any card between events.
export function initializeCardActions(root) {
  const openActions = () => root.querySelector('[data-card-actions].is-open');

  function setOpen(actions, open) {
    actions.classList.toggle('is-open', open);
    actions.querySelector('[data-card-actions-trigger]').setAttribute('aria-expanded', String(open));
    actions.querySelector('.card-actions-menu').hidden = !open;
  }

  function close() {
    const actions = openActions();
    if (actions) setOpen(actions, false);
  }

  function open(actions, last = false) {
    close();
    setOpen(actions, true);
    const items = actions.querySelectorAll('.card-actions-menu a, .card-actions-menu button');
    (last ? items[items.length - 1] : items[0])?.focus();
  }

  root.addEventListener('click', (event) => {
    const trigger = event.target.closest('[data-card-actions-trigger]');
    if (trigger) {
      const actions = trigger.closest('[data-card-actions]');
      if (actions === openActions()) close();
      else open(actions);
    } else if (event.target.closest('.card-actions-menu a, .card-actions-menu button')) {
      close();
    }
  });

  document.addEventListener('click', (event) => {
    const actions = openActions();
    if (actions && !actions.contains(event.target)) close();
  });
  document.addEventListener('focusout', (event) => {
    const actions = openActions();
    // Safari can report no destination when tapping an action. Let its click
    // complete; only a known destination outside the menu dismisses here.
    if (actions && event.relatedTarget && !actions.contains(event.relatedTarget)) close();
  });
  document.addEventListener('keydown', (event) => {
    const actions = openActions();
    if (event.key === 'Escape' && actions) {
      event.preventDefault();
      close();
      actions.querySelector('[data-card-actions-trigger]').focus();
      return;
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    const targetActions = event.target.closest('[data-card-actions]');
    if (!targetActions || !root.contains(targetActions)) return;
    const last = event.key === 'ArrowUp' || event.key === 'End';
    if (!actions || actions !== targetActions) {
      if (!event.target.closest('[data-card-actions-trigger]')) return;
      event.preventDefault();
      open(targetActions, last);
      return;
    }
    event.preventDefault();
    const items = [...actions.querySelectorAll('.card-actions-menu a, .card-actions-menu button')];
    const index = items.indexOf(document.activeElement);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1
      : (index + (last ? -1 : 1) + items.length) % items.length;
    items[next]?.focus();
  });
}
