package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
)

// Compare shipped content to a plain shared dialog under the same page CSS.
// Width, scrolling and content layouts deliberately are not identical: those
// exceptions are documented in ui-components.md. Typography/forms/actions are.
func checkModalVisuals(t *testing.T, ctx context.Context, selector string) {
	t.Helper()
	differences, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](fmt.Sprintf(`(() => {
 const dialog = document.querySelector(%q);
 const reference = document.createElement('dialog');
 reference.className = 'modal-dialog';
 reference.innerHTML = '<h2>Heading</h2><label for="visual-reference">Caption</label><input id="visual-reference" type="text"><p>Body copy</p><label class="modal-check"><input type="checkbox">Option</label><div class="modal-actions"><button class="btn">Save</button></div><div class="modal-error">Error</div>';
 document.body.append(reference);
 const differences = [];
 function compare(actual, baseline, properties) {
   const a=getComputedStyle(actual), b=getComputedStyle(baseline);
   for (const property of properties) if (a[property] !== b[property])
     differences.push(actual.tagName+'.'+actual.className+' '+property+': '+a[property]+' (default '+b[property]+')');
 }
 compare(dialog, reference, ['backgroundColor','boxShadow']);
 compare(dialog.querySelector('h2'), reference.querySelector('h2'), ['fontSize','fontWeight','marginBottom']);
 for (const caption of dialog.querySelectorAll('label[for]'))
   compare(caption, reference.querySelector('label[for]'), ['fontSize','fontWeight','textTransform','color','marginTop','marginBottom']);
 for (const field of dialog.querySelectorAll('input[type=text]:not([readonly]),input[type=password],select'))
   compare(field, reference.querySelector('input'), ['paddingTop','paddingRight','borderRadius','fontSize','backgroundColor']);
 for (const p of dialog.querySelectorAll('p:not(.modal-error):not([data-permission-status])'))
   compare(p, reference.querySelector('p'), ['fontSize','lineHeight','color']);
 for (const check of dialog.querySelectorAll('.modal-check'))
   compare(check, reference.querySelector('.modal-check'), ['display','fontSize','gap','alignItems']);
 for (const footer of dialog.querySelectorAll('.modal-actions'))
   compare(footer, reference.querySelector('.modal-actions'), ['display','gap','marginTop','paddingTop','borderTopWidth','flexWrap']);
 for (const error of dialog.querySelectorAll('.modal-error,[data-permission-status]'))
   compare(error, reference.querySelector('.modal-error'), ['fontSize','color','marginTop']);
 const bounds=dialog.getBoundingClientRect();
 if (bounds.left < 0 || bounds.right > innerWidth || dialog.scrollWidth > dialog.clientWidth)
   differences.push('dialog overflows horizontally');
 reference.remove();
 return differences;
})()`, selector)))
	require.NoError(t, err)
	require.Empty(t, differences, "dialog must use shared visual defaults: %s", selector)
}
