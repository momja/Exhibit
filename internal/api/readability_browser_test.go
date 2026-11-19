package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

// Use the shipped pages and computed colors: palette regressions can affect
// labels on the ground, selected controls, or hover fills as well as cards.
func TestReadabilityBrowser(t *testing.T) {
	ro := newTestRouter(t)
	for _, title := range []string{
		"A long primary title that should fade on one line alongside card metadata controls",
		"AnUnbrokenArtifactNameThatMustFadeWithoutPushingControlsOffTheCardOrAddingAnotherLine",
	} {
		id := createArtifact(t, ro, map[string]any{"title": title, "body": "<h1>Readability</h1>"})
		require.NoError(t, ro.cfg.Store.SetArtifactStarred(t.Context(), store.ViewerID(1), id, true))
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
			for _, path := range []string{"/search", "/new"} {
				t.Run(fmt.Sprintf("%s/%d%s", theme, width, path), func(t *testing.T) {
					require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+path), chromedp.WaitVisible(chromedp.CSS("main"))))
					if path == "/search" {
						require.NoError(t, chromedp.Do(ctx, chromedp.WaitVisible(chromedp.CSS(".card-title"))))
					}
					check(t, readabilityContrastChecks)
					for _, percent := range []int{100, 200} {
						// A larger root font models text-only enlargement, not a wider layout
						// viewport or a page-scale transform that leaves text metrics unchanged.
						require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`document.documentElement.style.fontSize = '%d%%'`, percent))))
						check(t, readabilityLayoutChecks)
						if path == "/search" && percent == 100 {
							modalBrowser{t, ctx}.capture("Exh-e736/gallery-"+theme, width)
						}
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
  if (style.whiteSpace !== 'nowrap' || style.overflowX !== 'hidden' || !style.maskImage.includes('linear-gradient')) failures.push('title must clip on one line with a fade');
  if (title.textContent !== title.title) failures.push('full title tooltip missing');
  if (phone && parseFloat(style.fontSize) < root) failures.push('mobile title too small');
  const lineHeight = parseFloat(style.lineHeight), padding = parseFloat(style.paddingTop) + parseFloat(style.paddingBottom);
  if (title.clientHeight > Math.max(phone ? 44 : 0, lineHeight + padding) + 1) failures.push('title reserves more than one line');
  const star = row.querySelector('.card-star'), actions = row.querySelector('.card-actions');
  if (star && actions) {
   const a = star.getBoundingClientRect(), b = actions.getBoundingClientRect();
   if (Math.abs((a.top + a.height/2) - (b.top + b.height/2)) > 1) failures.push('star is not centered with card actions');
  }
  for (const control of row.querySelectorAll('button,[role=button]')) {
   if (control.getBoundingClientRect().width && !fits(control,row)) failures.push('metadata control outside its row');
  }
 }
 for (const route of document.querySelectorAll('.route')) {
  for (const text of route.querySelectorAll('b,.route-note')) {
   if (!text.getClientRects().length) continue;
   if (!fits(text,route) || text.scrollWidth > text.clientWidth + 1) failures.push('creation choice text clipped');
   const expected = text.matches('b') ? root : root * .875;
   if (phone && parseFloat(getComputedStyle(text).fontSize) < expected) failures.push('creation choice text too small');
  }
 }
 const description = document.querySelector('#route-description');
 if (description?.getClientRects().length) {
  if (!fits(description,description.closest('main')) || description.scrollWidth > description.clientWidth + 1) failures.push('selected creation explanation clipped');
  if (phone && parseFloat(getComputedStyle(description).fontSize) < root * .875) failures.push('selected creation explanation too small');
 }
 if (phone) for (const field of document.querySelectorAll('main input[type=text],main textarea')) {
  if (field.getBoundingClientRect().width && parseFloat(getComputedStyle(field).fontSize) < Math.max(16,root)) failures.push('touch field text too small');
 }
 const viewport = document.querySelector('meta[name=viewport]').content;
 if (/user-scalable\s*=\s*(no|0)|maximum-scale\s*=\s*1\b/.test(viewport)) failures.push('zoom disabled');
 return failures;
})()`

// Relative field tokens also reach the agent composer and the saved-data
// choices. Exercise those consumers at doubled text size, not just /new.
func TestEnlargedFieldsBrowser(t *testing.T) {
	ro := newTestRouter(t)
	id := createTestArtifact(t, ro, "Enlarged fields")
	app := httptest.NewServer(ro)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int64{320, 375, 390, 430} {
			t.Run(fmt.Sprintf("%s/%d", theme, width), func(t *testing.T) {
				b := newModalBrowser(t, width)
				_, err := chromedp.Call(b.ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: theme}}})
				require.NoError(t, err)
				b.run(chromedp.EmulateViewport(width, 568, chromedp.EmulateTouch), chromedp.Navigate(app.URL+"/agent"))
				b.wait(`window.closeKeyModal && document.getElementById('input')`)
				b.eval(`window.closeKeyModal();document.documentElement.style.fontSize='200%';
     const input=document.getElementById('input');input.value='First line of a longer message\nSecond line of a longer message\nThird line';input.dispatchEvent(new Event('input',{bubbles:true}))`)
				b.check(`(() => {
     const input=document.getElementById('input'), style=getComputedStyle(input), box=input.getBoundingClientRect(), row=input.closest('.compose-row').getBoundingClientRect(), send=document.getElementById('send-btn').getBoundingClientRect();
     return matchMedia('(pointer:coarse)').matches && parseFloat(style.fontSize)>=32 && parseFloat(style.lineHeight)>=parseFloat(style.fontSize)*1.2 && box.left>=row.left && box.right<=row.right && row.right<=innerWidth && box.top>=document.querySelector('.app-header').getBoundingClientRect().bottom && box.bottom<=innerHeight && send.top>=0 && send.bottom<=innerHeight;
    })()`)
				b.capture("Exh-h9m4/agent-"+theme, width)
				b.run(chromedp.Navigate(app.URL + "/artifacts/" + id))
				b.wait(`window.apiFetch && document.getElementById('share-open')`)
				b.eval(`document.documentElement.style.fontSize='200%';document.getElementById('share-open').click()`)
				b.wait(`document.getElementById('share-panel').open`)
				b.check(`(() => {
     const dialog=document.getElementById('share-panel');
     return dialog.scrollWidth<=dialog.clientWidth && [...dialog.querySelectorAll('.state-choice strong')].every(heading=>{
      const box=heading.closest('.state-choice').getBoundingClientRect(),range=document.createRange();range.selectNodeContents(heading);
      return parseFloat(getComputedStyle(heading).fontSize)>=32 && [...range.getClientRects()].every(line=>line.left>=box.left && line.right<=box.right);
     });
    })()`)
				b.capture("Exh-h9m4/share-"+theme, width)
			})
		}
	}
}

// Both notices load through their real htmx partials. Their combined height
// must leave the enlarged composer usable, including on a short phone.
func TestEnlargedComposerNoticesBrowser(t *testing.T) {
	ro := newTestRouter(t)
	st := ro.cfg.Store.(*store.SQLiteStore)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: true, Store: st, Box: ro.cfg.Secrets})
	app := httptest.NewServer(ro)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	for _, fixture := range []struct {
		name, body, input string
		warnings          int
	}{
		{"short", `<script>fetch('https://api.example.test/', {mode:'no-cors'})</script>`, "First\nSecond\nThird", 1},
		{"wrapped", `<script>fetch('https://corsproxy.io/', {mode:'no-cors'})</script>`, "First line of a longer message\nSecond line of a longer message\nThird line", 2},
	} {
		id := createArtifact(t, ro, map[string]any{"title": "Composer notices", "body": fixture.body})
		_, err := st.RequestArtifactConnection(context.Background(), 1, id, "bearer", "Show my saved reading list")
		require.NoError(t, err)
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/agent-enabled=%t", fixture.name, enabled), func(t *testing.T) {
				ro.cfg.Agent = nil
				if enabled {
					ro.cfg.Agent = &agent.Manager{} // Only presence is read; no sidecar starts.
				}
				b := newModalBrowser(t, 320)
				b.run(chromedp.EmulateViewport(320, 568, chromedp.EmulateTouch), chromedp.Navigate(app.URL+"/agent?artifact="+id))
				b.wait(fmt.Sprintf(`window.closeKeyModal && document.querySelectorAll('[data-network-warning]').length === %d && document.querySelectorAll('#agent-connection-requests a').length === 2`, fixture.warnings))
				b.eval(fmt.Sprintf(`window.closeKeyModal();document.documentElement.style.fontSize='200%%';const input=document.getElementById('input');input.value=%q;input.dispatchEvent(new Event('input',{bubbles:true}))`, fixture.input))
				b.capture("Exh-h9m4/notices-"+fixture.name+fmt.Sprintf("-%t", enabled), 320)
				checkControls := func() {
					t.Helper()
					failures, err := chromedp.Run(b.ctx, chromedp.Evaluate[[]string](`(() => {
     const failures=[];
     if (!matchMedia('(pointer:coarse)').matches || parseFloat(getComputedStyle(document.getElementById('input')).fontSize)<32) failures.push('text enlargement not applied');
     if (document.documentElement.scrollWidth>innerWidth) failures.push('horizontal overflow');
     for (const id of ['input','send-btn']) {
      const element=document.getElementById(id),box=element.getBoundingClientRect();
      if (box.top<document.querySelector('.app-header').getBoundingClientRect().bottom || box.bottom>innerHeight || !element.contains(document.elementFromPoint(box.x+box.width/2,box.y+box.height/2))) failures.push(id+' clipped: '+box.top+'..'+box.bottom);
     }
     return failures;
    })()`))
					require.NoError(t, err)
					require.Empty(t, failures)
				}
				checkControls()
				// Native focus must reveal both account actions without moving the
				// input or Send out of view. The warning and request text stay present.
				// Notices scroll in their own capped region, so enlarged text can make
				// an item taller than that region. A focused item must then fill all of
				// it, unobstructed, and the rest is reached by scrolling.
				for _, selector := range []string{"#agent-network-warnings strong", "#agent-network-warnings li:last-child", "#agent-connection-requests a:first-of-type", "#agent-connection-requests a:last-of-type"} {
					b.eval(fmt.Sprintf(`{const item=document.querySelector(%q);item.focus();item.scrollIntoView({block:'nearest'})}`, selector))
					b.check(fmt.Sprintf(`(() => {const item=document.querySelector(%q),box=item.getBoundingClientRect(),region=item.closest('.composer-notices')?.getBoundingClientRect()??{top:0,bottom:innerHeight,height:innerHeight};const top=Math.max(box.top,region.top,0),bottom=Math.min(box.bottom,region.bottom,innerHeight);return bottom-top>=Math.min(box.height,region.height)-1 && [top+1,(top+bottom)/2,bottom-1].every(y=>item.contains(document.elementFromPoint(box.x+box.width/2,y)))})()`, selector))
					checkControls()
				}
				b.check(`document.querySelector('.connection-request-notice').textContent.includes('Show my saved reading list') && document.querySelector('[data-network-warning="no_cors"]').textContent.includes('opaque, unreadable')`)
				b.run(chromedp.Navigate(app.URL + "/agent"))
				b.wait(`window.closeKeyModal && document.getElementById('input')`)
				b.eval(`window.closeKeyModal()`)
				b.check(`(() => {const composer=document.querySelector('.composer'),row=document.querySelector('.compose-row');return !document.querySelector('#agent-network-warnings section,#agent-connection-requests section') && row.getBoundingClientRect().top-composer.getBoundingClientRect().top<=20})()`)
			})
		}
	}
}
