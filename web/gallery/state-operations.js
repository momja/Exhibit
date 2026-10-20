// A replacement must wait for every request in an inspector save, and the
// inspector must finish reloading before another mutation can start.
let busy = false;

export async function withStateOperation(operation) {
  if (busy) return;
  busy = true;
  const controls = document.getElementById('state-controls');
  if (controls) controls.disabled = true;
  try {
    return await operation();
  } finally {
    if (controls) controls.disabled = false;
    busy = false;
  }
}

export async function notifyStateImported() {
  const refreshes = [];
  document.body.dispatchEvent(new CustomEvent('exhibit:state-imported', {
    detail: { waitUntil: promise => refreshes.push(promise) },
  }));
  await Promise.all(refreshes);
}
