package api

import (
	"encoding/base64"
	"fmt"

	"github.com/momja/Exhibit/internal/color"
)

// exhibitLogoSVG is the Exhibit brand mark, inlined here so it travels with
// the binary as source; the gallery template injects it as trusted markup
// (template.HTML, see renderGalleryPage) rather than loading it as an asset.
//
// It is the design_files/exhibit_logo.svg artwork with editor-only cruft
// (Inkscape/sodipodi metadata, the XML prolog, export hints) stripped; every
// presentation attribute is preserved verbatim so it renders identically. The
// root carries only viewBox (no width/height) so CSS can size it, plus role +
// aria-label for an accessible name. It is used two ways: inlined directly in
// the header, and base64-encoded into the favicon data URI below.
var exhibitLogoSVG = fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 40.316795 49.983387" role="img" aria-label="Exhibit logo" class="logo" focusable="false">
  <defs>
    <linearGradient id="linearGradient294">
      <stop style="stop-color:#808080;stop-opacity:1" offset="0"/>
      <stop style="stop-color:#000000;stop-opacity:0" offset="1"/>
    </linearGradient>
    <linearGradient id="linearGradient292">
      <stop style="stop-color:#808080;stop-opacity:1" offset="0.39322534"/>
      <stop style="stop-color:#808080;stop-opacity:0" offset="1"/>
    </linearGradient>
    <linearGradient id="linearGradient291">
      <stop style="stop-color:#000000;stop-opacity:1" offset="0"/>
      <stop style="stop-color:#000000;stop-opacity:0" offset="0.68021381"/>
    </linearGradient>
    <filter style="color-interpolation-filters:sRGB" id="filter30" x="-0.019211887" y="-0.019468884" width="1.0757802" height="1.0756993">
      <feFlood result="flood" in="SourceGraphic" flood-opacity="0.498039" flood-color="rgb(0,0,0)"/>
      <feGaussianBlur result="blur" in="SourceGraphic" stdDeviation="0.300000"/>
      <feOffset result="offset" in="blur" dx="1.400000" dy="1.359517"/>
      <feComposite result="comp1" operator="in" in="flood" in2="offset"/>
      <feComposite result="comp2" operator="over" in="SourceGraphic" in2="comp1"/>
    </filter>
    <radialGradient xlink:href="#linearGradient291" id="radialGradient292" cx="64.152306" cy="38.879955" fx="64.152306" fy="38.879955" r="0.90295792" gradientUnits="userSpaceOnUse" gradientTransform="matrix(5.0437911,5.2364045,-4.5993467,4.430167,-86.154751,-467.75459)"/>
    <radialGradient xlink:href="#linearGradient292" id="radialGradient294" cx="64.152313" cy="38.879951" fx="64.152313" fy="38.879948" r="0.90295792" gradientUnits="userSpaceOnUse" gradientTransform="matrix(2.9795058,0,0,2.9795058,-132.5485,-75.425508)"/>
    <radialGradient xlink:href="#linearGradient294" id="radialGradient296" cx="60.668537" cy="43.098862" fx="60.668537" fy="43.098862" r="4.7164111" gradientUnits="userSpaceOnUse" gradientTransform="matrix(-1.5026119,5.749967,-1.9159477,-0.50068562,235.03101,-297.57977)"/>
  </defs>
  <g transform="translate(-36.08545,-36.625489)">
    <rect style="fill:#808080;fill-opacity:1;stroke:#808080;stroke-width:0;stroke-linecap:round;stroke-dasharray:none;paint-order:stroke markers fill;filter:url(#filter30)" width="37.476799" height="36.98209" x="36.80545" y="47.547268"/>
    <path style="color:#000000;fill:url(#radialGradient296);stroke-width:2.43232;stroke-linecap:round;-inkscape-stroke:none" d="M 58.467752,40.747145 45.560307,58.310204 a 0.26725721,0.26725721 0 0 0 0.05701,0.375298 0.26725721,0.26725721 0 0 0 0.375301,-0.05701 L 58.41075,41.72577 67.950003,58.376709 a 0.26725721,0.26725721 0 0 0 0.3658,0.09977 0.26725721,0.26725721 0 0 0 0.09977,-0.365799 z"/>
    <rect style="fill:#ffffff;fill-opacity:1;stroke:#000000;stroke-width:1.00649;stroke-linecap:round;stroke-dasharray:none" width="37.476799" height="36.98209" x="36.80545" y="47.547268"/>
    <rect style="fill:%s;fill-opacity:1;stroke:#000000;stroke-width:1.99229;stroke-linecap:round;stroke-dasharray:none;paint-order:stroke markers fill" width="12.447359" height="16.068565" x="61.341995" y="67.967896"/>
    <circle style="fill:url(#radialGradient294);stroke:url(#radialGradient292);stroke-width:0;stroke-linecap:round;stroke-dasharray:none;paint-order:stroke markers fill" cx="58.593651" cy="40.417534" r="2.6903682"/>
    <path style="font-weight:bold;font-size:41.0282px;line-height:0;font-family:'.Diwan Kufi PUA';text-align:center;letter-spacing:2.44032px;text-anchor:middle;fill:%s;stroke:#000000;stroke-width:1.98523;stroke-linecap:round;stroke-dasharray:none;paint-order:stroke markers fill" d="M 60.383501,72.781407 H 37.290736 V 48.036637 H 60.383501 V 52.82273 H 45.481485 v 4.270925 h 13.829928 v 4.786094 H 45.481485 v 6.115565 h 14.902016 z" aria-label="E"/>
    <path style="fill:none;fill-opacity:1;stroke:#000000;stroke-width:1;stroke-linecap:round;stroke-dasharray:none" d="m 50.96285,47.859368 5.356268,-7.874052 4.90919,8.007163"/>
    <rect style="fill:%s;fill-opacity:1;stroke:#000000;stroke-width:0;stroke-linecap:round;stroke-dasharray:none;stroke-opacity:1;paint-order:stroke markers fill" width="23.057453" height="10.245288" x="37.318401" y="73.774017"/>
    <circle style="fill:#000000;fill-opacity:1;stroke:#000000;stroke-width:0;stroke-linecap:round;stroke-dasharray:none;paint-order:stroke markers fill" cx="56.133476" cy="38.734268" r="2.1087794"/>
  </g>
</svg>`, color.BrandBlue, color.BrandRed, color.BrandYellow)

// exhibitFaviconDataURI is the same artwork encoded for a <link rel="icon">.
// base64 sidesteps the URL-escaping the SVG's many '#' color values would
// otherwise need in a data URI. Rendered in the favicon's own document context,
// so its element ids never collide with the inline header copy.
var exhibitFaviconDataURI = "data:image/svg+xml;base64," +
	base64.StdEncoding.EncodeToString([]byte(exhibitLogoSVG))
