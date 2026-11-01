// A connection's listeners belong to that connection, including document
// listeners and callbacks into elements removed by a server-rendered swap.
export function eventScope() {
  const removals = [];
  return {
    on(target, name, callback, options) {
      target.addEventListener(name, callback, options);
      removals.push(() => target.removeEventListener(name, callback, options));
    },
    disconnect() {
      removals.splice(0).forEach(remove => remove());
    },
  };
}
