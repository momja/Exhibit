package api

import (
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"
)

// A completed save from a removed dialog must not dismiss a new decision on
// the same reconnected element. Exercise the real section coordinator/handle.
func TestEditorModalLifecycleBrowser(t *testing.T) {
	ro := newTestRouter(t)
	id := createTestArtifact(t, ro, "Reconnected editor")
	app := httptest.NewServer(ro)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	b := newModalBrowser(t, 1280)
	b.run(chromedp.Navigate(app.URL + "/artifacts/" + id + "/edit"))
	b.wait(`document.querySelector('.cm-editor')`)
	b.eval(`(async()=>{
 const src=document.querySelector('script[src*="/gallery/controllers.js"]').src;
 const {registerEditorSection}=await import(new URL('editor-session.js',src));
 registerEditorSection('delayed-dialog-test',{isDirty:()=>true,discard(){},save:()=>new Promise(resolve=>window.finishEditorSave=resolve).then(value=>{window.editorSaveSettled=true;return value})});
 document.querySelector('.app-header-back').click();
 })()`)
	b.wait(`document.querySelector('.editor-leave-dialog').open`)
	b.eval(`document.querySelector('[data-action="unsaved-changes#saveAndLeave"]').click()`)
	b.wait(`window.finishEditorSave && document.querySelector('[data-keep-editing]').disabled`)
	b.eval(`window.editorDialog=document.querySelector('.editor-leave-dialog');window.editorParent=editorDialog.parentElement;editorDialog.remove()`)
	b.wait(`!editorDialog.open`)
	b.eval(`editorParent.append(editorDialog)`)
	b.eval(`document.querySelector('.app-header-back').click()`)
	b.wait(`editorDialog.open`)
	b.eval(`finishEditorSave(false)`)
	b.wait(`window.editorSaveSettled`)
	b.check(`editorDialog.open && !editorDialog.querySelector('[data-keep-editing]').disabled`)
}
