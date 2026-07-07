package server

import (
	"net/http"
	"strings"
)

// serveConsole renders the collaboration console: the live app framed as a phone
// on the left, and a terminal-style activity-log window on the right showing who
// (human/agent) did what in the shared session. This is what the desktop app
// opens; it mirrors the polished demo layout.
func (s *Server) serveConsole(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	name := s.rt.App.Name
	s.mu.Unlock()
	if name == "" {
		name = "QORM"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(strings.ReplaceAll(consoleHTML, "{{title}}", htmlEscape(name))))
}

const consoleHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{title}} — collaboration console</title>
<style>
  :root{ --page:#ececef; --ink:#16171b; --muted:#6a6d76; --brand:#0a84ff; --live:#1eb854;
    --agent:#5ac8fa; --human:#30d158; }
  @media (prefers-color-scheme:dark){ :root{ --page:#0b0c0f; --ink:#eef0f3; --muted:#969aa4; } }
  *{margin:0;padding:0;box-sizing:border-box;}
  body{height:100vh;overflow:hidden;background:var(--page);color:var(--ink);
    font-family:-apple-system,BlinkMacSystemFont,"SF Pro Text","Helvetica Neue",Arial,sans-serif;
    -webkit-font-smoothing:antialiased;letter-spacing:-.011em;
    display:flex;align-items:center;justify-content:center;gap:48px;padding:32px;}
  @media (max-width:820px){ body{flex-direction:column;gap:20px;padding:16px;overflow:auto;} }

  /* phone */
  .device{display:flex;flex-direction:column;align-items:center;gap:12px;flex:none;}
  .phone{width:312px;height:646px;background:#05060a;border:1px solid #24262c;border-radius:48px;
    padding:11px;box-shadow:0 34px 80px -26px rgba(0,0,0,.55);position:relative;}
  .phone::before{content:"";position:absolute;top:16px;left:50%;transform:translateX(-50%);
    width:100px;height:27px;background:#05060a;border-radius:0 0 17px 17px;z-index:5;}
  .screen{width:100%;height:100%;border-radius:37px;overflow:hidden;background:#fff;position:relative;display:flex;flex-direction:column;}
  .statusbar{height:46px;flex:none;display:flex;align-items:flex-end;justify-content:space-between;
    padding:0 26px 6px;font-size:14px;font-weight:600;color:#000;background:#f2f2f7;
    position:relative;z-index:2;}
  .statusbar .r{letter-spacing:1px;}
  .screen iframe{flex:1;width:100%;border:0;display:block;}
  .caption{font-size:13px;color:var(--muted);}
  .caption b{color:var(--ink);font-weight:600;}

  /* console */
  .console{width:420px;height:646px;background:#0c0e13;border:1px solid #23262e;border-radius:22px;
    overflow:hidden;display:flex;flex-direction:column;box-shadow:0 34px 80px -30px rgba(0,0,0,.5);flex:none;}
  @media (max-width:820px){ .console{width:312px;height:300px;} }
  .con-head{display:flex;align-items:center;gap:9px;padding:16px 18px;border-bottom:1px solid #22252d;color:#e6e8ee;}
  .con-head .dot{width:9px;height:9px;border-radius:50%;background:var(--live);box-shadow:0 0 0 0 var(--live);animation:pulse 2s infinite;}
  @keyframes pulse{0%{box-shadow:0 0 0 0 rgba(30,184,84,.5);}70%{box-shadow:0 0 0 9px transparent;}100%{box-shadow:0 0 0 0 transparent;}}
  .con-head b{font-size:14.5px;} .con-head small{margin-left:auto;color:#7d8493;font-size:12.5px;}
  .legend{display:flex;gap:14px;padding:8px 18px;border-bottom:1px solid #22252d;font-size:12px;color:#7d8493;}
  .legend .k{display:inline-flex;align-items:center;gap:6px;}
  .legend .sw{width:8px;height:8px;border-radius:50%;}
  .log{flex:1;overflow-y:auto;padding:14px 18px;font-family:ui-monospace,"SF Mono",Menlo,monospace;
    font-size:12.5px;line-height:1.75;color:#c7ccd6;}
  .log .e{display:flex;gap:9px;padding:2px 0;animation:in .25s ease;}
  @keyframes in{from{opacity:0;transform:translateY(4px);}to{opacity:1;transform:none;}}
  .log .t{color:#5a616e;flex:none;font-variant-numeric:tabular-nums;}
  .log .who{flex:none;width:50px;font-weight:600;text-transform:lowercase;}
  .log .agent .who{color:var(--agent);} .log .human .who{color:var(--human);} .log .system .who{color:#8a93a3;}
  .log .d{color:#dfe3ea;word-break:break-word;}
  .con-foot{padding:13px 18px;border-top:1px solid #22252d;color:#7d8493;font-size:12px;}
  .con-foot code{color:#9aa2b1;background:#161922;padding:1px 6px;border-radius:5px;}
</style></head>
<body>
  <div class="device">
    <div class="phone"><div class="screen"><div class="statusbar"><span>9:41</span><span class="r">5G   ᛒ</span></div><iframe src="/" title="{{title}}"></iframe></div></div>
    <div class="caption"><b>{{title}}</b> · live app — tap it, it's real</div>
  </div>

  <aside class="console">
    <div class="con-head"><span class="dot"></span><b>Collaboration log</b><small>one shared session</small></div>
    <div class="legend">
      <span class="k"><span class="sw" style="background:var(--human)"></span>you</span>
      <span class="k"><span class="sw" style="background:var(--agent)"></span>AI agent</span>
      <span class="k"><span class="sw" style="background:#8a93a3"></span>system</span>
    </div>
    <div class="log" id="log"><div class="e system"><span class="t">--:--:--</span><span class="who">system</span><span class="d">connected — waiting for activity…</span></div></div>
    <div class="con-foot">Human taps and agent MCP calls on the <code>same</code> app appear here, live.</div>
  </aside>
<script>
  var log=document.getElementById('log'), since=0, first=true;
  function add(e){
    var d=document.createElement('div'); d.className='e '+e.source;
    d.innerHTML='<span class="t"></span><span class="who"></span><span class="d"></span>';
    d.querySelector('.t').textContent=e.time;
    d.querySelector('.who').textContent=(e.source==='human'?'you':e.source);
    d.querySelector('.d').textContent=e.detail;
    log.appendChild(d); log.scrollTop=log.scrollHeight;
  }
  function poll(){
    fetch('/log?since='+since).then(function(r){return r.json();}).then(function(rows){
      if(rows && rows.length){ if(first){ log.innerHTML=''; first=false; }
        rows.forEach(function(e){ since=Math.max(since,e.seq); add(e); }); }
    }).catch(function(){});
  }
  setInterval(poll,600); poll();
</script>
</body></html>`

// serveLogWindow renders a standalone, full-window activity log — the SEPARATE
// log window that accompanies the real app window in the desktop app.
func (s *Server) serveLogWindow(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	name := s.rt.App.Name
	s.mu.Unlock()
	if name == "" {
		name = "QORM"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(strings.ReplaceAll(logWindowHTML, "{{title}}", htmlEscape(name))))
}

const logWindowHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{title}} — activity log</title>
<style>
  *{margin:0;padding:0;box-sizing:border-box;}
  html,body{height:100%;}
  body{background:#0c0e13;color:#c7ccd6;display:flex;flex-direction:column;
    font-family:ui-monospace,"SF Mono",Menlo,monospace;-webkit-font-smoothing:antialiased;}
  header{display:flex;align-items:center;gap:9px;padding:14px 16px;border-bottom:1px solid #22252d;
    color:#e6e8ee;font-family:-apple-system,BlinkMacSystemFont,sans-serif;}
  header .dot{width:9px;height:9px;border-radius:50%;background:#1eb854;animation:pulse 2s infinite;}
  @keyframes pulse{0%{box-shadow:0 0 0 0 rgba(30,184,84,.5);}70%{box-shadow:0 0 0 9px transparent;}100%{box-shadow:0 0 0 0 transparent;}}
  header b{font-size:14px;} header small{margin-left:auto;color:#7d8493;font-size:12px;}
  .legend{display:flex;gap:14px;padding:8px 16px;border-bottom:1px solid #22252d;font-size:12px;color:#7d8493;
    font-family:-apple-system,sans-serif;}
  .legend .k{display:inline-flex;align-items:center;gap:6px;} .legend .sw{width:8px;height:8px;border-radius:50%;}
  #log{flex:1;overflow-y:auto;padding:12px 16px;font-size:12.5px;line-height:1.8;}
  .e{display:flex;gap:9px;animation:in .25s ease;}
  @keyframes in{from{opacity:0;transform:translateY(4px);}to{opacity:1;transform:none;}}
  .t{color:#5a616e;flex:none;} .who{flex:none;width:50px;font-weight:600;}
  .agent .who{color:#5ac8fa;} .human .who{color:#30d158;} .system .who{color:#8a93a3;} .app .who{color:#ffd60a;}
  .d{color:#dfe3ea;word-break:break-word;}
  .ctrl{display:flex;align-items:center;gap:6px;flex-wrap:wrap;padding:8px 16px;border-bottom:1px solid #22252d;
    font-family:-apple-system,sans-serif;}
  .ctrl .cl{color:#7d8493;font-size:12px;margin-right:4px;}
  .ctrl button{background:#1a1d24;color:#c7ccd6;border:1px solid #2c313b;border-radius:7px;
    padding:5px 10px;font-size:12px;cursor:pointer;}
  .ctrl button:hover{background:#242832;color:#fff;}
  .pres{padding:8px 16px;border-bottom:1px solid #22252d;font-size:12px;color:#9aa2b1;font-family:-apple-system,sans-serif;}
  .pres .lbl{color:#7d8493;} .pres b{color:#30d158;font-weight:600;} .pres .pw{color:#ffd60a;}
</style></head><body>
  <header><span class="dot"></span><b>Activity log</b><small>shared session</small></header>
  <div class="legend">
    <span class="k"><span class="sw" style="background:#30d158"></span>you</span>
    <span class="k"><span class="sw" style="background:#5ac8fa"></span>AI agent</span>
    <span class="k"><span class="sw" style="background:#8a93a3"></span>system</span>
  </div>
  <div class="ctrl">
    <span class="cl">window</span>
    <button onclick="qw(40,40,400,820)"> left</button>
    <button onclick="qw((screen.width-400)/2,(screen.height-820)/2,400,820)"> center</button>
    <button onclick="qw(screen.width-440,40,400,820)">right </button>
    <button onclick="qw(0,0,screen.availWidth/2,screen.availHeight)"> half-L</button>
    <button onclick="qw(screen.availWidth/2,0,screen.availWidth/2,screen.availHeight)">half-R </button>
    <button onclick="qw(0,0,screen.availWidth,screen.availHeight)"> max</button>
    <button onclick="qw(40,40,900,680)"> wide</button>
    <button onclick="qw(40,40,400,820)"> phone</button>
    <button onclick="qo(&quot;focus&quot;)">⤒ focus</button>
    <button onclick="qo(&quot;minimize&quot;)">— min</button>
    <button onclick="qo(&quot;pin&quot;)"> pin</button>
    <button onclick="qo(&quot;unpin&quot;)">unpin</button>
    <span class="cl" style="margin-left:8px">multi</span>
    <button onclick="qopen(&quot;win&quot;+(++qn),location.origin+&quot;/&quot;,400,600)">＋ window</button>
    <button onclick="qo(&quot;tile&quot;)"> tile all</button>
  </div>
  <div class="pres"><span class="lbl">shared with the AI:</span> <span id="qpres">nothing yet</span></div>
  <div id="log"><div class="e system"><span class="t">--:--:--</span><span class="who">system</span><span class="d">waiting for activity…</span></div></div>
<script>
  var log=document.getElementById('log'),since=0,first=true;
  function poll(){fetch('/log?since='+since).then(function(r){return r.json();}).then(function(rows){
    if(rows&&rows.length){if(first){log.innerHTML='';first=false;}
      rows.forEach(function(e){since=Math.max(since,e.seq);
        var d=document.createElement('div');d.className='e '+e.source;
        d.innerHTML='<span class="t"></span><span class="who"></span><span class="d"></span>';
        d.querySelector('.t').textContent=e.time;d.querySelector('.who').textContent=(e.source==='human'?'you':e.source);
        d.querySelector('.d').textContent=e.detail;log.appendChild(d);});
      log.scrollTop=log.scrollHeight;}
  }).catch(function(){});}
  function qw(x,y,w,h){fetch('/window',{method:'POST',body:JSON.stringify({op:'move',x:Math.round(x),y:Math.round(y),w:w,h:h})}).catch(function(){});}
  var qn=1;
  function qo(op){fetch('/window',{method:'POST',body:JSON.stringify({op:op})}).catch(function(){});}
  function qopen(id,url,w,h){fetch('/window',{method:'POST',body:JSON.stringify({op:'open',id:id,url:url,w:w,h:h})}).catch(function(){});}
  setInterval(poll,600);poll();
  // Show the human what the agent can currently see about them (transparency).
  function esc(s){ return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;'); }
  function qpres(){ fetch('/presence').then(function(r){return r.json();}).then(function(p){
    var el=document.getElementById('qpres'); if(!el) return; var parts=[];
    if(p.focus) parts.push('on <b>'+esc(p.focus)+'</b>');
    if(p.typing) parts.push('typed <b>'+esc(p.typing)+'</b>');
    if(p.filled) parts.push('<span class="pw">'+esc(p.filled)+' filled (value hidden)</span>');
    el.innerHTML = parts.length ? parts.join(' &middot; ') : 'nothing yet';
  }).catch(function(){}); }
  setInterval(qpres,900); qpres();
</script></body></html>`
