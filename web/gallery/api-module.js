// Bridge module callers to the shared credential client while classic pages
// still load api.js. Keep its read-only and authentication rules in one place.
export function apiFetch(path, options) {
  return window.apiFetch(path, options);
}
