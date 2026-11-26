import { gallerySnapshotMaxAge, widgetSource } from './gallery-space-cache.js';

function widgetKey(well) {
  const frame = well.querySelector('template.card-widget-frame-template')?.content.firstElementChild;
  if (!frame || !well.dataset.widgetVersion) return;
  return JSON.stringify([well.dataset.widgetReload, well.dataset.widgetVersion,
    frame.getAttribute('sandbox'), widgetSource(frame.getAttribute('srcdoc') ?? frame.getAttribute('src'))]);
}

function recentlyMounted(frame) {
  return performance.now() - Number(frame.dataset.galleryMountedAt) < gallerySnapshotMaxAge;
}

// Park only the live wells the viewport observer has already mounted. Moving
// them with moveBefore keeps their iframe documents alive. Ordinary insertion
// would reload them, so browsers without this API use the existing lazy mount.
export function galleryWidgetTabs(shell) {
  if (typeof shell.moveBefore !== 'function') return;
  const tabs = new Map();

  function discard(path) {
    const tab = tabs.get(path);
    if (!tab) return;
    clearTimeout(tab.expiry);
    for (const frame of tab.parking.querySelectorAll('.card-widget-frame')) {
      frame.dispatchEvent(new Event('exhibit:widget-dispose'));
    }
    tab.parking.remove();
    tabs.delete(path);
  }

  return {
    park(path, space) {
      discard(path);
      const mounted = space.querySelectorAll('[data-gallery-widget].has-frame:not(.widget-failed)');
      if (!mounted.length) return;
      const parking = document.createElement('div');
      parking.hidden = true;
      shell.append(parking);
      const wells = new Map();
      for (const well of mounted) {
        const key = widgetKey(well);
        const frame = well.querySelector('.card-widget-frame');
        if (!key || !frame || !recentlyMounted(frame) || wells.has(key)) continue;
        // The health watcher marks the surrounding well if a parked frame fails.
        const holder = well.cloneNode(false);
        parking.append(holder);
        holder.moveBefore(frame, null);
        wells.set(key, holder);
      }
      if (wells.size) {
        tabs.set(path, { parking, wells, expiry: setTimeout(() => discard(path), gallerySnapshotMaxAge) });
      } else {
        parking.remove();
      }
    },

    restore(path, space) {
      const tab = tabs.get(path);
      if (!tab) return;
      for (const well of space.querySelectorAll('[data-gallery-widget]')) {
        const previous = tab.wells.get(widgetKey(well));
        const frame = previous?.querySelector('.card-widget-frame');
        if (!frame || !recentlyMounted(frame) || previous.classList.contains('widget-failed')) continue;
        const template = well.querySelector('template.card-widget-frame-template');
        frame.title = template.content.firstElementChild.title;
        well.moveBefore(frame, null);
        well.classList.add('has-frame');
      }
      discard(path);
    },

    discard,
    clear() {
      for (const path of tabs.keys()) discard(path);
    },
  };
}
