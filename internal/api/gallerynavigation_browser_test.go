package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

// Exercise the real Clerk loader/controller against a local SDK double. No
// third-party service or credentials are needed to count loads/reloads/mounts.
func galleryNavigationApp(t *testing.T, withoutScripts ...bool) (*httptest.Server, grantedInstance) {
	t.Helper()
	ro, st := newLoginTestRouter(t, &stubProvider{}, nil, func(c *Config) {
		c.Clerk = hostedLoginStub{}
		c.ClerkPublishableKey = "pk_test_navigation"
		c.AuthToken = ""
	})
	ctx := context.Background()
	viewer, err := st.UpsertHostedUser(ctx, "clerk:navigation", "person@example.com", time.Now().UnixMilli())
	require.NoError(t, err)
	owner, err := st.UpsertUser(ctx, "owner", "owner@example.test")
	require.NoError(t, err)
	in := grantedInstance{twoOwnerInstance: twoOwnerInstance{ro: ro, ownerOne: owner.ID, ownerTwo: viewer.ID}}
	in.artifactTwo = seedOwnedArtifact(t, ro, viewer.ID, "owned-tool", "Owned tool", "owned source", "owned state")
	in.granted = seedOwnedArtifact(t, ro, owner.ID, "incoming-tool", "Incoming tool", "incoming source", "incoming state")
	_, err = st.CreateGrant(ctx, viewer.ID, in.artifactTwo, owner.ID)
	require.NoError(t, err)
	_, err = st.CreateGrant(ctx, owner.ID, in.granted, viewer.ID)
	require.NoError(t, err)
	seedSharedGallery(t, in, 30)
	tag := &store.Tag{ID: "nav-work", OwnerID: viewer.ID, Name: "Work", Color: "#2563eb"}
	require.NoError(t, st.CreateTag(ctx, tag))
	require.NoError(t, st.AddArtifactTag(ctx, viewer.ID, in.artifactTwo, tag.ID))
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(withoutScripts) > 0 && withoutScripts[0] {
			w.Header().Set("Content-Security-Policy", "script-src 'none'")
		}
		if strings.HasPrefix(r.URL.Path, "/npm/") {
			w.Header().Set("Content-Type", "text/javascript")
			if strings.Contains(r.URL.Path, "clerk-js") {
				fmt.Fprint(w, `window.clerkCalls = {load:0,reload:0,mount:0,unmount:0}; window.Clerk = {
     load:async()=>{clerkCalls.load++}, isSignedIn:true,
     user:{reload:async()=>{clerkCalls.reload++}},
     mountUserButton:root=>{clerkCalls.mount++; const button=document.createElement('button'); button.className='cl-userButtonTrigger'; button.textContent='Account';root.append(button)},
     unmountUserButton:root=>{clerkCalls.unmount++;root.replaceChildren()}
    };`)
			} else {
				fmt.Fprint(w, `window.__internal_ClerkUICtor = function() {};`)
			}
			return
		}
		r.Header.Set("Test-Clerk-Subject", "clerk:navigation")
		ro.ServeHTTP(w, r)
	}))
	t.Cleanup(app.Close)
	ro.cfg.AppOrigin, ro.cfg.ClerkFrontendAPI = app.URL, app.URL
	render := httptest.NewServer(ro.RenderHandler())
	t.Cleanup(render.Close)
	ro.cfg.RenderOrigin = render.URL
	return app, in
}

func TestGallerySpaceNavigationBrowser(t *testing.T) {
	for _, phone := range []bool{false, true} {
		t.Run(fmt.Sprintf("phone=%t", phone), func(t *testing.T) {
			app, _ := galleryNavigationApp(t)
			browser, err := chromium.New(chromiumtest.Start(t))
			require.NoError(t, err)
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			width := int64(1280)
			if phone {
				width = 390
			}
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1, Mobile: phone})
			require.NoError(t, err)
			run := func(js string) {
				t.Helper()
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](js)))
			}
			poll := func(js string) {
				t.Helper()
				err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](js, chromedp.WithPollingTimeout(6*time.Second)))
				if err != nil {
					state, _ := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify({url:location.href,query:document.querySelector('[data-gallery-query]')?.value,form:document.querySelector('[data-gallery-search]')?.outerHTML,content:document.querySelector('#gallery-content')?.innerHTML.slice(0,800),calls:window.clerkCalls})`))
					t.Log(state)
				}
				require.NoError(t, err, js)
			}
			check := func(js string) {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool]("Boolean("+js+")"))
				require.NoError(t, err)
				if !ok {
					state, _ := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify({url:location.href,calls:window.clerkCalls,doc:document===original.document,header:document.querySelector('header')===original.header,clerk:document.querySelector('[data-controller="clerk"]')===original.clerk,button:document.querySelector('[data-clerk-target="mount"] button')===original.button,search:document.querySelector('[data-gallery-query]')?.value})`))
					t.Log(state)
				}
				require.True(t, ok, js)
			}
			visit := func(path string) {
				t.Helper()
				run(`document.querySelector('.gallery-spaces a[href="` + path + `"]').click()`)
				poll(`location.pathname === '` + path + `' && document.querySelector('.gallery-spaces [aria-current]').getAttribute('href') === '` + path + `'`)
			}
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/shared")))
			poll(`window.clerkCalls?.mount === 1`)
			run(`window.original = {document,header:document.querySelector('header'),clerk:document.querySelector('[data-controller="clerk"]'),button:document.querySelector('[data-clerk-target="mount"] button')};`)
			for i := 0; i < 2; i++ {
				visit("/")
				visit("/search")
				visit("/activity")
				visit("/shared")
			}
			check(`document===original.document && document.querySelector('header')===original.header && document.querySelector('[data-controller="clerk"]')===original.clerk && document.querySelector('[data-clerk-target="mount"] button')===original.button && JSON.stringify(clerkCalls)==='{"load":1,"reload":1,"mount":1,"unmount":0}'`)
			check(`document.title==='Shared · Exhibit' && document.querySelector('main h1').textContent==='Shared' && document.querySelector('.card-owner-name') && document.querySelector('[data-duplicate-dialog]')`)
			run(`document.querySelector('[data-gallery-query]').value='chart';document.querySelector('[data-gallery-query]').dispatchEvent(new Event('input',{bubbles:true}))`)
			poll(`location.search==='?q=chart' && document.querySelectorAll('.card').length>=24`)
			run(`document.querySelector('[data-gallery-page]')?.click()`)
			poll(`document.querySelectorAll('.card').length===30`)
			visit("/activity")
			run(`history.back()`)
			poll(`location.pathname==='/shared' && document.querySelector('[data-gallery-query]')?.value==='chart'`)
			run(`history.forward()`)
			poll(`location.pathname==='/activity' && document.querySelector('.activity-heading')`)
			visit("/search")
			run(`document.querySelector('.gallery-rail-filters [data-gallery-filter]').click()`)
			poll(`location.search==='?tag=Work' && document.querySelectorAll('.card').length===1`)
			run(`document.querySelector('.card-actions-trigger').click();document.querySelector('[data-action-menu-event-param="exhibit:duplicate"]').click()`)
			poll(`document.querySelector('[data-duplicate-dialog]').open`)
			run(`document.querySelector('[data-duplicate-dialog] form').requestSubmit()`)
			poll(`!document.querySelector('[data-duplicate-dialog]').open`)
			visit("/search")
			poll(`Array.from(document.querySelectorAll('.card-title')).some(a=>a.textContent==='Owned tool (copy)')`)
			run(`document.body.dispatchEvent(new KeyboardEvent('keydown',{key:'/',bubbles:true}))`)
			check(`document.activeElement.matches('[data-gallery-query]')`)
			run(`document.querySelector('[data-gallery-query]').value='not-a-match';document.querySelector('[data-gallery-query]').dispatchEvent(new Event('input',{bubbles:true}))`)
			poll(`document.querySelectorAll('.card').length===0 && document.querySelector('.empty')?.textContent==='No artifacts match your search.'`)
			run(`document.querySelector('[data-gallery-clear]').click()`)
			poll(`document.querySelectorAll('.card').length>=24 && location.search===''`)
			visit("/shared")
			check(`document.querySelector('.card-by').closest('.card').querySelector('[data-gallery-widget]').dataset.widgetReload.includes('/widget/viewer/open')`)
			run(`window.widget=document.querySelector('.card-by').closest('.card').querySelector('[data-gallery-widget]');widget.style.marginTop='2000px'`)
			poll(`!widget.querySelector('iframe')`)
			run(`widget.style.marginTop=''`)
			poll(`widget.querySelector('iframe')?.src.includes('/widget/viewer/open')`)
			check(`document===original.document && document.querySelector('header')===original.header && document.querySelector('[data-clerk-target="mount"] button')===original.button && clerkCalls.mount===1 && clerkCalls.unmount===0 && document.documentElement.scrollWidth<=innerWidth`)

		})
	}
}

func TestGallerySpaceNavigationRacesBrowser(t *testing.T) {
	app, _ := galleryNavigationApp(t)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	run := func(js string) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](js)))
	}
	poll := func(js string) {
		t.Helper()
		err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](js, chromedp.WithPollingTimeout(6*time.Second)))
		if err != nil {
			state, _ := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify({url:location.href,query:document.querySelector('[data-gallery-query]')?.value,form:document.querySelector('[data-gallery-search]')?.outerHTML,content:document.querySelector('#gallery-content')?.innerHTML.slice(0,800),calls:window.clerkCalls})`))
			t.Log(state)
		}
		require.NoError(t, err, js)
	}
	check := func(js string) {
		t.Helper()
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool]("Boolean("+js+")"))
		require.NoError(t, err)
		require.True(t, ok, js)
	}
	visit := func(path string) {
		t.Helper()
		run(`document.querySelector('.gallery-spaces a[href="` + path + `"]').click()`)
		poll(`location.pathname === '` + path + `' && document.querySelector('.gallery-spaces [aria-current]').getAttribute('href') === '` + path + `'`)
	}
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/shared")))
	poll(`window.clerkCalls?.mount === 1`)
	run(`window.originalHeader=document.querySelector('header');window.originalButton=document.querySelector('[data-clerk-target="mount"] button');
 window.nativeFetch=window.fetch.bind(window);
 // Hold responses after the network has finished, ignoring abort deliberately.
 // This exercises both the body-reading window and requests already received.
 window.hold=(match,stage)=>{
   window.release=undefined;window.released=false;window.completed=false;
   window.fetch=async(url,options)=>{
     const response=await nativeFetch(url,options);
     if(!match(url)) return response;
     const text=await response.text();
     const delayed=()=>new Promise(resolve=>{window.release=()=>{window.released=true;resolve(text)}});
     if(stage==='body') return {status:response.status,headers:response.headers,url:response.url,text:delayed};
     await delayed();window.completed=true;
     return new Response(text,{status:response.status,headers:response.headers});
   };
 };
 hold(url=>url.includes('/partials/gallery-grid/shared?q=chart'),'body');
 document.querySelector('[data-gallery-query]').value='chart';document.querySelector('[data-gallery-query]').dispatchEvent(new Event('input',{bubbles:true}));`)
	poll(`typeof release === 'function'`)
	visit("/search")
	run(`release();window.fetch=nativeFetch`)
	poll(`released`)
	check(`location.pathname === '/search' && document.title === 'Search · Exhibit' && document.querySelector('[data-duplicate-dialog]') && document.querySelector('.card-owner-name')`)
	// Pagination from an incoming space may not append into the owner space.
	visit("/shared")
	run(`hold(url=>url.includes('/partials/gallery-grid/shared?offset='),'headers');document.querySelector('[data-gallery-page]')?.click()`)
	poll(`typeof release === 'function'`)
	visit("/")
	run(`release();window.fetch=nativeFetch`)
	poll(`completed`)
	check(`location.pathname === '/' && document.querySelectorAll('.card').length === 0 && !document.querySelector('.card-owner-name')`)
	// Two space requests in quick succession: the last destination wins even
	// if the first request finishes reading its body afterwards.
	run(`hold(url=>url==='/activity','body');document.querySelector('.gallery-spaces a[href="/activity"]').click()`)
	poll(`typeof release === 'function'`)
	// The tab bar answers the click while the content is still in flight: the
	// destination is marked current and the outgoing results step back. Without
	// this the click produces no visible change at all until the response
	// lands, which is most of what made a switch feel slow.
	check(`document.querySelector('.gallery-spaces a[href="/activity"]').getAttribute('aria-current') === 'page' && document.querySelector('#gallery-shell').hasAttribute('data-gallery-pending') && document.querySelector('[data-gallery-results]') !== null`)
	visit("/search")
	run(`release();window.fetch=nativeFetch`)
	poll(`released`)
	check(`location.pathname === '/search' && document.querySelector('[data-gallery-search]').getAttribute('hx-get') === '/partials/gallery-grid/search' && !document.querySelector('#gallery-shell').hasAttribute('data-gallery-pending')`)
	// Back interrupts an outstanding space request and restores the old URL
	// through the same region, without replacing the header.
	run(`hold(url=>url==='/activity','body');document.querySelector('.gallery-spaces a[href="/activity"]').click()`)
	poll(`typeof release === 'function'`)
	run(`history.back()`)
	poll(`location.pathname === '/' && document.querySelector('.gallery-spaces [aria-current]').getAttribute('href') === '/'`)
	run(`release();window.fetch=nativeFetch`)
	poll(`released`)
	check(`document.querySelector('header') === originalHeader && document.querySelector('[data-clerk-target="mount"] button') === originalButton && clerkCalls.mount === 1 && clerkCalls.reload === 1 && clerkCalls.unmount === 0`)
	// Destination failures retain a coherent current space and expose a retry.
	run(`window.fetch=(url,options)=>url==='/activity'?Promise.resolve(new Response('private server failure',{status:500})):nativeFetch(url,options);document.querySelector('.gallery-spaces a[href="/activity"]').click()`)
	poll(`!document.querySelector('[data-gallery-navigation-error]').hidden`)
	check(`location.pathname === '/' && document.querySelector('.gallery-spaces [aria-current]').getAttribute('href') === '/' && !document.body.textContent.includes('private server failure')`)
	run(`window.fetch=(url,options)=>url==='/activity'?Promise.reject(new TypeError('offline')):nativeFetch(url,options);document.querySelector('.gallery-spaces a[href="/activity"]').click()`)
	poll(`!document.querySelector('[data-gallery-navigation-error]').hidden`)
	run(`window.fetch=nativeFetch`)
	visit("/shared")
	check(`document.querySelector('[data-gallery-navigation-error]').hidden && document.querySelector('header') === originalHeader && clerkCalls.mount === 1`)
	// Modified clicks remain browser actions and do not issue htmx requests.
	run(`window.modifiedRequests=0;document.addEventListener('htmx:before:request',()=>modifiedRequests++);
 for(const modifier of ['ctrlKey','metaKey','shiftKey','altKey']) {
   const link=document.querySelector('.gallery-spaces a[href="/"]');
   const event=new MouseEvent('click',{bubbles:true,cancelable:true,button:0,[modifier]:true});
   // Suppress the native popup/download. htmx still sees the modifiers.
   document.addEventListener('click',e=>e.preventDefault(),{once:true,capture:true});
   link.dispatchEvent(event);
 }`)
	check(`modifiedRequests === 0 && location.pathname === '/shared'`)
}

func TestGalleryCachedTabScrollBrowser(t *testing.T) {
	app, in := galleryNavigationApp(t)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: 390, Height: 844, DeviceScaleFactor: 1, Mobile: true})
	require.NoError(t, err)
	run := func(js string) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](js)))
	}
	poll := func(js string) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](js, chromedp.WithPollingTimeout(8*time.Second))), js)
	}
	screenshot := func(name string) {
		t.Helper()
		if dir := os.Getenv("GALLERY_SCREENSHOT_DIR"); dir != "" {
			require.NoError(t, os.MkdirAll(dir, 0755))
			shot, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, name+".png"), shot, 0644))
		}
	}
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/shared")))
	poll(`document.querySelector('[data-gallery-page][data-htmx-powered]') && window.clerkCalls?.mount === 1`)
	run(`document.querySelector('[data-gallery-page]').click()`)
	poll(`document.querySelectorAll('.card').length === 32`)
	run(`
		window.cards = () => [...document.querySelectorAll('.card')];
		window.anchorKey = cards()[25].dataset.href;
		window.anchor = () => cards().find(card => card.dataset.href === anchorKey);
		window.freshKey = '/artifacts/fresh-share';
		window.fresh = () => cards().some(card => card.dataset.href === freshKey);
		window.visit = path => document.querySelector('.gallery-spaces a[href="'+path+'"]').click();
		window.nativeFetch = window.fetch.bind(window);
		window.holdShared = () => {
			window.release = undefined;
			window.sharedRequests = 0;
			window.fetch = async (url, options) => {
				const response = await nativeFetch(url, options);
				if (!url.startsWith('/shared')) return response;
				const html = await response.text();
				sharedRequests++;
				await new Promise(resolve => { window.release = resolve; });
				return new Response(html, {status: response.status, headers: response.headers});
			};
		};
		// Part of the card above the anchor is visible too. Use the first
		// visible card so this assertion matches the reader's leading row.
		window.scrollTo(0, scrollY + anchor().getBoundingClientRect().top + 20);
		window.originalOffset = anchor().getBoundingClientRect().top;
		window.originalScroll = scrollY;
		window.track = () => {
			if (location.pathname === '/shared') {
				window.offsets.push(anchor()?.getBoundingClientRect().top ?? null);
			}
			window.animationFrame = requestAnimationFrame(track);
		};
	`)
	poll(`Math.abs(originalOffset + 20) < 1 && document.querySelector('#gallery-shell').hasAttribute('data-chrome-hidden')`)
	run(`visit('/activity')`)
	poll(`location.pathname === '/activity' && scrollY === 0`)

	// Insert one item and remove two ahead of the anchor. Its scroll offset
	// has to change, while its viewport offset has to stay exactly the same.
	added := seedOwnedArtifact(t, in.ro, in.ownerOne, "fresh-share", "Fresh share", "source", "state")
	_, err = in.ro.cfg.Store.CreateGrant(ctx, in.ownerOne, added, in.ownerTwo)
	require.NoError(t, err)
	for _, id := range []string{"shared-029", "shared-028"} {
		require.NoError(t, in.ro.cfg.Store.DeleteGrant(ctx, in.ownerOne, id, in.ownerTwo))
	}
	run(`holdShared(); window.offsets = []; track(); visit('/shared');`)
	poll(`typeof release === 'function' && location.pathname === '/shared' && !fresh() && cards().length === 32 && offsets.length >= 3 && !document.querySelector('#gallery-shell').getAnimations({subtree: true}).length`)
	screenshot("cached-tab-before-refresh")
	run(`release()`)
	poll(`fresh() && cards().length === 31 && offsets.length >= 8 && scrollY < originalScroll && sharedRequests === 1 && location.search === ''`)
	// Sample several frames after the swap too, catching a delayed native
	// anchor adjustment or a late widget/layout shift.
	run(`window.settledAt = offsets.length`)
	poll(`offsets.length >= settledAt + 5`)
	run(`cancelAnimationFrame(animationFrame)`)
	offsets, err := chromedp.Run(ctx, chromedp.Evaluate[[]*float64](`offsets`))
	require.NoError(t, err)
	for i, offset := range offsets {
		require.NotNil(t, offset, "anchor present on frame %d", i)
		require.InDelta(t, -20, *offset, 1, "anchor position on frame %d", i)
	}
	screenshot("cached-tab-after-refresh")

	// If the response can't fit the viewport anchor, keep the current view
	// until the reader scrolls to a position where the update fits.
	run(`visit('/activity')`)
	poll(`location.pathname === '/activity'`)
	key, err := chromedp.Run(ctx, chromedp.Evaluate[string](`anchorKey`))
	require.NoError(t, err)
	for i := range 28 {
		id := fmt.Sprintf("shared-%03d", i)
		if "/artifacts/"+id != key {
			require.NoError(t, in.ro.cfg.Store.DeleteGrant(ctx, in.ownerOne, id, in.ownerTwo))
		}
	}
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(ctx, in.ownerOne, added, in.ownerTwo))
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(ctx, in.ownerOne, in.granted, in.ownerTwo))
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(ctx, in.ownerTwo, in.artifactTwo, in.ownerOne))
	run(`holdShared(); visit('/shared')`)
	poll(`typeof release === 'function' && location.pathname === '/shared' && cards().length === 31`)
	run(`
		window.refreshFinished = false;
		document.addEventListener('htmx:finally:request', event => {
			if (event.detail.ctx.galleryRefresh?.path === '/shared') refreshFinished = true;
		});
		release();
	`)
	poll(`refreshFinished && Math.abs(anchor().getBoundingClientRect().top - originalOffset) < 1 && cards().length === 31`)
	run(`window.scrollTo(0, 0)`)
	poll(`cards().length === 1 && anchor() && scrollY === 0`)

	// A response already read from the network can arrive after navigation,
	// even when the outgoing tab's request was aborted.
	run(`visit('/activity')`)
	poll(`location.pathname === '/activity'`)
	_, err = in.ro.cfg.Store.CreateGrant(ctx, in.ownerOne, added, in.ownerTwo)
	require.NoError(t, err)
	run(`holdShared(); visit('/shared')`)
	poll(`typeof release === 'function' && location.pathname === '/shared' && cards().length === 1`)
	run(`visit('/activity')`)
	poll(`location.pathname === '/activity'`)
	run(`refreshFinished = false; release()`)
	poll(`refreshFinished && location.pathname === '/activity' && document.querySelector('.activity-heading') && cards().length === 0`)
}

func TestGallerySpaceNavigationWithoutJSBrowser(t *testing.T) {
	app, in := galleryNavigationApp(t, true)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+fmt.Sprintf("/shared?owner=%d&q=chart", in.ownerOne))))
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.querySelector('[data-gallery-query]').value === 'chart' && document.querySelector('[data-gallery-search] input[name=owner]') === null && document.querySelectorAll('.card').length === 24 && !window.Clerk`))
	require.NoError(t, err)
	require.True(t, ok, "direct query/filter URLs work without JavaScript")
	for _, path := range []string{"/", "/search", "/shared"} {
		require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`document.querySelector('.gallery-spaces a[href="`+path+`"]').click()`)))
		// Native navigation destroys the old execution context. Retry the
		// assertion against the new document rather than polling inside it.
		var lastURL string
		var lastErr error
		defer func() {
			if t.Failed() {
				t.Logf("native destination %s: url=%q err=%v", path, lastURL, lastErr)
			}
		}()
		require.Eventually(t, func() bool {
			frames, err := chromedp.Call(ctx, page.GetFrameTree, cdp.Empty{})
			lastErr = err
			if err != nil {
				return false
			}
			lastURL = frames.FrameTree.Frame.URL
			return lastURL == app.URL+path
		}, 5*time.Second, 50*time.Millisecond)
	}
}
