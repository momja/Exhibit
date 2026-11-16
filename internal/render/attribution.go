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
// Artifact code never shares the bar's DOM. No message bridge is installed.
func (rd *Renderer) attributedShare(body string, a *store.Artifact, meta string) string {
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
   const anchor=event.target.closest&&event.target.closest('a[href]');
   if(!anchor||event.defaultPrevented||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
   const href=anchor.getAttribute('href');
   if(!href.startsWith('#'))return;
   event.preventDefault();
   if(href==='#'){scrollTo(0,0);return}
   let id;try{id=decodeURIComponent(href.slice(1))}catch{return}
   const target=document.getElementById(id)||document.getElementsByName(id)[0];
   if(target)target.scrollIntoView();
 });
 </script>`)
	mark := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(rd.cfg.FaviconSVG))
	return fmt.Sprintf(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title>%s<link rel="icon" href="%s"><style>
 :root{color-scheme:light dark;font:12px/1.4 system-ui,sans-serif;background:#fff;color:#414751}
 *{box-sizing:border-box}html,body{margin:0;width:100%%;height:100%%;overflow:hidden}
 body{display:grid;grid-template-rows:36px minmax(0,1fr)}
 header{display:flex;align-items:center;justify-content:center;border-bottom:1px solid #e2e5e9;background:#f8f9fb}
 a{display:inline-flex;gap:7px;align-items:center;color:inherit;text-decoration:none;padding:4px 8px;border-radius:4px}
 a[href]:hover{text-decoration:underline}a:focus-visible{outline:2px solid #2463c5;outline-offset:1px}
 img{width:16px;height:20px;object-fit:contain}iframe{display:block;border:0;width:100%%;height:100%%;min-height:0}
 @media(prefers-color-scheme:dark){:root{background:#17191e;color:#c8cdd5}header{background:#202329;border-color:#363b44}a:focus-visible{outline-color:#8ab4f8}}
 </style></head><body><header aria-label="Exhibit attribution"><a id="exhibit-attribution" href="%s" rel="noreferrer"><img src="%s" alt="">made with exhibit</a></header><iframe title="%s" sandbox="allow-scripts allow-forms" allow="fullscreen *; clipboard-write *" allowfullscreen onload="if(document.activeElement===document.body||document.activeElement===this){this.focus()}" srcdoc="%s"></iframe><script>
 if(window.self!==window.top){const a=document.getElementById('exhibit-attribution');a.removeAttribute('href');a.title='Exhibit attribution — open the share directly to visit Exhibit';}
 </script></body></html>`, html.EscapeString(a.Title), meta, artifacticon.DataURI(a.Tags), ExhibitProductOrigin, mark, html.EscapeString(a.Title), html.EscapeString(body))
}
