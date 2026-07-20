package server

import (
	"encoding/json"
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
	html := strings.ReplaceAll(consoleHTML, "{{title}}", htmlEscape(name))
	// The DevTool is a human-side surface: it gets the same event token as the
	// app page so its writes (/dev/state, /dev/highlight) are authenticated.
	html = strings.ReplaceAll(html, "{{token}}", s.eventToken)
	w.Write([]byte(html))
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
    <div class="caption"><b>{{title}}</b> · <span data-i18n="caption">live app — tap it, it's real</span></div>
  </div>

  <aside class="console">
    <div class="con-head"><span class="dot"></span><b data-i18n="head">Collaboration log</b><small data-i18n="headSub">one shared session</small><select id="langsel" onchange="setLang(this.value)" style="margin-left:10px;background:#161922;color:#c7ccd6;border:1px solid #2c313b;border-radius:7px;padding:2px 6px;font-size:11px;cursor:pointer;"></select></div>
    <div class="legend">
      <span class="k"><span class="sw" style="background:var(--human)"></span><span data-i18n="legendYou">you</span></span>
      <span class="k"><span class="sw" style="background:var(--agent)"></span><span data-i18n="legendAgent">AI agent</span></span>
      <span class="k"><span class="sw" style="background:#8a93a3"></span><span data-i18n="legendSystem">system</span></span>
    </div>
    <div class="log" id="log"><div class="e system"><span class="t">--:--:--</span><span class="who">system</span><span class="d" data-i18n="waiting">connected — waiting for activity…</span></div></div>
    <div class="con-foot" data-i18n="foot">Human taps and agent MCP calls on the <code>same</code> app appear here, live.</div>
  </aside>
<script>
  var LANGNAMES={en:'English',zh:'中文',ja:'日本語',ko:'한국어',es:'Español',fr:'Français',de:'Deutsch'};
  var I18N={
    en:{caption:"live app — tap it, it's real",head:'Collaboration log',headSub:'one shared session',legendYou:'you',legendAgent:'AI agent',legendSystem:'system',waiting:'connected — waiting for activity…',foot:'Human taps and agent MCP calls on the {same} app appear here, live.'},
    zh:{caption:'真实应用——点它,是真的',head:'协作日志',headSub:'同一个共享会话',legendYou:'你',legendAgent:'AI 智能体',legendSystem:'系统',waiting:'已连接——等待活动…',foot:'你的点击与 agent 的 MCP 调用都会实时出现在这里({same} 应用)。'},
    ja:{caption:'実アプリ——タップできます',head:'コラボレーションログ',headSub:'1つの共有セッション',legendYou:'あなた',legendAgent:'AI エージェント',legendSystem:'システム',waiting:'接続しました——アクティビティを待っています…',foot:'人間のタップと agent の MCP 呼び出しが {same} アプリにライブ表示されます。'},
    ko:{caption:'실제 앱입니다——탭해 보세요',head:'협업 로그',headSub:'하나의 공유 세션',legendYou:'나',legendAgent:'AI 에이전트',legendSystem:'시스템',waiting:'연결됨——활동 대기 중…',foot:'사람의 탭과 에이전트의 MCP 호출이 {same} 앱에서 여기에 실시간으로 표시됩니다.'},
    es:{caption:'app real en vivo — tócala, es de verdad',head:'Registro de colaboración',headSub:'una sesión compartida',legendYou:'tú',legendAgent:'agente IA',legendSystem:'sistema',waiting:'conectado — esperando actividad…',foot:'Los toques humanos y las llamadas MCP del agente en la {same} app aparecen aquí en vivo.'},
    fr:{caption:"app réelle en direct — touchez-la, elle fonctionne",head:'Journal de collaboration',headSub:'une seule session partagée',legendYou:'vous',legendAgent:'agent IA',legendSystem:'système',waiting:"connecté — en attente d'activité…",foot:"Les taps humains et les appels MCP de l'agent sur la {same} app apparaissent ici en direct."},
    de:{caption:'echte Live-App — antippen, sie ist echt',head:'Kollaborationsprotokoll',headSub:'eine gemeinsame Sitzung',legendYou:'du',legendAgent:'KI-Agent',legendSystem:'System',waiting:'verbunden — warte auf Aktivität…',foot:'Menschliche Taps und Agent-MCP-Aufrufe in {same} App erscheinen hier live.'}
  };
  function curLang(){ try{ var l=localStorage.getItem('qorm-lang'); if(l&&I18N[l]) return l; }catch(e){}
    var n=(navigator.language||'en').toLowerCase().split('-')[0]; return I18N[n]?n:'en'; }
  var LANG=curLang();
  function T(k){ return (I18N[LANG]&&I18N[LANG][k])||I18N.en[k]||k; }
  function applyLang(){ document.documentElement.lang=LANG;
    document.querySelectorAll('[data-i18n]').forEach(function(el){
      var k=el.getAttribute('data-i18n');
      if(k==='foot'){ el.innerHTML=T('foot').replace('{same}','<code>same</code>'); }
      else { el.textContent=T(k); }
    });
    var s=document.getElementById('langsel');
    if(s && !s.options.length){ Object.keys(LANGNAMES).forEach(function(k){ var o=document.createElement('option'); o.value=k; o.textContent=LANGNAMES[k]; s.appendChild(o); }); }
    if(s) s.value=LANG; }
  function setLang(l){ if(!I18N[l]) return; LANG=l; try{ localStorage.setItem('qorm-lang',l); }catch(e){} applyLang(); }
  var log=document.getElementById('log'), since=0, first=true;
  function add(e){
    var d=document.createElement('div'); d.className='e '+e.source;
    d.innerHTML='<span class="t"></span><span class="who"></span><span class="d"></span>';
    d.querySelector('.t').textContent=e.time;
    d.querySelector('.who').textContent=(e.source==='human'?T('legendYou'):e.source);
    d.querySelector('.d').textContent=e.detail;
    log.appendChild(d); log.scrollTop=log.scrollHeight;
  }
  function poll(){
    fetch('/log?since='+since).then(function(r){return r.json();}).then(function(rows){
      if(rows && rows.length){ if(first){ log.innerHTML=''; first=false; }
        rows.forEach(function(e){ since=Math.max(since,e.seq); add(e); }); }
    }).catch(function(){});
  }
  setInterval(poll,600); poll(); applyLang();
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
	s.actMu.Lock()
	logsData, _ := json.Marshal(s.activity)
	s.actMu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := strings.ReplaceAll(logWindowHTML, "{{title}}", htmlEscape(name))
	html = strings.ReplaceAll(html, "{{initial_logs}}", string(logsData))
	w.Write([]byte(html))
}

const logWindowHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{title}} — QORM DevTool</title>
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
  
  /* Tab Bar Styles */
  .tabs-bar{display:flex;background:#13161c;border-bottom:1px solid #22252d;padding:0 8px;gap:4px;
    font-family:-apple-system,BlinkMacSystemFont,sans-serif;}
  .tab-btn{background:none;border:none;color:#7d8493;padding:10px 14px;font-size:13px;cursor:pointer;
    border-bottom:2px solid transparent;transition:all .15s ease;font-weight:500;}
  .tab-btn:hover{color:#dfe3ea;}
  .tab-btn.active{color:#0a84ff;border-bottom-color:#0a84ff;}
  
  .panel-body{flex:1;overflow:hidden;position:relative;display:flex;flex-direction:column;}
  .tab-panel{display:none;height:100%;width:100%;overflow-y:auto;padding:12px 16px;}
  .tab-panel.active{display:block;}

  /* Legend */
  .legend{display:flex;gap:14px;padding:8px 0 12px;border-bottom:1px solid #22252d;font-size:12px;color:#7d8493;
    font-family:-apple-system,sans-serif;}
  .legend .k{display:inline-flex;align-items:center;gap:6px;} .legend .sw{width:8px;height:8px;border-radius:50%;}
  
  #log{font-size:12.5px;line-height:1.8;padding-top:8px;}
  .e{display:flex;gap:9px;animation:in .25s ease;}
  @keyframes in{from{opacity:0;transform:translateY(4px);}to{opacity:1;transform:none;}}
  .t{color:#5a616e;flex:none;} .who{flex:none;width:50px;font-weight:600;}
  .agent .who{color:#5ac8fa;} .human .who{color:#30d158;} .system .who{color:#8a93a3;} .app .who{color:#ffd60a;}
  .d{color:#dfe3ea;word-break:break-word;}
  
  /* State Styles */
  .state-title{font-size:13px;color:#e6e8ee;margin-bottom:12px;font-family:-apple-system,sans-serif;font-weight:600;}
  .state-row{display:flex;padding:8px 10px;border-bottom:1px solid #1a1d24;font-size:12.5px;align-items:center;
    transition:background .15s;}
  .state-row:hover{background:#13161c;}
  .state-key{color:#5ac8fa;font-weight:bold;margin-right:8px;}
  .state-val{color:#ffd60a;cursor:pointer;text-decoration:underline;text-underline-offset:3px;}
  .state-val:hover{color:#fff;}

  /* Tree Styles */
  .tree-title{font-size:13px;color:#e6e8ee;margin-bottom:12px;font-family:-apple-system,sans-serif;font-weight:600;}
  .tree-node{margin-left:14px;border-left:1px dashed #2c313b;padding-left:10px;}
  .node-header{display:flex;align-items:center;gap:8px;padding:5px 6px;border-radius:4px;cursor:pointer;
    transition:all .15s;font-size:12.5px;user-select:none;}
  .node-header:hover{background:#1c1f26;color:#fff;}
  .node-type{color:#ffd60a;font-weight:bold;}
  .node-id{color:#30d158;font-size:11.5px;}
  .node-text{color:#7d8493;font-style:italic;}
  #panel-tree.active { display: flex; flex-direction: column; }

  /* Ctrl panel (Sticky at bottom) */
  .ctrl{display:flex;align-items:center;gap:6px;flex-wrap:wrap;padding:10px 16px;border-top:1px solid #22252d;
    background:#13161c;font-family:-apple-system,sans-serif;}
  .ctrl .cl{color:#7d8493;font-size:12px;margin-right:4px;}
  .ctrl button{background:#1a1d24;color:#c7ccd6;border:1px solid #2c313b;border-radius:7px;
    padding:5px 10px;font-size:12px;cursor:pointer;}
  .ctrl button:hover{background:#242832;color:#fff;}
  .pres{padding:8px 16px;border-top:1px solid #22252d;font-size:12px;color:#9aa2b1;font-family:-apple-system,sans-serif;
    background:#0c0e13;}
  .pres .lbl{color:#7d8493;} .pres b{color:#30d158;font-weight:600;} .pres .pw{color:#ffd60a;}
</style></head><body>
  <header><span class="dot"></span><b>QORM DevTool</b><small data-i18n="active">active session</small><select id="langsel" onchange="setLang(this.value)" style="margin-left:12px;background:#1a1d24;color:#c7ccd6;border:1px solid #2c313b;border-radius:7px;padding:2px 6px;font-size:11.5px;cursor:pointer;"></select></header>
  <div class="tabs-bar">
    <button class="tab-btn active" id="btn-logs" onclick="showTab('logs')" data-i18n="tabLogs">Activity Logs</button>
    <button class="tab-btn" id="btn-state" onclick="showTab('state')" data-i18n="tabState">State Manager</button>
    <button class="tab-btn" id="btn-tree" onclick="showTab('tree')" data-i18n="tabTree">Component Tree</button>
  </div>
  
  <div class="panel-body">
    <!-- Logs Panel -->
    <div class="tab-panel active" id="panel-logs">
      <div class="legend">
        <span class="k"><span class="sw" style="background:#30d158"></span><span data-i18n="legendYou">you</span></span>
        <span class="k"><span class="sw" style="background:#5ac8fa"></span><span data-i18n="legendAgent">AI agent</span></span>
        <span class="k"><span class="sw" style="background:#8a93a3"></span><span data-i18n="legendSystem">system</span></span>
      </div>
      <div id="log"><div class="e system"><span class="t">--:--:--</span><span class="who">system</span><span class="d" data-i18n="waiting">waiting for activity…</span></div></div>
    </div>
    
    <!-- State Panel -->
    <div class="tab-panel" id="panel-state">
      <div class="state-title" data-i18n="stateTitle">Live App State</div>
      <div id="state-list">Loading...</div>
    </div>
    
    <!-- Tree Panel -->
    <div class="tab-panel" id="panel-tree">
      <div class="tree-title" data-i18n="treeTitle">Rendered Component Architecture</div>
      <div id="tree-root" style="flex:1;overflow-y:auto;margin-bottom:12px;">Loading...</div>
      <div id="node-properties" style="height:180px;border-top:1px solid #22252d;padding-top:10px;font-size:12px;display:flex;flex-direction:column;overflow:hidden;">
        <div style="color:#7d8493;font-style:italic;" data-i18n="treeHint">Click a component node to inspect its properties.</div>
      </div>
    </div>
  </div>

  <div class="ctrl">
    <span class="cl" data-i18n="winLabel">window</span>
    <button onclick="qw(40,40,400,820)" data-i18n="winLeft"> left</button>
    <button onclick="qw((screen.width-400)/2,(screen.height-820)/2,400,820)" data-i18n="winCenter"> center</button>
    <button onclick="qw(screen.width-440,40,400,820)" data-i18n="winRight">right </button>
    <button onclick="qw(0,0,screen.availWidth/2,screen.availHeight)" data-i18n="winHalfL"> half-L</button>
    <button onclick="qw(screen.availWidth/2,0,screen.availWidth/2,screen.availHeight)" data-i18n="winHalfR">half-R </button>
    <button onclick="qw(0,0,screen.availWidth,screen.availHeight)" data-i18n="winMax"> max</button>
    <button onclick="qw(40,40,900,680)" data-i18n="winWide"> wide</button>
    <button onclick="qw(40,40,400,820)" data-i18n="winPhone"> phone</button>
    <button onclick="qo(&quot;focus&quot;)">⤒ <span data-i18n="winFocus">focus</span></button>
    <button onclick="qo(&quot;minimize&quot;)">— <span data-i18n="winMin">min</span></button>
    <button onclick="qo(&quot;pin&quot;)"> <span data-i18n="winPin">pin</span></button>
    <button onclick="qo(&quot;unpin&quot;)"><span data-i18n="winUnpin">unpin</span></button>
    <span class="cl" style="margin-left:8px" data-i18n="multiLabel">multi</span>
    <button onclick="qopen(&quot;win&quot;+(++qn),location.origin+&quot;/&quot;,400,600)">＋ <span data-i18n="winNew">window</span></button>
    <button onclick="qo(&quot;tile&quot;)"> <span data-i18n="winTile">tile all</span></button>
  </div>
  <div class="pres"><span class="lbl" data-i18n="pres">shared with the AI:</span> <span id="qpres" data-i18n="presNone">nothing yet</span></div>

<script>
  // DevTool i18n: 7 languages, shared preference key with the website (qorm-lang).
  var LANGNAMES={en:'English',zh:'中文',ja:'日本語',ko:'한국어',es:'Español',fr:'Français',de:'Deutsch'};
  var I18N={
    en:{tabLogs:'Activity Logs',tabState:'State Manager',tabTree:'Component Tree',legendYou:'you',legendAgent:'AI agent',legendSystem:'system',waiting:'waiting for activity…',stateTitle:'Live App State',loading:'Loading...',noState:'No state variables defined yet.',stateErr:'Error loading state: ',treeTitle:'Rendered Component Architecture',treeHint:'Click a component node to inspect its properties.',treeErr:'Error loading node tree: ',editPromptPre:'Edit value for "',editPromptSuf:'" (JSON formatted):',badJson:'Invalid JSON representation: ',pres:'shared with the AI:',presNone:'nothing yet',presOn:'on',presTyped:'typed',presFilled:'filled (value hidden)',active:'active session',winLabel:'window',multiLabel:'multi',winLeft:' left',winCenter:' center',winRight:'right ',winHalfL:' half-L',winHalfR:'half-R ',winMax:' max',winWide:' wide',winPhone:' phone',winFocus:'focus',winMin:'min',winPin:'pin',winUnpin:'unpin',winNew:'window',winTile:'tile all'},
    zh:{tabLogs:'活动日志',tabState:'状态管理',tabTree:'组件树',legendYou:'你',legendAgent:'AI 智能体',legendSystem:'系统',waiting:'等待活动…',stateTitle:'实时应用状态',loading:'加载中…',noState:'还没有定义状态变量。',stateErr:'加载状态出错:',treeTitle:'渲染组件结构',treeHint:'点击组件节点查看属性。',treeErr:'加载组件树出错:',editPromptPre:'编辑 "',editPromptSuf:'" 的值(JSON 格式):',badJson:'JSON 格式无效:',pres:'与 AI 共享:',presNone:'暂无',presOn:'聚焦',presTyped:'输入了',presFilled:'已填写(值已隐藏)',active:'会话进行中',winLabel:'窗口',multiLabel:'多窗',winLeft:' 靠左',winCenter:' 居中',winRight:'靠右 ',winHalfL:' 左半屏',winHalfR:'右半屏 ',winMax:' 最大化',winWide:' 宽屏',winPhone:' 手机',winFocus:'聚焦',winMin:'最小化',winPin:'置顶',winUnpin:'取消置顶',winNew:'窗口',winTile:'全部平铺'},
    ja:{tabLogs:'アクティビティログ',tabState:'状態管理',tabTree:'コンポーネントツリー',legendYou:'あなた',legendAgent:'AI エージェント',legendSystem:'システム',waiting:'アクティビティを待っています…',stateTitle:'ライブアプリ状態',loading:'読み込み中…',noState:'状態変数はまだ定義されていません。',stateErr:'状態の読み込みエラー:',treeTitle:'レンダリング済みコンポーネント構造',treeHint:'ノードをクリックするとプロパティを表示します。',treeErr:'ツリーの読み込みエラー:',editPromptPre:'「',editPromptSuf:'」の値を編集(JSON 形式):',badJson:'JSON が無効です:',pres:'AI と共有:',presNone:'まだなし',presOn:'フォーカス',presTyped:'入力',presFilled:'入力済み(値は非表示)',active:'セッション中',winLabel:'ウィンドウ',multiLabel:'マルチ',winLeft:' 左寄せ',winCenter:' 中央',winRight:'右寄せ ',winHalfL:' 左半分',winHalfR:'右半分 ',winMax:' 最大化',winWide:' ワイド',winPhone:' フォン',winFocus:'フォーカス',winMin:'最小化',winPin:'ピン留め',winUnpin:'ピン解除',winNew:'ウィンドウ',winTile:'すべて並べる'},
    ko:{tabLogs:'활동 로그',tabState:'상태 관리',tabTree:'컴포넌트 트리',legendYou:'나',legendAgent:'AI 에이전트',legendSystem:'시스템',waiting:'활동 대기 중…',stateTitle:'실시간 앱 상태',loading:'로딩 중…',noState:'정의된 상태 변수가 없습니다.',stateErr:'상태 로드 오류:',treeTitle:'렌더링된 컴포넌트 구조',treeHint:'컴포넌트 노드를 클릭하면 속성을 볼 수 있습니다.',treeErr:'트리 로드 오류:',editPromptPre:'"',editPromptSuf:'" 값 편집(JSON 형식):',badJson:'잘못된 JSON:',pres:'AI와 공유:',presNone:'아직 없음',presOn:'포커스',presTyped:'입력',presFilled:'입력됨(값 숨김)',active:'세션 진행 중',winLabel:'창',multiLabel:'멀티',winLeft:' 왼쪽',winCenter:' 가울데',winRight:'오른쪽 ',winHalfL:' 왼쪽 절반',winHalfR:'오른쪽 절반 ',winMax:' 최대화',winWide:' 와이드',winPhone:' 폰',winFocus:'포커스',winMin:'최소화',winPin:'고정',winUnpin:'고정 해제',winNew:'창',winTile:'모두 타일'},
    es:{tabLogs:'Registro de actividad',tabState:'Gestor de estado',tabTree:'Árbol de componentes',legendYou:'tú',legendAgent:'agente IA',legendSystem:'sistema',waiting:'esperando actividad…',stateTitle:'Estado en vivo de la app',loading:'Cargando…',noState:'Aún no hay variables de estado definidas.',stateErr:'Error al cargar el estado: ',treeTitle:'Arquitectura de componentes renderizados',treeHint:'Haz clic en un nodo para inspeccionar sus propiedades.',treeErr:'Error al cargar el árbol: ',editPromptPre:'Editar el valor de "',editPromptSuf:'" (formato JSON):',badJson:'JSON no válido: ',pres:'compartido con la IA:',presNone:'nada aún',presOn:'en',presTyped:'escribió',presFilled:'completado (valor oculto)',active:'sesión activa',winLabel:'ventana',multiLabel:'multi',winLeft:' izquierda',winCenter:' centro',winRight:'derecha ',winHalfL:' mitad izq.',winHalfR:'mitad der. ',winMax:' maximizar',winWide:' ancha',winPhone:' teléfono',winFocus:'enfocar',winMin:'minimizar',winPin:'fijar',winUnpin:'soltar',winNew:'ventana',winTile:'organizar'},
    fr:{tabLogs:"Journal d'activité",tabState:"Gestionnaire d'état",tabTree:'Arbre des composants',legendYou:'vous',legendAgent:'agent IA',legendSystem:'système',waiting:"en attente d'activité…",stateTitle:"État de l'app en direct",loading:'Chargement…',noState:"Aucune variable d'état définie pour le moment.",stateErr:"Erreur de chargement de l'état : ",treeTitle:'Architecture des composants rendus',treeHint:'Cliquez sur un nœud pour inspecter ses propriétés.',treeErr:"Erreur de chargement de l'arbre : ",editPromptPre:'Modifier la valeur de « ',editPromptSuf:' » (format JSON) :',badJson:'JSON invalide : ',pres:"partagé avec l'IA :",presNone:"rien pour l'instant",presOn:'sur',presTyped:'a saisi',presFilled:'rempli (valeur masquée)',active:'session active',winLabel:'fenêtre',multiLabel:'multi',winLeft:' gauche',winCenter:' centre',winRight:'droite ',winHalfL:' moitié G',winHalfR:'moitié D ',winMax:' agrandir',winWide:' large',winPhone:' téléphone',winFocus:'focus',winMin:'réduire',winPin:'épingler',winUnpin:'désépingler',winNew:'fenêtre',winTile:'mosaïque'},
    de:{tabLogs:'Aktivitätsprotokoll',tabState:'Statusverwaltung',tabTree:'Komponentenbaum',legendYou:'du',legendAgent:'KI-Agent',legendSystem:'System',waiting:'warte auf Aktivität…',stateTitle:'Live-App-Status',loading:'Laden…',noState:'Noch keine Statusvariablen definiert.',stateErr:'Fehler beim Laden des Status: ',treeTitle:'Gerenderte Komponentenstruktur',treeHint:'Klicke einen Knoten an, um seine Eigenschaften zu sehen.',treeErr:'Fehler beim Laden des Baums: ',editPromptPre:'Wert für „',editPromptSuf:'“ bearbeiten (JSON-Format):',badJson:'Ungültiges JSON: ',pres:'mit der KI geteilt:',presNone:'noch nichts',presOn:'auf',presTyped:'tippte',presFilled:'ausgefüllt (Wert verborgen)',active:'aktive Sitzung',winLabel:'Fenster',multiLabel:'Multi',winLeft:' links',winCenter:' Mitte',winRight:'rechts ',winHalfL:' linke Hälfte',winHalfR:'rechte Hälfte ',winMax:' maximieren',winWide:' breit',winPhone:' Telefon',winFocus:'fokussieren',winMin:'minimieren',winPin:'anheften',winUnpin:'lösen',winNew:'Fenster',winTile:'alle kacheln'}
  };
  function curLang(){ try{ var l=localStorage.getItem('qorm-lang'); if(l&&I18N[l]) return l; }catch(e){}
    var n=(navigator.language||'en').toLowerCase().split('-')[0]; return I18N[n]?n:'en'; }
  var LANG=curLang();
  function T(k){ return (I18N[LANG]&&I18N[LANG][k])||I18N.en[k]||k; }
  function applyLang(){ document.documentElement.lang=LANG;
    document.querySelectorAll('[data-i18n]').forEach(function(el){ el.textContent=T(el.getAttribute('data-i18n')); });
    var s=document.getElementById('langsel');
    if(s && !s.options.length){ Object.keys(LANGNAMES).forEach(function(k){ var o=document.createElement('option'); o.value=k; o.textContent=LANGNAMES[k]; s.appendChild(o); }); }
    if(s) s.value=LANG; }
  function setLang(l){ if(!I18N[l]) return; LANG=l; try{ localStorage.setItem('qorm-lang',l); }catch(e){} applyLang(); }
  var log=document.getElementById('log'),since=0,first=true,activeTab='logs';
  var initialLogs = {{initial_logs}};
  if (initialLogs && initialLogs.length) {
    log.innerHTML = '';
    first = false;
    initialLogs.forEach(function(e) {
      since = Math.max(since, e.seq);
      var d = document.createElement('div');
      d.className = 'e ' + e.source;
      d.innerHTML = '<span class="t"></span><span class="who"></span><span class="d"></span>';
      d.querySelector('.t').textContent = e.time;
      d.querySelector('.who').textContent = (e.source === 'human' ? T('legendYou') : e.source);
      d.querySelector('.d').textContent = e.detail;
      log.appendChild(d);
    });
    log.scrollTop = log.scrollHeight;
  }
  
  function showTab(tabId) {
    activeTab = tabId;
    document.querySelectorAll('.tab-btn').forEach(function(b){ b.classList.remove('active'); });
    document.querySelectorAll('.tab-panel').forEach(function(p){ p.classList.remove('active'); });
    document.getElementById('btn-' + tabId).classList.add('active');
    document.getElementById('panel-' + tabId).classList.add('active');
    
    if(tabId === 'state') loadState();
    if(tabId === 'tree') loadTree();
  }

  function poll(){
    if (activeTab !== 'logs') return;
    fetch('/log?since='+since).then(function(r){return r.json();}).then(function(rows){
      if(rows&&rows.length){if(first){log.innerHTML='';first=false;}
        rows.forEach(function(e){since=Math.max(since,e.seq);
          var d=document.createElement('div');d.className='e '+e.source;
          d.innerHTML='<span class="t"></span><span class="who"></span><span class="d"></span>';
          d.querySelector('.t').textContent=e.time;d.querySelector('.who').textContent=(e.source==='human'?T('legendYou'):e.source);
          d.querySelector('.d').textContent=e.detail;log.appendChild(d);});
        log.scrollTop=log.scrollHeight;}
    }).catch(function(){});
  }
  
  function esc(s){ return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/"/g,'&quot;'); }
  
  // State Manager Functions
  function loadState() {
    var container = document.getElementById('state-list');
    fetch('/dev/state').then(r => r.json()).then(data => {
      container.innerHTML = '';
      if (!Object.keys(data).length) {
        container.innerHTML = '<div style="color:#7d8493;font-size:12.5px;">' + T('noState') + '</div>';
        return;
      }
      for (var k in data) {
        var row = document.createElement('div');
        row.className = 'state-row';
        var valStr = JSON.stringify(data[k]);
        row.innerHTML = '<span class="state-key">' + esc(k) + ':</span>' +
          '<span class="state-val" onclick="editState(\''+esc(k)+'\','+esc(valStr)+')">' + esc(valStr) + '</span>';
        container.appendChild(row);
      }
    }).catch(err => { container.innerHTML = T('stateErr') + ' ' + err; });
  }

  function editState(key, oldVal) {
    var newVal = prompt(T('editPromptPre') + key + T('editPromptSuf'), JSON.stringify(oldVal));
    if (newVal !== null) {
      try {
        var parsed = JSON.parse(newVal);
        fetch('/dev/state').then(r => r.json()).then(current => {
          current[key] = parsed;
          fetch('/dev/state', {
            method: 'POST',
            headers: {'Content-Type': 'application/json', 'X-Qorm-Token': '{{token}}'},
            body: JSON.stringify(current)
          }).then(function(){ loadState(); });
        });
      } catch(e) {
        alert(T('badJson') + ' ' + e.message);
      }
    }
  }

  // Tree Inspector Functions
  function loadTree() {
    var root = document.getElementById('tree-root');
    fetch('/dev/tree').then(r => r.json()).then(data => {
      root.innerHTML = '';
      root.appendChild(renderNode(data));
    }).catch(err => { root.innerHTML = T('treeErr') + ' ' + err; });
  }

  function renderNode(node) {
    if (!node) return document.createTextNode('');
    var div = document.createElement('div');
    div.className = 'tree-node';
    var header = document.createElement('div');
    header.className = 'node-header';
    
    var typeText = esc(node.Type || 'View');
    var idText = node.ID ? ' <span class="node-id">#' + esc(node.ID) + '</span>' : '';
    var contentText = node.Text ? ' <span class="node-text">"' + esc(node.Text) + '"</span>' : '';
    
    header.innerHTML = '<span class="node-type">' + typeText + '</span>' + idText + contentText;
    
    if (node.ID) {
      header.onmouseenter = function() { highlight(node.ID); };
      header.onmouseleave = function() { highlight(''); };
    }
    header.onclick = function() {
      document.querySelectorAll('.node-header').forEach(function(h){ h.style.background = ''; });
      header.style.background = '#2c313b';
      showNodeProps(node);
    };
    div.appendChild(header);
    
    if (node.Children && node.Children.length) {
      var childrenDiv = document.createElement('div');
      childrenDiv.className = 'node-children';
      node.Children.forEach(function(child) {
        childrenDiv.appendChild(renderNode(child));
      });
      div.appendChild(childrenDiv);
    }
    return div;
  }

  function highlight(id) {
    fetch('/dev/highlight', {
      method: 'POST',
      headers: {'Content-Type': 'application/json', 'X-Qorm-Token': '{{token}}'},
      body: JSON.stringify({id: id})
    }).catch(function(){});
  }

  function showNodeProps(node) {
    var container = document.getElementById('node-properties');
    container.innerHTML = '<div style="color:#e6e8ee;font-weight:600;margin-bottom:8px;border-bottom:1px solid #22252d;padding-bottom:4px;font-family:-apple-system,sans-serif;">Node Properties</div>' +
      '<div style="flex:1;overflow-y:auto;padding-right:4px;" id="props-list"></div>';
    var list = document.getElementById('props-list');
    
    function addProp(k, v) {
      if (v === undefined || v === null || v === "") return;
      var row = document.createElement('div');
      row.style.display = 'flex';
      row.style.padding = '4px 0';
      row.style.borderBottom = '1px solid #1a1d24';
      row.style.fontFamily = 'ui-monospace,monospace';
      var valStr = typeof v === 'object' ? JSON.stringify(v) : String(v);
      row.innerHTML = '<span style="color:#5ac8fa;width:100px;flex-shrink:0;font-weight:bold;user-select:none;">' + esc(k) + '</span>' +
        '<span style="color:#ffd60a;word-break:break-all;">' + esc(valStr) + '</span>';
      list.appendChild(row);
    }
    
    addProp('Type', node.Type);
    addProp('ID', node.ID);
    addProp('Text', node.Text);
    addProp('Label', node.Label);
    addProp('Placeholder', node.Placeholder);
    addProp('Value', node.Value);
    if(node.Style && Object.keys(node.Style).length) addProp('Style', node.Style);
    if(node.Layout && Object.keys(node.Layout).length) addProp('Layout', node.Layout);
    if(node.Props && Object.keys(node.Props).length) addProp('Props', node.Props);
  }

  function qw(x,y,w,h){fetch('/window',{method:'POST',body:JSON.stringify({op:'move',x:Math.round(x),y:Math.round(y),w:w,h:h})}).catch(function(){});}
  var qn=1;
  function qo(op){fetch('/window',{method:'POST',body:JSON.stringify({op:op})}).catch(function(){});}
  function qopen(id,url,w,h){fetch('/window',{method:'POST',body:JSON.stringify({op:'open',id:id,url:url,w:w,h:h})}).catch(function(){});}
  
  setInterval(poll,600);poll();
  
  function qpres(){
    fetch('/presence').then(function(r){return r.json();}).then(function(p){
      var el=document.getElementById('qpres'); if(!el) return; var parts=[];
      if(p.focus) parts.push(T('presOn')+' <b>'+esc(p.focus)+'</b>');
      if(p.typing) parts.push(T('presTyped')+' <b>'+esc(p.typing)+'</b>');
      if(p.filled) parts.push('<span class="pw">'+esc(p.filled)+' '+T('presFilled')+'</span>');
      el.innerHTML = parts.length ? parts.join(' &middot; ') : T('presNone');
    }).catch(function(){});
  }
  setInterval(qpres,900); qpres();
  applyLang();
  if (window.location.search.indexOf('demo=1') !== -1) {
    setTimeout(function(){
      showTab('tree');
      setTimeout(function(){
        var h = document.querySelector('.node-header');
        if(h) h.click();
      }, 400);
    }, 1000);
  }
</script></body></html>`

// serveDevState handles GET (read state) and POST (update state) for DevTools.
func (s *Server) serveDevState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Method == "POST" {
		// DevTool writes are human-side: they need the page-embedded token
		// (like /event), and they are attributed as "devtool" — never lumped
		// into "system", so the audit trail shows who actually mutated state.
		if r.Header.Get("X-Qorm-Token") != s.eventToken {
			http.Error(w, "invalid event token", http.StatusForbidden)
			return
		}
		var newState map[string]any
		if err := json.NewDecoder(r.Body).Decode(&newState); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.rt.State = newState
		s.logEvent("devtool", "state modified via devtool")
		s.bump()
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.rt.State)
}

// serveDevTree returns the current scene's node tree structure.
func (s *Server) serveDevTree(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sceneID := s.rt.CurrentScene()
	if sceneID == "" {
		sceneID = s.rt.App.Entry
	}
	rootNode, ok := s.rt.App.Scenes[sceneID]
	if !ok {
		// Fallback to any scene
		for _, n := range s.rt.App.Scenes {
			rootNode = n
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rootNode)
}

// serveDevHighlight broadcasts a node inspect highlight payload over SSE.
func (s *Server) serveDevHighlight(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Qorm-Token") != s.eventToken {
		http.Error(w, "invalid event token", http.StatusForbidden)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Broadcast inspectNode over SSE subscribers
	m := map[string]any{
		"rev":         s.rev.Load(),
		"inspectNode": req.ID,
	}
	payload, _ := json.Marshal(m)
	msg := string(payload)

	s.subsMu.Lock()
	for ch := range s.subs {
		select {
		case ch <- msg:
		default:
		}
	}
	s.subsMu.Unlock()

	w.WriteHeader(http.StatusOK)
}
