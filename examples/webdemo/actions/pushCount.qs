// Go → page: the QORM button drives the embedded page's DOM from qscript —
// one eval writes the counter text (DOM), flips its colour (style), and calls
// the page's own qormOnNetwork-like function pattern (script), then a second
// eval posts a probe back so the host log shows the round trip.
state.count = state.count + 1
set hue "#4da3ff"
if (state.count % 2 == 1) { set hue "#ff9f4d" }
native("webviewEval", {
  "id": "page",
  "js": "document.getElementById('s').textContent = 'from QORM: count = ' + " + str(state.count) + ";document.getElementById('s').style.color = '" + hue + "'"
})
native("webviewEval", {
  "id": "page",
  "js": "window.qormDesktop(JSON.stringify({op:'probe',note:'eval-ran'}))"
})
