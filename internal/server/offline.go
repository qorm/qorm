package server

import (
	"fmt"
	"strings"

	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/runtime"
)

// OfflineHTML produces a fully standalone index.html for an installable package:
// it reuses the exact same theme CSS, DOM-morph engine, and gesture helpers as
// the live server Page, but swaps the fetch('/event') driver for an in-process
// Go-WASM call (qormEvent) and drops the server-only live-sync + self-measure.
// The app runs entirely client-side — no server — so it can be wrapped in a
// PWA / APK / IPA. bundleJSON is the app compiled with bundle.Build+Marshal.
func OfflineHTML(rt *runtime.Runtime, bundleJSON string) (string, error) {
	page := Page(rt, render.Render(rt).HTML, 0)

	// 1. online dispatch (POST /event) -> in-process WASM dispatch
	onlineDriver := `  fetch('/event',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},
    body:JSON.stringify({h:h,inputs:inputs})})
    .then(function(r){ var rv=parseInt(r.headers.get('X-Qorm-Rev'))||0; var nav=r.headers.get('X-Qorm-Nav')||''; qormTheme(r.headers.get('X-Qorm-Theme')); return r.text().then(function(html){ return {rv:rv,html:html,nav:nav}; }); })
    .then(function(o){ if(o.rv && o.rv<=__rev) return; if(o.rv) __rev=o.rv; window.__qormNav=o.nav; qormMorphInto(document.getElementById('qorm-root'), o.html); });`
	offlineDriver := `  var res=qormEvent(h, JSON.stringify(inputs));
  if(res){ qormTheme(res.theme); qormDir(res.dir); qormMorphInto(document.getElementById('qorm-root'), res.html); if(typeof qormMeasure!=='undefined') setTimeout(qormMeasure,30); }`
	page, err := replaceOnce(page, onlineDriver, offlineDriver)
	if err != nil {
		return "", err
	}

	// 2. drop live-sync (SSE/poll) — an offline app has no server to sync with
	liveSync := `if(window.EventSource){
  var es=new EventSource('/events');
  es.onmessage=function(e){ try{ qormApply(JSON.parse(e.data)); }catch(_){} };
}else{
  setInterval(function(){
    fetch('/poll?rev='+__rev).then(function(r){return r.json();}).then(qormApply).catch(function(){});
  }, 800);
}`
	page, err = replaceOnce(page, liveSync, "")
	if err != nil {
		return "", err
	}

	// 3. self-measure (qormMeasure -> POST /measure) is harmless offline (the
	//    fetch just fails with no server) and lets the same framework harness
	//    verify a packaged app when it IS served with a /measure sink — so we
	//    keep it rather than strip it.

	// 4. load the WASM runtime + boot it from the SEPARATE bundle.json (the
	//    JSON stays its own artifact — inspectable, cacheable, and what OTA
	//    swaps — rather than being inlined into the HTML). Injected just before
	//    </script> so all helpers (morph, gestures) are defined.
	_ = bundleJSON // the bundle is written to bundle.json by the packager, not inlined
	boot := `
function qormDir(d){ document.documentElement.setAttribute('dir', d||'ltr'); }
(function(){
  var go=new Go();
  Promise.all([
    fetch('qorm.wasm').then(function(r){ return r.arrayBuffer(); }),
    fetch('bundle.json').then(function(r){ return r.text(); })
  ]).then(function(res){
    return WebAssembly.instantiate(res[0], go.importObject).then(function(w){
      go.run(w.instance);
      var r=qormInit(res[1]);
      if(r){ qormTheme(r.theme); qormDir(r.dir); qormMorphInto(document.getElementById('qorm-root'), r.html); if(typeof qormMeasure!=='undefined') setTimeout(qormMeasure,40); }
    });
  }).catch(function(e){ document.getElementById('qorm-root').innerHTML='<div style="padding:20px">app load failed: '+e+'</div>'; });
})();
`
	page, err = replaceOnce(page, "\n</script>\n</body>", boot+"</script>\n</body>")
	if err != nil {
		return "", err
	}

	// 5. head: load wasm_exec.js before the main script, add PWA manifest +
	//    Apple standalone metas so "Add to Home Screen" launches full-screen.
	// "Made with QORM" generator note in the packaged app's metadata (not UI).
	// Removing it is a commercial white-label feature (App.Branding=false) — see TERMS.
	gen := ""
	if rt.App.Branding {
		gen = "\n<meta name=\"generator\" content=\"Made with QORM — https://qorm.com\">"
	}
	head := `<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no, viewport-fit=cover">`
	pwa := `<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#000000">
<meta name="mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="default">
<meta name="apple-mobile-web-app-title" content="` + htmlEscape(rt.App.Name) + `">` + gen + `
<link rel="manifest" href="manifest.webmanifest">
<link rel="apple-touch-icon" href="icon-192.png">
<script src="wasm_exec.js"></script>`
	page, err = replaceOnce(page, head, pwa)
	if err != nil {
		return "", err
	}

	// The app's own native/web.js (custom qormOn<X> callbacks + button wiring)
	// travels with the offline package too, so custom Go/WASM ops round-trip on
	// device. Injected last so it doesn't disturb the anchor-based rewrites above.
	if js := userWebJS(rt); js != "" {
		page, err = replaceOnce(page, "</body>", "<script>"+js+"</script>\n</body>")
		if err != nil {
			return "", err
		}
	}
	return page, nil
}

// replaceOnce replaces exactly one occurrence of old, erroring if the anchor is
// absent — so any drift in the Page template is caught loudly at build time.
func replaceOnce(s, old, new string) (string, error) {
	if !strings.Contains(s, old) {
		return "", fmt.Errorf("offline: page anchor not found: %.60q", old)
	}
	return strings.Replace(s, old, new, 1), nil
}
