package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Use the shipped pages and computed colors: palette regressions can affect
// labels on the ground, selected controls, or hover fills as well as cards.
func TestReadabilityBrowser(t *testing.T) {
	ro := newTestRouter(t)
	for _, title := range []string{
		"A long primary title that should remain fully readable alongside card metadata controls",
		"AnUnbrokenArtifactNameThatMustWrapWithoutPushingControlsOffTheCardOrClippingTheLastLine",
	} {
		createArtifact(t, ro, map[string]any{"title": title, "body": "<h1>Readability</h1>"})
	}
	app := httptest.NewServer(ro)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	check := func(t *testing.T, script string) {
		t.Helper()
		failures, err := chromedp.Run(ctx, chromedp.Evaluate[[]string](script))
		require.NoError(t, err)
		require.Empty(t, failures)
	}
	for _, theme := range []string{"light", "dark"} {
		_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: theme}}})
		require.NoError(t, err)
		for _, width := range []int64{320, 375, 390, 430, 1280} {
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1, Mobile: width < 768})
			require.NoError(t, err)
			_, err = chromedp.Call(ctx, emulation.SetTouchEmulationEnabled, emulation.SetTouchEmulationEnabledParams{Enabled: width < 768})
			require.NoError(t, err)
			for _, path := range []string{"/", "/new"} {
				t.Run(fmt.Sprintf("%s/%d%s", theme, width, path), func(t *testing.T) {
					require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+path), chromedp.WaitVisible(chromedp.CSS("main"))))
					check(t, readabilityContrastChecks)
					for _, percent := range []int{100, 200} {
						// A larger root font models text-only enlargement, not a wider layout
						// viewport or a page-scale transform that leaves text metrics unchanged.
						require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`document.documentElement.style.fontSize = '%d%%'`, percent))))
						check(t, readabilityLayoutChecks)
					}
				})
			}
		}
	}
}

const readabilityContrastChecks = `(() => {
 const probe = document.createElement('span'); document.body.append(probe);
 const failures = [];
 function luminance(color) {
  const channels = color.match(/[\d.]+/g).slice(0,3).map(Number).map(v => {
   v /= 255; return v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4;
  });
  return channels[0]*.2126 + channels[1]*.7152 + channels[2]*.0722;
 }
 function pair(ink, surface) {
  probe.style.color = 'var(--' + ink + ')'; probe.style.backgroundColor = 'var(--' + surface + ')';
  const style = getComputedStyle(probe), a = luminance(style.color), b = luminance(style.backgroundColor);
  const ratio = (Math.max(a,b)+.05)/(Math.min(a,b)+.05);
  if (ratio < 4.5) failures.push(ink + ' on ' + surface + ': ' + ratio.toFixed(2));
 }
 for (const ink of ['color-ink','color-ink-soft','color-ink-mid','color-muted','color-label','color-faint','color-placeholder'])
  for (const surface of ['color-panel','color-ground','color-subtle','color-hover-fill']) pair(ink,surface);
 for (const ink of ['color-slate-medium','color-slate-muted','color-slate-placeholder'])
  for (const surface of ['color-panel','color-ground']) pair(ink,surface);
 for (const [ink,surface] of [
  ['brand-blue','color-panel'],['brand-blue','color-accent-tint'],['color-on-accent','brand-blue'],
  ['color-danger','color-panel'],['color-on-danger','color-danger'],['color-on-danger','color-danger-hover'],
  ['color-danger-ink','color-danger-tint'],['color-warn-ink','color-warn-tint'],
  ['color-info-ink','color-info-tint'],['color-note-ink','color-note-tint']
 ]) pair(ink,surface);
 probe.remove(); return failures;
})()`

const readabilityLayoutChecks = `(() => {
 const failures = [], phone = innerWidth < 768, root = parseFloat(getComputedStyle(document.documentElement).fontSize);
 if (document.documentElement.scrollWidth > innerWidth) failures.push('horizontal page overflow');
 const fits = (child,parent) => {
  const a = child.getBoundingClientRect(), b = parent.getBoundingClientRect();
  return a.left >= b.left - 1 && a.right <= b.right + 1 && a.top >= b.top - 1 && a.bottom <= b.bottom + 1;
 };
 for (const title of document.querySelectorAll('.card-title')) {
  const row = title.closest('.card-meta'), style = getComputedStyle(title);
  if (!fits(title,row)) failures.push('title outside metadata row');
  if (phone && (title.scrollWidth > title.clientWidth + 1 || style.whiteSpace === 'nowrap' || parseFloat(style.fontSize) < root)) failures.push('mobile title clipped or too small');
  for (const control of row.querySelectorAll('button,[role=button]')) {
   if (control.getBoundingClientRect().width && !fits(control,row)) failures.push('metadata control outside its row');
  }
 }
 for (const route of document.querySelectorAll('.route')) {
  for (const text of route.querySelectorAll('b,.route-note')) {
   if (!fits(text,route) || text.scrollWidth > text.clientWidth + 1) failures.push('creation choice text clipped');
   const expected = text.matches('b') ? root : root * .875;
   if (phone && parseFloat(getComputedStyle(text).fontSize) < expected) failures.push('creation choice text too small');
  }
 }
 if (phone) for (const field of document.querySelectorAll('main input[type=text],main textarea')) {
  if (field.getBoundingClientRect().width && parseFloat(getComputedStyle(field).fontSize) < Math.max(16,root)) failures.push('touch field text too small');
 }
 const viewport = document.querySelector('meta[name=viewport]').content;
 if (/user-scalable\s*=\s*(no|0)|maximum-scale\s*=\s*1\b/.test(viewport)) failures.push('zoom disabled');
 return failures;
})()`
