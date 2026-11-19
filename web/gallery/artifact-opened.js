import { viewerFetch } from './api-module.js';

// Kept separate from the viewer's state bridge: opening is personal metadata,
// never a write to an artifact's saved data or its current version.
const id = document.querySelector('[data-artifact-opened]')?.dataset.artifactOpened;
if (id) viewerFetch('/api/artifacts/' + encodeURIComponent(id) + '/opened', {method: 'POST', keepalive: true}).catch(() => {});
