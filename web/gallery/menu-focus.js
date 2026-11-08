// Shared movement for menus whose owners control opening, closing and placement.
export function moveMenuFocus(event, items) {
  const { key } = event;
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(key)) return false;
  event.preventDefault();
  const current = items.indexOf(document.activeElement);
  const next = key === 'Home' ? 0 : key === 'End' ? items.length - 1
    : (current + (key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
  items[next]?.focus();
  return true;
}
