package render

import (
	"encoding/base64"
	"fmt"
	"html"

	"github.com/momja/Exhibit/internal/artifacticon"
	"github.com/momja/Exhibit/internal/store"
)

// ExhibitProductOrigin is the single destination for anonymous-share attribution.
const ExhibitProductOrigin = "https://exhibit.dizzard.net"

// attributedShare gives viewport-based tools their own reduced viewport. srcdoc
// inherits the response CSP; the child also has an explicit opaque sandbox.
// Artifact code never shares the bar's DOM. Eligible public shares may request
// same-context navigation; the wrapper has no app credentials or capabilities.
func (rd *Renderer) attributedShare(body string, a *store.Artifact, meta string, navigationAllowed bool) string {
	navigation := ""
	if navigationAllowed {
		navigation = `<script>
 // Only the approved child may spend live browser activation to navigate this
 // public share context. This grants no app API, popup or embedding-host authority.
 const tool=document.querySelector('iframe');
 addEventListener('message',event=>{
   if(event.source!==tool.contentWindow||event.origin!=='null')return;
   const data=event.data;
   if(!data||data.exhibitPublicNavigate!==true||typeof data.url!=='string')return;
   if(!navigator.userActivation||!navigator.userActivation.isActive)return;
   if(data.url.length>8192||data.url.trim()!==data.url)return;
   let destination;try{destination=new URL(data.url)}catch{return}
   if(!['https:','http:'].includes(destination.protocol)||!destination.hostname||destination.username||destination.password)return;
   location.assign(destination.href);
 });
 </script>`
	}
	// Let synchronous artifact handlers cancel before preventing native child
	// navigation. Register each bubble tail from the preceding target's tail,
	// so later document/window handlers (including ones added below) run first.
	body = prependToHead(body, fmt.Sprintf(`<script>
 document.addEventListener('click',event=>{
   const anchor=event.target.closest&&event.target.closest('a');
   if(!anchor||anchor.hasAttribute('download')||event.defaultPrevented||event.button!==0)return;
   const raw=anchor.getAttribute('href')??anchor.getAttributeNS('http://www.w3.org/1999/xlink','href');
   if(raw===null||raw.startsWith('#'))return;
   let destination;try{destination=new URL(raw,document.baseURI)}catch{return}
   if(!['https:','http:'].includes(destination.protocol))return;
   if(!%t||!event.isTrusted||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey||destination.username||destination.password){event.preventDefault();return}
   const path=event.composedPath();
   const removals=[];
   const cleanup=()=>{for(const remove of removals)remove()};
   // These listeners belong to this event, not a reentrant click. A task also
   // removes tails skipped by stopImmediatePropagation or early capture stops.
   const listen=(node,callback,capture=false)=>{
     const listener=observed=>{
       if(observed!==event)return;
       node.removeEventListener('click',listener,capture);
       callback();
     };
     node.addEventListener('click',listener,capture);
     removals.push(()=>node.removeEventListener('click',listener,capture));
   };
   const finish=()=>{
     const canceled=event.defaultPrevented;
     event.preventDefault();
     cleanup();
     if(!canceled)setTimeout(()=>{
       // The wrapper independently checks activation and this exact child.
       parent.postMessage({exhibitPublicNavigate:true,url:destination.href},'*');
     },0);
   };
   const bubbleTail=index=>listen(path[index],()=>{
     if(event.cancelBubble||index===path.length-1)finish();
     else bubbleTail(index+1);
   });
   // The target capture tail runs after existing target capture handlers and
   // appends its bubble tail after any handlers they installed for this click.
   listen(event.target,()=>{
     if(event.cancelBubble)finish();
     else bubbleTail(path.indexOf(event.target));
   },true);
   setTimeout(cleanup,0);
 },true);
 </script>`, navigationAllowed))

	// Opaque cross-origin frames do not consistently honor native autofocus.
	// Wait until the wrapper has focused the child, and never reclaim focus if
	// the viewer has already reached the attribution link.
	body = prependToHead(body, `<script>
 function focusInitialControl(){
   if(document.activeElement===document.body){
     const initial=document.querySelector('[autofocus]');
     if(initial)initial.focus();
   }
 }
 function scheduleInitialFocus(){
   requestAnimationFrame(()=>{if(document.hasFocus())focusInitialControl()});
 }
 addEventListener('load',scheduleInitialFocus,{once:true});
 addEventListener('focus',scheduleInitialFocus,{once:true});
 // srcdoc resolves fragment anchors against the outer share URL. Keep these
 // scrolls inside the tool instead of navigating away or requesting an app host.
 document.addEventListener('click',event=>{
   const anchor=event.target.closest&&event.target.closest('a');
   if(!anchor||event.defaultPrevented||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
   const href=anchor.getAttribute('href')??anchor.getAttributeNS('http://www.w3.org/1999/xlink','href');
   if(href===null||!href.startsWith('#'))return;
   event.preventDefault();
   if(href==='#'){scrollTo(0,0);return}
   let id;try{id=decodeURIComponent(href.slice(1))}catch{return}
   const target=document.getElementById(id)||document.getElementsByName(id)[0];
   if(target)target.scrollIntoView();
 });
 </script>`)
	mark := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(rd.cfg.FaviconSVG))
	return fmt.Sprintf(`<!doctype html><html lang="en"><head>%s<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title><link rel="icon" href="%s"><style>
 :root{color-scheme:light dark;font:12px/1.4 system-ui,sans-serif;background:#fff;color:#414751}
 *{box-sizing:border-box}html,body{margin:0;width:100%%;height:100%%;overflow:hidden}
 body{display:grid;grid-template-rows:36px minmax(0,1fr)}
 header{display:flex;align-items:center;justify-content:center;border-bottom:1px solid #e2e5e9;background:#f8f9fb}
 a{display:inline-flex;gap:7px;align-items:center;color:inherit;text-decoration:none;padding:4px 8px;border-radius:4px}
 a[href]{text-decoration:underline;text-underline-offset:2px}a:focus-visible{outline:2px solid #2463c5;outline-offset:1px}
 img{width:16px;height:20px;object-fit:contain}iframe{display:block;border:0;width:100%%;height:100%%;min-height:0}
 @media(prefers-color-scheme:dark){:root{background:#17191e;color:#c8cdd5}header{background:#202329;border-color:#363b44}a:focus-visible{outline-color:#8ab4f8}}
 </style></head><body><header aria-label="Exhibit attribution"><a id="exhibit-attribution" href="%s" rel="noreferrer"><img src="%s" alt="">made with exhibit</a></header><iframe title="%s" sandbox="allow-scripts allow-forms" allow="fullscreen *; clipboard-write *" onload="if(window.top===window&&(document.activeElement===document.body||document.activeElement===this)){this.focus()}" srcdoc="%s"></iframe><script>
 if(window.self!==window.top){const a=document.getElementById('exhibit-attribution');a.removeAttribute('href');a.title='Exhibit attribution — open the share directly to visit Exhibit';}
 </script>%s</body></html>`, meta, html.EscapeString(a.Title), artifacticon.DataURI(a.Tags), ExhibitProductOrigin, mark, html.EscapeString(a.Title), html.EscapeString(body), navigation)
}
