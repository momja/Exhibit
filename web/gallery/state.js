/* Artifact state inspector — the edit page's typed view of the state an
 * artifact wrote through localStorage (av-hg5f). Served from the app origin at
 * /assets/gallery/state.js, after edit.js, and reads the per-request globals
 * the page's inline bootstrap defines: TOKEN, ID, TITLE.
 *
 * Two rules shape everything below.
 *
 * 1. No raw-text editing, ever. State is a flat key/value map of strings, but
 *    artifacts almost always store JSON, so each value is parsed and rendered
 *    through the control its shape implies — a list as a list, a number as a
 *    number. A hand-typed JSON blob is precisely the corruption this panel
 *    exists to undo, so a value the form model can't represent renders
 *    read-only (and stays deletable) rather than falling back to a textarea.
 *
 * 2. Keys and values are artifact-controlled text rendered on the *app*
 *    origin. Every one of them reaches the DOM through createElement +
 *    textContent; nothing here interpolates them into markup. Same reasoning
 *    as edit.js's buildOriginRow (av-tux9).
 *
 * Nothing is written until Save: edits mutate an in-memory working copy, and
 * Cancel rebuilds that copy from the last server-confirmed serialization —
 * which is exactly what a page reload would show.
 */

(function () {
  const panel = document.getElementById('state-panel');
  if (!panel) return;

  const rowsEl = document.getElementById('state-rows');
  const statusEl = document.getElementById('state-status');
  const summaryEl = document.getElementById('state-summary-text');

  // Objects nest, but not without end: past this many levels the labelled-field
  // model stops being readable, and the value falls back to the read-only view.
  const MAX_OBJECT_DEPTH = 3;

  const TYPE_LABELS = {
    string: 'text',
    number: 'number',
    boolean: 'true/false',
    list: 'list',
    records: 'records',
    object: 'object',
  };

  let entries = [];   // working copy; see parseEntry
  let loaded = false; // state is fetched on first open, not with the page

  // --- type inference ---------------------------------------------------
  // describe() maps a parsed value onto the form model, or returns null for
  // "no control can represent this" — the read-only case (AC 3).

  function isPrimitive(v) {
    const t = typeof v;
    return t === 'string' || t === 'number' || t === 'boolean';
  }

  function isPlainObject(v) {
    return v !== null && typeof v === 'object' && !Array.isArray(v);
  }

  function describe(value, depth) {
    if (isPrimitive(value)) return { kind: typeof value };
    if (Array.isArray(value)) return describeArray(value, depth);
    if (isPlainObject(value)) return describeObject(value, depth);
    // null (and nothing else reaches here from JSON.parse): absence has no
    // control that could edit it into something.
    return null;
  }

  function describeArray(items, depth) {
    if (items.every(isPrimitive)) {
      // An empty list has no item to infer from; text is the type a user can
      // always retype into something else.
      return { kind: 'list', item: items.length ? typeof items[0] : 'string' };
    }
    const columns = uniformColumns(items);
    if (columns) return { kind: 'records', columns };
    return null;
  }

  // An array of objects renders as a repeater only when the objects agree on
  // their fields and every field is a primitive — a ragged array has no
  // columns to label, so it is read-only instead.
  function uniformColumns(items) {
    if (!items.length) return null;
    let columns = null;
    for (const item of items) {
      if (!isPlainObject(item)) return null;
      const keys = Object.keys(item);
      if (!keys.every(k => isPrimitive(item[k]))) return null;
      if (columns === null) {
        columns = keys.map(k => ({ key: k, type: typeof item[k] }));
      } else if (columns.length !== keys.length ||
                 !columns.every(c => Object.prototype.hasOwnProperty.call(item, c.key))) {
        return null;
      }
    }
    return columns;
  }

  function describeObject(obj, depth) {
    if (depth >= MAX_OBJECT_DEPTH) return null;
    const fields = [];
    for (const key of Object.keys(obj)) {
      const node = describe(obj[key], depth + 1);
      if (!node) return null; // one unrepresentable branch makes the whole value read-only
      fields.push({ key, node });
    }
    return { kind: 'object', fields };
  }

  // --- entries ----------------------------------------------------------

  // An entry is one (key, value) row's working state.
  //   value  — the parsed JSON value, or the raw string when it wasn't JSON
  //   json   — how to serialize it back: JSON.stringify vs verbatim
  //   saved  — the serialization the server holds, or null for a key added
  //            here; Save writes only where the current serialization differs,
  //            so an untouched value is a true no-op even if the stored text
  //            was formatted differently.
  //   deleted — pending row removal, applied by Save, forgotten by Cancel
  function parseEntry(key, raw) {
    let value, json = true;
    try {
      value = JSON.parse(raw);
    } catch (e) {
      // Not JSON — a plain string, which includes any legacy empty-string
      // tombstones left by the early removeItem that wrote "" instead of
      // deleting (av-ms3r, since fixed). They are indistinguishable from an
      // intentional "" and are shown as what they are: an empty text value
      // the user can delete.
      value = raw;
      json = false;
    }
    const entry = { key, value, json, saved: null, deleted: false };
    entry.saved = serialize(entry);
    return entry;
  }

  function serialize(entry) {
    return entry.json ? JSON.stringify(entry.value) : String(entry.value);
  }

  function isChanged(entry) {
    return entry.saved === null || serialize(entry) !== entry.saved;
  }

  function zeroFor(kind) {
    switch (kind) {
      case 'number': return 0;
      case 'boolean': return false;
      case 'list': return [];
      case 'object': return {};
      default: return '';
    }
  }

  // --- small DOM helpers ------------------------------------------------

  function el(tag, className) {
    const n = document.createElement(tag);
    if (className) n.className = className;
    return n;
  }

  function icon(name) {
    const i = el('i', 'ph ' + name);
    i.setAttribute('aria-hidden', 'true');
    return i;
  }

  function iconButton(glyph, label, className, onClick) {
    const b = el('button', className);
    b.type = 'button';
    b.title = label;
    b.setAttribute('aria-label', label);
    b.appendChild(icon(glyph));
    b.addEventListener('click', onClick);
    return b;
  }

  function moveButton(glyph, label, enabled, onClick) {
    const b = iconButton(glyph, label, 'btn btn-sm btn-sec state-move', onClick);
    b.disabled = !enabled;
    return b;
  }

  function addButton(label, onClick) {
    const b = el('button', 'btn btn-sm btn-sec state-inline-add');
    b.type = 'button';
    b.appendChild(icon('ph-plus'));
    b.appendChild(document.createTextNode(' ' + label));
    b.addEventListener('click', onClick);
    return b;
  }

  function hint(text) {
    const s = el('div', 'text-sm faint');
    s.textContent = text;
    return s;
  }

  function setStatus(text) {
    statusEl.textContent = text;
  }

  function markDirty() {
    setStatus('Unsaved changes');
  }

  // --- controls ---------------------------------------------------------
  // assign(v) hands a new primitive value to whatever holds this position;
  // restructure() re-renders the row after a change to the *shape* of a value
  // (an item added, a property removed), which is also what keeps the inferred
  // form model in step with the value it describes.

  function buildControl(value, node, depth, assign, restructure) {
    switch (node.kind) {
      case 'number': return numberInput(value, assign);
      case 'boolean': return checkbox(value, assign);
      case 'list': return buildList(value, node, depth, restructure);
      case 'records': return buildRecords(value, node, depth, restructure);
      case 'object': return buildObject(value, node, depth, restructure);
      default: return textInput(value, assign);
    }
  }

  function textInput(value, assign) {
    const inp = el('input', 'field state-input');
    inp.type = 'text';
    inp.value = value;
    inp.addEventListener('input', () => { assign(inp.value); markDirty(); });
    return inp;
  }

  function numberInput(value, assign) {
    const inp = el('input', 'field state-input');
    inp.type = 'number';
    inp.step = 'any';
    inp.value = String(value);
    // A cleared or half-typed field ("-", "1e") is not a number yet, so the
    // model keeps its last valid value rather than taking NaN — which
    // JSON.stringify would silently store as null. Blur restores the display.
    inp.addEventListener('input', () => {
      const n = Number(inp.value);
      if (inp.value === '' || Number.isNaN(n)) return;
      assign(n);
      markDirty();
    });
    inp.addEventListener('blur', () => {
      const n = Number(inp.value);
      if (inp.value === '' || Number.isNaN(n)) inp.value = String(value);
    });
    return inp;
  }

  function checkbox(value, assign) {
    const label = el('label', 'state-toggle');
    const box = document.createElement('input');
    box.type = 'checkbox';
    box.checked = !!value;
    const text = el('span', 'text-sm muted');
    text.textContent = box.checked ? 'true' : 'false';
    box.addEventListener('change', () => {
      assign(box.checked);
      text.textContent = box.checked ? 'true' : 'false';
      markDirty();
    });
    label.append(box, text);
    return label;
  }

  function buildList(items, node, depth, restructure) {
    const box = el('div', 'state-list');
    items.forEach((item, i) => {
      const row = el('div', 'state-list-row');
      row.appendChild(buildControl(item, { kind: typeof item }, depth + 1,
        v => { items[i] = v; }, restructure));
      row.appendChild(moveButton('ph-arrow-up', 'Move up', i > 0,
        () => { swap(items, i, i - 1); markDirty(); restructure(); }));
      row.appendChild(moveButton('ph-arrow-down', 'Move down', i < items.length - 1,
        () => { swap(items, i, i + 1); markDirty(); restructure(); }));
      row.appendChild(iconButton('ph-x', 'Remove item', 'btn btn-sm btn-sec',
        () => { items.splice(i, 1); markDirty(); restructure(); }));
      box.appendChild(row);
    });
    if (!items.length) box.appendChild(hint('empty list'));
    box.appendChild(addButton('Add item', () => {
      items.push(zeroFor(node.item));
      markDirty();
      restructure();
    }));
    return box;
  }

  function buildRecords(items, node, depth, restructure) {
    const box = el('div', 'state-records');
    items.forEach((record, i) => {
      const card = el('div', 'state-record');
      const head = el('div', 'state-record-head');
      const label = el('span', 'text-sm muted');
      label.textContent = '#' + (i + 1);
      head.append(label, el('span', 'spacer'),
        moveButton('ph-arrow-up', 'Move up', i > 0,
          () => { swap(items, i, i - 1); markDirty(); restructure(); }),
        moveButton('ph-arrow-down', 'Move down', i < items.length - 1,
          () => { swap(items, i, i + 1); markDirty(); restructure(); }),
        iconButton('ph-x', 'Remove record', 'btn btn-sm btn-sec',
          () => { items.splice(i, 1); markDirty(); restructure(); }));
      card.appendChild(head);
      node.columns.forEach(col => {
        const control = buildControl(record[col.key], { kind: typeof record[col.key] },
          depth + 1, v => { record[col.key] = v; }, restructure);
        card.appendChild(labelledField(col.key, control));
      });
      box.appendChild(card);
    });
    box.appendChild(addButton('Add record', () => {
      const blank = {};
      node.columns.forEach(col => { blank[col.key] = zeroFor(col.type); });
      items.push(blank);
      markDirty();
      restructure();
    }));
    return box;
  }

  function buildObject(obj, node, depth, restructure) {
    const box = el('div', 'state-object');
    node.fields.forEach(field => {
      const control = buildControl(obj[field.key], field.node, depth + 1,
        v => { obj[field.key] = v; }, restructure);
      const wrap = labelledField(field.key, control);
      wrap.appendChild(iconButton('ph-x', 'Remove property', 'btn btn-sm btn-sec',
        () => { delete obj[field.key]; markDirty(); restructure(); }));
      box.appendChild(wrap);
    });
    if (!node.fields.length) box.appendChild(hint('no properties'));
    box.appendChild(propertyAdder(obj, depth, restructure));
    return box;
  }

  function labelledField(name, control) {
    const wrap = el('div', 'state-field');
    const label = el('span', 'state-field-label');
    label.textContent = name;
    label.title = name;
    wrap.append(label, control);
    return wrap;
  }

  // The same key+type question the top-level add-key row asks, scoped to one
  // object — without it a freshly added object would be a dead end, since the
  // form model has no other way to give it a property.
  function propertyAdder(obj, depth, restructure) {
    const box = el('div', 'state-adder');
    const name = el('input', 'field state-input');
    name.type = 'text';
    name.placeholder = 'New property';
    const type = typeSelect(depth + 1);
    const add = () => {
      const key = name.value.trim();
      if (!key) { setStatus('Give the new property a name.'); return; }
      if (Object.prototype.hasOwnProperty.call(obj, key)) {
        setStatus('Property "' + key + '" already exists.');
        return;
      }
      obj[key] = zeroFor(type.value);
      markDirty();
      restructure();
    };
    name.addEventListener('keydown', e => {
      if (e.key === 'Enter') { e.preventDefault(); add(); }
    });
    box.append(name, type, addButton('Add property', add));
    return box;
  }

  function typeSelect(depth) {
    const sel = el('select', 'select state-type-select');
    const kinds = [['string', 'Text'], ['number', 'Number'], ['boolean', 'True/false'], ['list', 'List']];
    // Only offer a nested object while one would still render as fields.
    if (depth < MAX_OBJECT_DEPTH) kinds.push(['object', 'Object']);
    kinds.forEach(([value, label]) => {
      const opt = document.createElement('option');
      opt.value = value;
      opt.textContent = label;
      sel.appendChild(opt);
    });
    return sel;
  }

  function swap(items, a, b) {
    const t = items[a];
    items[a] = items[b];
    items[b] = t;
  }

  function readOnlyView(value) {
    const box = el('div', 'state-readonly');
    const note = el('div', 'text-sm muted');
    note.textContent = 'No form control fits this shape, so it is shown as stored. ' +
      'Delete it here, or let the artifact rewrite it.';
    const pre = document.createElement('pre');
    pre.textContent = JSON.stringify(value, null, 2);
    box.append(note, pre);
    return box;
  }

  // --- rows -------------------------------------------------------------

  function buildRow(entry) {
    const node = entry.json ? describe(entry.value, 0) : { kind: 'string' };
    const row = el('div', 'state-row' + (entry.deleted ? ' is-deleted' : ''));
    row.dataset.key = entry.key;

    const head = el('div', 'state-row-head');
    const key = el('code', 'state-key');
    key.textContent = entry.key;
    key.title = entry.key;
    const badge = el('span', 'badge state-type');
    badge.textContent = node ? TYPE_LABELS[node.kind] : 'read-only';
    head.append(key, badge, el('span', 'spacer'));

    if (entry.deleted) {
      const note = el('span', 'text-sm muted');
      note.textContent = 'deleted on save';
      head.append(note, iconButton('ph-arrow-counter-clockwise', 'Keep this key',
        'btn btn-sm btn-sec', () => { entry.deleted = false; render(); markDirty(); }));
    } else {
      head.appendChild(iconButton('ph-trash', 'Delete this key', 'btn btn-sm btn-sec', () => {
        // A key added in this session was never written, so dropping it needs
        // no server round-trip and leaves nothing to undo on Save.
        if (entry.saved === null) entries = entries.filter(e => e !== entry);
        else entry.deleted = true;
        render();
        markDirty();
      }));
    }
    row.appendChild(head);

    if (!entry.deleted) {
      const restructure = () => { row.replaceWith(buildRow(entry)); };
      row.appendChild(node
        ? buildControl(entry.value, node, 0, v => { entry.value = v; }, restructure)
        : readOnlyView(entry.value));
    }
    return row;
  }

  function render() {
    rowsEl.replaceChildren();
    if (!entries.length) rowsEl.appendChild(emptyNotice());
    else entries.forEach(entry => rowsEl.appendChild(buildRow(entry)));
    updateSummary();
  }

  function emptyNotice() {
    const box = el('div', 'state-empty');
    box.appendChild(icon('ph-database'));
    const text = document.createElement('span');
    text.textContent = 'This artifact has not stored anything yet. Add a key below to seed one by hand.';
    box.appendChild(text);
    return box;
  }

  function updateSummary() {
    if (!loaded) { summaryEl.textContent = 'Stored state'; return; }
    const live = entries.filter(e => !e.deleted).length;
    summaryEl.textContent = live
      ? 'Stored state · ' + live + (live === 1 ? ' key' : ' keys')
      : 'Stored state · empty';
  }

  // --- API --------------------------------------------------------------

  function api(method, path, body) {
    const headers = { 'Authorization': 'Bearer ' + TOKEN };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    return fetch(path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  }

  // Both delegate to state-api.js, the one definition of these URLs shared
  // with the storage-bridge listeners in detail.js and agent.js. State keys
  // are arbitrary artifact-chosen text, so the key travels as a query value
  // rather than a path segment — see that file for why a key of ".." made
  // this URL delete the artifact (av-hh1o).
  function stateURL() {
    return window.ExhibitState.url(ID);
  }

  function stateDeleteURL(key) {
    return window.ExhibitState.deleteURL(ID, key);
  }

  async function loadState() {
    setStatus('Loading…');
    let resp;
    try {
      resp = await api('GET', stateURL());
    } catch (err) {
      setStatus('✗ Could not load state: ' + err.message);
      return;
    }
    if (!resp.ok) {
      setStatus('✗ Could not load state: ' + resp.statusText);
      return;
    }
    const data = await resp.json().catch(() => ({}));
    entries = Object.keys(data).sort().map(key => parseEntry(key, data[key]));
    loaded = true;
    render();
    setStatus('');
  }

  async function saveState() {
    const doomed = entries.filter(e => e.deleted && e.saved !== null);
    const changed = entries.filter(e => !e.deleted && isChanged(e));
    if (!doomed.length && !changed.length) { setStatus('Nothing to save.'); return; }
    setStatus('Saving…');
    try {
      // Deletes first: a key removed and re-added in one session must end up
      // written, not deleted. Each write is its own request — a handful of
      // keys per artifact makes batching cost more clarity than it saves.
      for (const entry of doomed) {
        const resp = await api('DELETE', stateDeleteURL(entry.key));
        if (!resp.ok) throw new Error('deleting "' + entry.key + '": ' + resp.statusText);
      }
      for (const entry of changed) {
        const value = serialize(entry);
        const resp = await api('PUT', stateURL(), { key: entry.key, value });
        if (!resp.ok) throw new Error('saving "' + entry.key + '": ' + resp.statusText);
        entry.saved = value; // a retry after a later failure re-sends only the rest
      }
    } catch (err) {
      setStatus('✗ ' + err.message);
      render();
      return;
    }
    entries = entries.filter(e => !e.deleted);
    render();
    setStatus('✓ Saved — the artifact reads this on its next render.');
  }

  // Cancel rebuilds the working copy from what the server last confirmed, so
  // it lands exactly where a page reload would: pending deletes forgotten,
  // added keys dropped, edited values back to stored.
  function cancelEdits() {
    entries = entries.filter(e => e.saved !== null).map(e => parseEntry(e.key, e.saved));
    render();
    setStatus('Edits discarded.');
  }

  async function eraseAll() {
    const ok = window.confirm(
      'Erase all stored data for “' + TITLE + '”?\n\n' +
      'Every key this artifact saved is deleted from the server. State has no ' +
      'version history, so this cannot be undone.\n\n' +
      'The artifact itself, its network allowlist, and its capability approvals ' +
      'are not touched.');
    if (!ok) return;
    setStatus('Erasing…');
    let resp;
    try {
      resp = await api('DELETE', stateDeleteURL());
    } catch (err) {
      setStatus('✗ Erase failed: ' + err.message);
      return;
    }
    if (!resp.ok) { setStatus('✗ Erase failed: ' + resp.statusText); return; }
    entries = [];
    render();
    setStatus('✓ All stored data erased.');
  }

  // --- wiring -----------------------------------------------------------

  const addKeyInput = document.getElementById('state-add-key');
  const addTypeSelect = document.getElementById('state-add-type');

  function addKey() {
    const key = addKeyInput.value.trim();
    if (!key) { setStatus('Give the new key a name.'); return; }
    if (entries.some(e => e.key === key)) { setStatus('Key "' + key + '" already exists.'); return; }
    const kind = addTypeSelect.value;
    entries.push({
      key,
      value: zeroFor(kind),
      // A hand-seeded text key is stored verbatim, so the artifact reads back
      // what was typed rather than a quoted JSON string.
      json: kind !== 'string',
      saved: null,
      deleted: false,
    });
    addKeyInput.value = '';
    render();
    markDirty();
  }

  document.getElementById('state-add-btn').addEventListener('click', addKey);
  addKeyInput.addEventListener('keydown', e => {
    if (e.key === 'Enter') { e.preventDefault(); addKey(); }
  });
  document.getElementById('state-save').addEventListener('click', saveState);
  document.getElementById('state-cancel').addEventListener('click', cancelEdits);
  document.getElementById('state-erase').addEventListener('click', eraseAll);

  // State is cold data no other part of the edit page needs, so it is fetched
  // on first open rather than inlined with the page.
  panel.addEventListener('toggle', () => {
    if (panel.open && !loaded) loadState();
  });
  // Arriving at #state-panel opens the panel before this script runs, which
  // fires no toggle event.
  if (panel.open) loadState();
})();
