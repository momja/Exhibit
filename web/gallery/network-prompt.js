/* The runtime network-permission prompt, shared by every page that embeds an
 * artifact — served from the app origin at /assets/gallery/network-prompt.js
 * and loaded by detail.tmpl and agent.tmpl.
 *
 * One definition on purpose (av-kmwj, av-6xvs), for the reason state-api.js
 * gives: this began as ~90 lines inside detail.js, and the agent page — which
 * embeds the same render document behind the same sandbox, on the same app
 * origin — simply had none of it, so an artifact reaching an unapproved origin
 * while being built there failed silently. Copying the block would have made
 * that two places to fix the next queueing or dedupe bug in.
 *
 * What it owns: the message listener, the report queue, the dialog wiring, the
 * host-ready handshake, the decision write, and the reload. What it does not
 * own is anything page-specific — the caller supplies those through the
 * options below, because the two pages differ in every one of them:
 *
 *   frame()      - the artifact's iframe, resolved on each use. The agent
 *                  page's is replaced by an htmx swap after every save, so a
 *                  cached reference goes stale (see agent.js).
 *   artifactId() - the id, resolved on each use. On the agent page there is no
 *                  artifact until one is created, and it can change afterwards.
 *   reload()     - how to refetch the render document so a widened CSP applies.
 *                  A CSP is a response header fixed at load, so a new policy
 *                  needs a new document, and each page reaches one differently.
 *   readOnly()   - optional; true refuses the prompt outright.
 *
 * The dialog markup is the shared `networkPrompt` template partial, so the ids
 * below are the same wherever it renders. A page that omits the partial gets a
 * no-op install rather than a thrown error, which is what keeps this safe to
 * load from a page that has not adopted it yet.
 */
(function () {
  // Every dismissal path and every answer runs through one install, so a page
  // that calls install() twice would get two listeners racing over one dialog.
  let installed = false;

  window.ExhibitNetworkPrompt = {
    install: function (opts) {
      const modal = document.getElementById('net-modal');
      if (!modal || installed) return;
      installed = true;

      const frame = opts.frame;
      const artifactId = opts.artifactId;
      const reload = opts.reload;
      const readOnly = opts.readOnly || function () { return false; };
      // Where progress and failure are reported. A function that takes text
      // rather than an element: the detail page has a status span, the agent
      // page writes into its transcript, and neither should have to pretend to
      // be the other. Omitted means the prompt reports nothing, which is worse
      // than either but never wrong.
      const report = opts.report || function () {};

      let pending = null;
      const queue = [];

      // Advances to the next queued report, or closes the dialog when the
      // queue is empty. Every dismissal path goes through here, so "next" and
      // "close" are one behaviour rather than two that can disagree.
      function showNext() {
        pending = queue.shift() || null;
        if (!pending) {
          modal.hidden = true;
          const f = frame();
          if (f) f.focus();
          return;
        }
        // textContent, never innerHTML: the origin comes out of the artifact's
        // own blocked request and must not be interpreted as markup here.
        document.getElementById('net-origin').textContent = pending.origin;
        document.getElementById('net-directive').textContent =
          pending.directive ? 'Blocked by ' + pending.directive : '';
        modal.hidden = false;
        // Land on the safe answer, the way the link prompt does.
        document.getElementById('net-once').focus();
      }

      // Records one origin's decision through the per-origin route. Not PATCH:
      // neither page holds a working copy of the allowlist, and restating one
      // would overwrite decisions made on the edit page since it loaded.
      async function decide(origin, decision) {
        const id = artifactId();
        if (!id) return false;
        const r = await apiFetch('/api/artifacts/' + encodeURIComponent(id) + '/origins', {
          method: 'POST',
          body: JSON.stringify({ origin: origin, decision: decision, source: 'runtime' })
        }).catch(function () { return null; });
        if (!r || !r.ok) { report('✗ Failed to save the decision for ' + origin); return false; }
        return true;
      }

      window.addEventListener('message', function (e) {
        const d = e.data;
        if (!d || d.__avNetwork !== true) return;
        const id = artifactId();
        if (!id || d.artifactId !== id) return;
        const f = frame();
        if (!f || e.source !== f.contentWindow) return;
        // A read-only visitor cannot record either answer — apiFetch would
        // refuse the write — so prompting them would be asking a question with
        // no answer. The render preamble already stays silent for an anonymous
        // render; this is the same fact on the side that owns the dialog.
        if (readOnly()) return;
        queue.push({ origin: String(d.origin || ''), directive: String(d.directive || '') });
        if (!pending) showNext();
      });

      document.getElementById('net-once').addEventListener('click', showNext);
      modal.addEventListener('click', function (e) { if (e.target === modal) showNext(); });
      document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape' && !modal.hidden) showNext();
      });

      document.getElementById('net-never').addEventListener('click', async function () {
        const req = pending;
        if (!req) { showNext(); return; }
        // A failed write leaves the prompt up rather than pretending the origin
        // was refused: the next load would ask again, having said it would not.
        if (!(await decide(req.origin, 'block'))) return;
        // Escape, the backdrop and Block once stay live while that write is in
        // flight, and each one promotes the next queued report. Advancing again
        // here would drop the promoted origin without ever showing it, and the
        // preamble reports each origin only once per load, so it would be gone
        // for good. Only this request's own dismissal is ours to make.
        if (pending === req) showNext();
      });

      document.getElementById('net-allow').addEventListener('click', async function () {
        const req = pending;
        if (!req) { showNext(); return; }
        if (!(await decide(req.origin, 'allow'))) return;
        modal.hidden = true;
        pending = null;
        // The reload re-runs the artifact under the new CSP, so anything still
        // blocked reports itself again. Drop the queue rather than ask twice
        // about origins the fresh load is about to raise on its own.
        queue.length = 0;
        report('Applying…');
        reload();
      });
    },

    // Tells a frame the host is listening, so the preamble flushes whatever it
    // buffered — CSP-violation reports and the capability diagnostic both fire
    // at frame load, before a page has attached its listeners, and a one-shot
    // postMessage there is simply lost.
    //
    // Announced twice, and both are needed. On every `load`, for frames that
    // load after this runs. And once immediately, because a frame that finished
    // loading *before* this ran has already fired its `load` event and will not
    // fire another — that ordering decides whether a prompt appears at all, and
    // it varies with cache state. A duplicate ping is harmless: the preamble
    // flushes its queue once.
    //
    // targetOrigin is '*' because the frame's origin is opaque; the preamble
    // validates the ping came from our app origin.
    announceTo: function (frame) {
      if (!frame) return;
      function announce() {
        if (frame.contentWindow) frame.contentWindow.postMessage({ __avHostReady: true }, '*');
      }
      frame.addEventListener('load', announce);
      announce();
    }
  };
})();
