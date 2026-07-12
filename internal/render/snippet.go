package render

import "fmt"

// snippetTemplate is the element-picker script injected alongside the storage
// shim (Exh-edjk). It sits completely idle until the embedding host page —
// verified by origin — posts {__exSnippet:'activate'}. Then it turns the
// document into a hover-highlight element picker: clicking an element captures
// a structural descriptor (selector, tag/id/classes, trimmed outerHTML) plus a
// best-effort screenshot rasterized inside the sandbox via SVG foreignObject
// (the opaque-origin iframe can screenshot its own DOM; the host cannot), and
// posts both back to the host pinned to the app origin. The host attaches them
// to the next agent prompt as multimodal context.
const snippetTemplate = `<script>
(function() {
  var APP_ORIGIN = %q;
  var active = false;
  var overlay = null, label = null, current = null;

  function ensureOverlay() {
    if (overlay) return;
    overlay = document.createElement('div');
    overlay.style.cssText = 'position:fixed;pointer-events:none;z-index:2147483646;border:2px solid #2563eb;background:rgba(37,99,235,.12);border-radius:2px;display:none';
    label = document.createElement('div');
    label.style.cssText = 'position:fixed;pointer-events:none;z-index:2147483647;background:#2563eb;color:#fff;font:11px/1.6 system-ui,sans-serif;padding:1px 7px;border-radius:3px;display:none;max-width:60vw;overflow:hidden;text-overflow:ellipsis;white-space:nowrap';
    document.documentElement.appendChild(overlay);
    document.documentElement.appendChild(label);
  }

  function shortName(elm) {
    var s = elm.tagName.toLowerCase();
    if (elm.id) s += '#' + elm.id;
    else if (elm.classList.length) s += '.' + Array.prototype.slice.call(elm.classList, 0, 2).join('.');
    return s;
  }

  function cssPath(elm) {
    var parts = [];
    var node = elm;
    while (node && node.nodeType === 1 && parts.length < 6 && node !== document.documentElement) {
      if (node.id) { parts.unshift('#' + node.id); break; }
      var part = node.tagName.toLowerCase();
      var parent = node.parentElement;
      if (parent) {
        var same = Array.prototype.filter.call(parent.children, function(c) { return c.tagName === node.tagName; });
        if (same.length > 1) part += ':nth-of-type(' + (same.indexOf(node) + 1) + ')';
      }
      parts.unshift(part);
      node = parent;
    }
    return parts.join(' > ');
  }

  function descriptorFor(elm) {
    var r = elm.getBoundingClientRect();
    var outer = elm.outerHTML || '';
    if (outer.length > 2000) outer = outer.slice(0, 2000) + '…';
    var text = (elm.textContent || '').trim().replace(/\s+/g, ' ');
    if (text.length > 200) text = text.slice(0, 200) + '…';
    return {
      selector: cssPath(elm),
      tag: elm.tagName.toLowerCase(),
      id: elm.id || '',
      classes: Array.prototype.slice.call(elm.classList),
      text: text,
      outerHTML: outer,
      rect: { width: r.width, height: r.height }
    };
  }

  // Rasterize the element inside the sandbox: clone it, freeze computed
  // styles inline (bounded), wrap in an SVG foreignObject, draw to canvas.
  // Best-effort — on any failure the capture proceeds without an image.
  function rasterize(elm, done) {
    try {
      var MAX_NODES = 300;
      var rect = elm.getBoundingClientRect();
      var w = Math.max(1, Math.ceil(rect.width)), h = Math.max(1, Math.ceil(rect.height));
      if (w > 2000 || h > 2000) { done(null); return; }
      var clone = elm.cloneNode(true);
      var srcWalk = [elm], dstWalk = [clone], count = 0;
      while (srcWalk.length && count < MAX_NODES) {
        var s = srcWalk.pop(), d = dstWalk.pop();
        count++;
        if (d.tagName === 'SCRIPT') { d.textContent = ''; continue; }
        var cs = getComputedStyle(s);
        var styleStr = '';
        for (var i = 0; i < cs.length; i++) {
          var p = cs[i];
          styleStr += p + ':' + cs.getPropertyValue(p).replace(/"/g, "'") + ';';
        }
        d.setAttribute('style', styleStr);
        for (var c = 0; c < s.children.length && c < d.children.length; c++) {
          srcWalk.push(s.children[c]);
          dstWalk.push(d.children[c]);
        }
      }
      var xml = new XMLSerializer().serializeToString(clone);
      var svg = '<svg xmlns="http://www.w3.org/2000/svg" width="' + w + '" height="' + h + '">' +
        '<foreignObject width="100%%" height="100%%"><div xmlns="http://www.w3.org/1999/xhtml">' +
        xml + '</div></foreignObject></svg>';
      var img = new Image();
      var scale = Math.min(2, window.devicePixelRatio || 1);
      img.onload = function() {
        try {
          var canvas = document.createElement('canvas');
          canvas.width = w * scale;
          canvas.height = h * scale;
          var ctx = canvas.getContext('2d');
          ctx.scale(scale, scale);
          ctx.drawImage(img, 0, 0);
          var dataUrl = canvas.toDataURL('image/png');
          done({ data: dataUrl.split(',')[1], mimeType: 'image/png' });
        } catch (e) { done(null); }
      };
      img.onerror = function() { done(null); };
      img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
    } catch (e) { done(null); }
  }

  function post(msg) {
    if (window.parent === window) return;
    window.parent.postMessage(msg, APP_ORIGIN);
  }

  function onMove(e) {
    var elm = document.elementFromPoint(e.clientX, e.clientY);
    if (!elm || elm === overlay || elm === label) return;
    current = elm;
    var r = elm.getBoundingClientRect();
    overlay.style.display = 'block';
    overlay.style.left = r.left + 'px';
    overlay.style.top = r.top + 'px';
    overlay.style.width = r.width + 'px';
    overlay.style.height = r.height + 'px';
    label.style.display = 'block';
    label.style.left = Math.max(4, r.left) + 'px';
    label.style.top = Math.max(4, r.top - 22) + 'px';
    label.textContent = shortName(elm);
  }

  function onClick(e) {
    e.preventDefault();
    e.stopPropagation();
    if (!current) return;
    var target = current;
    var desc = descriptorFor(target);
    deactivate(false);
    rasterize(target, function(image) {
      post({ __exSnippet: 'captured', descriptor: desc, image: image });
    });
  }

  function onKey(e) {
    if (e.key === 'Escape') {
      deactivate(false);
      post({ __exSnippet: 'cancelled' });
    }
  }

  function activate() {
    if (active) return;
    active = true;
    ensureOverlay();
    document.documentElement.style.cursor = 'crosshair';
    document.addEventListener('mousemove', onMove, true);
    document.addEventListener('click', onClick, true);
    document.addEventListener('keydown', onKey, true);
  }

  function deactivate(notify) {
    if (!active) return;
    active = false;
    document.documentElement.style.cursor = '';
    document.removeEventListener('mousemove', onMove, true);
    document.removeEventListener('click', onClick, true);
    document.removeEventListener('keydown', onKey, true);
    if (overlay) { overlay.style.display = 'none'; label.style.display = 'none'; }
    if (notify) post({ __exSnippet: 'cancelled' });
  }

  window.addEventListener('message', function(e) {
    // Only the embedding app-origin host may drive snippet mode.
    if (e.origin !== APP_ORIGIN || !e.data || !e.data.__exSnippet) return;
    if (e.data.__exSnippet === 'activate') activate();
    else if (e.data.__exSnippet === 'deactivate') deactivate(false);
  });
})();
</script>`

// snippetScript renders the snippet-mode script for the given app origin.
func snippetScript(appOrigin string) string {
	return fmt.Sprintf(snippetTemplate, appOrigin)
}
