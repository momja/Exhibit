// Bridge module callers to the shared credential client while classic pages
// still load api.js. Keep its read-only and authentication rules in one place.
export function apiFetch(path, options) {
  return window.apiFetch(path, options);
}

export function apiEventSource(path, ticket) {
  return window.apiEventSource(path, ticket);
}

// Viewer resources belong to the signed-in person even on a received artifact.
// Keep this narrow: it does not confer permission to edit somebody else's file.
export function viewerFetch(path, options = {}) {
  if (!/^\/api\/(?:connections(?:\/|$)|artifacts\/[^/]+\/(?:relay(?:-grant|-schedules)?|connections|connection-data|agent-connections|connection-requests)(?:\/|$))/.test(path)) {
    throw new Error('This is not a viewer resource.');
  }
  const headers = new Headers(options.headers);
  headers.set('Accept', 'application/json');
  if (options.body !== undefined) headers.set('Content-Type', 'application/json');
  if (typeof TOKEN === 'string' && TOKEN) headers.set('Authorization', 'Bearer ' + TOKEN);
  return fetch(path, { ...options, headers, credentials: 'same-origin' });
}
