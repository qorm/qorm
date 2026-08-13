// Gather all state-bound controls into a typed map, dispatch handler h, and
// swap in the re-rendered UI. h === -1 means "just sync state" (no action).
// morphChildren diffs the new HTML into the live DOM in place, so unchanged
// nodes are never re-created — no flicker, entrance animations don't replay on
// every click, and input focus/scroll survive.
// qormParseRGB parses a computed "rgb(r,g,b)"/"rgba(r,g,b,a)" string to [r,g,b,a].
function qormParseRGB(s){
  var m=/rgba?\(([^)]+)\)/.exec(s||''); if(!m) return null;
  var p=m[1].split(',').map(function(x){return parseFloat(x);});
  return [p[0]||0, p[1]||0, p[2]||0, p.length>3?p[3]:1];
}
// qormLum returns the WCAG relative luminance of an [r,g,b] triple.
function qormLum(c){
  var f=c.map(function(v){ v=v/255; return v<=0.03928 ? v/12.92 : Math.pow((v+0.055)/1.055,2.4); });
  return 0.2126*f[0]+0.7152*f[1]+0.0722*f[2];
}
// qormContrast returns the WCAG contrast ratio between el's text colour and its
// EFFECTIVE background — walking up ancestors and compositing translucent layers
// (element backgrounds are usually transparent, so the self colour vs bg is wrong
// without this). Returns 0 when it can't be determined.
function qormContrast(el, cs){
  try{
    var fg=qormParseRGB(cs.color); if(!fg) return 0;
    var bg=[255,255,255], node=el, found=false;
    while(node && node.nodeType===1){
      var b=qormParseRGB(getComputedStyle(node).backgroundColor);
      if(b && b[3]>0){
        var a=b[3];
        bg=[Math.round(b[0]*a+bg[0]*(1-a)), Math.round(b[1]*a+bg[1]*(1-a)), Math.round(b[2]*a+bg[2]*(1-a))];
        if(a>=0.999){ found=true; break; }
      }
      node=node.parentElement;
    }
    var L1=qormLum([fg[0],fg[1],fg[2]]), L2=qormLum(bg);
    var hi=Math.max(L1,L2), lo=Math.min(L1,L2);
    return Math.round(((hi+0.05)/(lo+0.05))*100)/100;
  }catch(e){ return 0; }
}
// Self-measurement: report each id'd element's rect + key styles to /measure,
// so the framework can verify its own layout/styles without an external browser.
function qormMeasure(){
  try{
    var out=[];
    document.querySelectorAll('[id]').forEach(function(el){
      // qorm-root (the app container) IS measured: the audit bounds every
      // element against its actual box, whatever the scene's root id is.
      // qorm-stage (the device frame) is not a component — skip it.
      if(el.id==='qorm-stage') return;
      var r=el.getBoundingClientRect(), cs=getComputedStyle(el);
      var vis = r.width>0 && r.height>0 && cs.display!=='none' && cs.visibility!=='hidden' && parseFloat(cs.opacity)>0.01;
      out.push({id:el.id, tag:el.tagName.toLowerCase(),
        x:Math.round(r.left), y:Math.round(r.top), w:Math.round(r.width), h:Math.round(r.height),
        visible:vis, text:(el.childElementCount===0?(el.textContent||'').trim().slice(0,60):''),
        display:cs.display, color:cs.color, background:cs.backgroundColor,
        fontSize:cs.fontSize, fontWeight:cs.fontWeight, textAlign:cs.textAlign,
        padding:cs.padding, margin:cs.margin, borderRadius:cs.borderRadius,
        border:(cs.borderTopWidth!=='0px'?cs.borderTopWidth+' '+cs.borderTopStyle+' '+cs.borderTopColor:'none'),
        opacity:cs.opacity, zIndex:cs.zIndex, position:cs.position,
        overflowX:el.scrollWidth>el.clientWidth+1,
        role:el.getAttribute('role')||'', ariaLabel:el.getAttribute('aria-label')||'',
        tabindex:el.getAttribute('tabindex')||'', contrast:qormContrast(el,cs)});
    });
    // The page token rides along so self-measurement survives the --lan
    // token gate (/measure is one of the five LAN-facing endpoints).
    fetch('/measure',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},body:JSON.stringify(out)});
  }catch(e){}
}
// qormFlash briefly outlines a node the AI just changed, so the human sees WHERE
// an edit landed (spatial attribution), not only the "AI edited" toast. No-op for
// the human's own edits and initial paint.
function qormFlash(el){
  if(!el || el.nodeType!==1 || window.__qormEditSrc!=='agent') return;
  el.classList.add('qorm-ai-touch');
  setTimeout(function(){ el.classList.remove('qorm-ai-touch'); }, 1300);
}
function qormMorphInto(root, html){
  var tmp=document.createElement('div'); tmp.innerHTML=html;
  var active=document.activeElement, activeId=active&&active.id;
  morphKids(root, tmp);
  if(activeId){ var el=document.getElementById(activeId); if(el&&el.focus) try{ el.focus(); }catch(e){} }
  qormTimersSync();
  qormSheetSync();       // re-apply the live sheet stop the morph just reset
  qormLargeTitleSync();  // no-op where the CSS scroll timeline does the work
  qormTabReveal();       // keep the active tab of a scrollable tab bar in view
  qormCarouselSync();    // re-arm autoplay, re-derive the indicator dots
  qormDebounceSync();    // drop pending input timers whose field left the DOM
  qormValiditySync();    // re-apply custom validity messages the morph reset
  qormVListSync();       // does the newly rendered window still cover the view?
  setTimeout(qormMeasure, 30);
}
// qormApplyFrame is the HOST FRAME SINK for the client-side runtimes: the WASM
// build (cmd/qorm-wasm) calls window.qormApplyFrame(res) whenever it has a
// frame to publish that no JS call is waiting on — an intermediate frame from a
// `render` step, or the completion of an http.* step whose round trip ran on a
// background goroutine. The server build never uses it (there the same frames
// arrive over SSE); it lives in the shared script so the offline package, which
// reuses this file verbatim, has it without a second copy in the boot string.
//
// res is renderNow()'s object: { html, theme, dir, locale }. Anything else —
// an error result, a frame that arrived after the root went away — is ignored
// rather than thrown, because this is called FROM Go and a throw would cross
// back into the wasm scheduler.
function qormApplyFrame(res){
  if(!res || typeof res.html!=='string') return;
  if(res.theme) qormTheme(res.theme);
  if(typeof qormDir==='function') qormDir(res.dir);   // defined by the offline boot
  var root=document.getElementById('qorm-root');
  if(root) qormMorphInto(root, res.html);
}
// ---- declarative timers ------------------------------------------------------
// A `timer` node renders as an invisible [data-qorm-timer] marker; this
// scheduler reconciles the live DOM against a registry after the initial paint
// and after every morph, so re-renders are idempotent: the same id is never
// scheduled twice, a marker that disappears (its `if` turned falsy, the scene
// changed) stops its schedule, and a changed interval reschedules. A tick
// dispatches qorm(h) — the exact same event chain as a button press — and
// re-reads data-h from the live DOM at fire time, so a handler table
// renumbered by a re-render is never dispatched stale. Repeating (`every`)
// ticks are skipped while the tab is hidden; an `after` one-shot fires once
// per appearance of its marker (it re-arms only after the marker leaves and
// comes back).
window.__qormTimers = window.__qormTimers || {};
function qormTimerEl(id){
  var found=null;
  document.querySelectorAll('[data-qorm-timer]').forEach(function(el){
    if(el.getAttribute('data-qorm-timer')===id) found=el;
  });
  return found; // last match: during a page transition the incoming scene's marker wins
}
function qormTimerStop(id){
  var t=window.__qormTimers[id]; if(!t) return;
  if(t.once){ clearTimeout(t.h); } else { clearInterval(t.h); }
  delete window.__qormTimers[id];
}
function qormTimerFire(id){
  var el=qormTimerEl(id);
  if(!el){ qormTimerStop(id); return; }
  var t=window.__qormTimers[id];
  if(t&&t.once){ if(t.fired) return; t.fired=true; }
  else if(document.hidden){ return; }
  var h=parseInt(el.getAttribute('data-h'),10);
  if(!isNaN(h)&&h>=0) qorm(h);
}
function qormTimersSync(){
  var seen={};
  document.querySelectorAll('[data-qorm-timer]').forEach(function(el){
    var id=el.getAttribute('data-qorm-timer'); if(!id) return;
    var every=parseInt(el.getAttribute('data-every'),10)||0;
    var after=parseInt(el.getAttribute('data-after'),10)||0;
    if(every>0&&every<16) every=16; // defense in depth with the renderer's clamp; 60fps floor
    seen[id]=1;
    var t=window.__qormTimers[id];
    if(t&&t.every===every&&t.after===after) return; // unchanged — never double-schedule
    if(t) qormTimerStop(id);
    if(every>0){ window.__qormTimers[id]={every:every,after:0,once:false,h:setInterval(function(){ qormTimerFire(id); },every)}; }
    else if(after>0){ window.__qormTimers[id]={every:0,after:after,once:true,fired:false,h:setTimeout(function(){ qormTimerFire(id); },after)}; }
  });
  Object.keys(window.__qormTimers).forEach(function(id){ if(!seen[id]) qormTimerStop(id); });
}
// qormPageTransition plays a coordinated push/pop: the incoming scene slides in
// from the edge while the outgoing one parallax-slides the other way (less far)
// and dims — the depth cue that makes an iOS navigation feel right. dir 'pop'
// reverses the direction. Both scenes are stacked absolutely during the run.
function qormPageTransition(container, oldEl, newEl, dir){
  var back=(dir==='pop');
  var inFrom=back?'-100%':'100%', outTo=back?'30%':'-30%';
  var pos=container.style.position, ovx=container.style.overflowX;
  container.style.position='relative'; container.style.overflowX='hidden';
  // Each sliding scene must be an OPAQUE block, or the two overlap and read as a
  // mess. Give a transparent scene the stage's background for the duration.
  var stageBg=getComputedStyle(document.getElementById('qorm-stage')).backgroundColor;
  [oldEl,newEl].forEach(function(e){ e.style.position='absolute'; e.style.top='0'; e.style.left='0'; e.style.right='0'; e.style.bottom='0'; e.style.margin='0'; e.style.willChange='transform,filter';
    var cbg=getComputedStyle(e).backgroundColor;
    if(!cbg||cbg==='rgba(0, 0, 0, 0)'||cbg==='transparent'){ e.style.background=stageBg; e.setAttribute('data-qorm-txbg','1'); }
  });
  container.appendChild(newEl);
  newEl.style.transform='translateX('+inFrom+')';
  void newEl.offsetWidth; // commit the start frame before transitioning
  var dur=560, ease='cubic-bezier(.32,.72,0,1)';
  newEl.style.transition='transform '+dur+'ms '+ease;
  oldEl.style.transition='transform '+dur+'ms '+ease+', filter '+dur+'ms '+ease;
  newEl.style.transform='translateX(0)';
  oldEl.style.transform='translateX('+outTo+')';
  oldEl.style.filter='brightness(.5)';
  setTimeout(function(){
    if(oldEl.parentNode===container) container.removeChild(oldEl);
    ['position','top','left','right','bottom','margin','transform','transition','filter','willChange'].forEach(function(p){ newEl.style.removeProperty(p); });
    if(newEl.getAttribute('data-qorm-txbg')){ newEl.style.removeProperty('background'); newEl.removeAttribute('data-qorm-txbg'); }
    container.style.position=pos; container.style.overflowX=ovx;
  }, dur+50);
}
function morphKids(from, to){
  var fc=from.firstChild, tc=to.firstChild;
  while(tc){
    var nt=tc.nextSibling;
    if(!fc){ var an=document.importNode(tc,true); from.appendChild(an); qormFlash(an); tc=nt; continue; }
    var nf=fc.nextSibling;
    if(fc.nodeType!==tc.nodeType || (fc.nodeType===1 && fc.nodeName!==tc.nodeName)){
      var rn=document.importNode(tc,true); from.replaceChild(rn, fc); qormFlash(rn);
    } else if(fc.nodeType===3 || fc.nodeType===8){
      if(fc.nodeValue!==tc.nodeValue){ fc.nodeValue=tc.nodeValue; qormFlash(from); }
    } else if(fc.nodeType===1 && fc.getAttribute('data-scene')!==null && fc.getAttribute('data-scene')!==tc.getAttribute('data-scene')){
      // navigation swapped the scene: play a coordinated iOS-style page transition
      qormPageTransition(from, fc, document.importNode(tc,true), window.__qormNav);
    } else if(fc.nodeType===1){
      morphEl(fc, tc);
    }
    fc=nf; tc=nt;
  }
  while(fc){ var n=fc.nextSibling; from.removeChild(fc); fc=n; }
}
window.__qormIsComposing=false;
if(typeof window!=='undefined'){
  window.addEventListener('compositionstart', function(){ window.__qormIsComposing=true; });
  window.addEventListener('compositionend', function(){ window.__qormIsComposing=false; });
}
function morphEl(from, to){
  // sync attributes
  var changed=false;
  var ta=to.attributes, i, a;
  for(i=ta.length-1;i>=0;i--){ a=ta[i]; if(from.getAttribute(a.name)!==a.value){ from.setAttribute(a.name,a.value); changed=true; } }
  var fa=from.attributes;
  for(i=fa.length-1;i>=0;i--){ a=fa[i]; if(!to.hasAttribute(a.name)){ from.removeAttribute(a.name); changed=true; } }
  if(changed) qormFlash(from);
  var focused=(document.activeElement===from);
  var isComposing=window.__qormIsComposing;
  // form controls: keep the user's live value/checked unless they're not focused/composing
  if(from.nodeName==='INPUT'){
    if(!focused && !isComposing){ if(to.hasAttribute('checked')!==from.checked) from.checked=to.hasAttribute('checked');
      if(to.getAttribute('value')!=null && from.value!==to.getAttribute('value')) from.value=to.getAttribute('value'); }
    return;
  }
  if(from.nodeName==='TEXTAREA'){ if(!focused && !isComposing) from.value=to.textContent; return; }
  morphKids(from, to);
}
function qorm(h){
  var inputs={};
  document.querySelectorAll('[data-state]').forEach(function(el){
    var k=el.getAttribute('data-state');
    if(el.type==='checkbox'){ inputs[k]=el.checked; }
    else if(el.type==='radio'){ if(el.checked) inputs[k]=el.value; }
    else if(el.type==='range'||el.type==='number'){ inputs[k]=parseFloat(el.value); }
    else { inputs[k]=el.value; }
  });
  // rev names the frame this button was rendered on: handler indices are
  // positional, so a frame that landed between the paint and the click (an
  // agent edit, or an intermediate frame from a `render` step) would otherwise
  // make h point at a different action server-side.
  fetch('/event',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},
    body:JSON.stringify({h:h,rev:__rev,inputs:inputs})})
    .then(function(r){ var rv=parseInt(r.headers.get('X-Qorm-Rev'))||0; var nav=r.headers.get('X-Qorm-Nav')||''; qormTheme(r.headers.get('X-Qorm-Theme')); return r.text().then(function(html){ return {rv:rv,html:html,nav:nav}; }); })
    .then(function(o){ if(o.rv && o.rv<=__rev) return; if(o.rv) __rev=o.rv; window.__qormNav=o.nav; qormMorphInto(document.getElementById('qorm-root'), o.html); });
}
// Camera: open the device camera/photo picker, read the chosen image as a data
// URL, show it in the preview, sync it into bound state, and fire onChange.
function qormCameraInit(){
  if(!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia && window.isSecureContext)) return;
  document.querySelectorAll('.qorm-camera').forEach(function(box){
    var live=box.querySelector('.qorm-cam-live'), file=box.querySelector('.qorm-cam-file');
    if(live) live.style.display='inline-block';
    if(file) file.style.display='none';
  });
}
// Set the live button's label without wiping the leading SVG icon: the Go
// renderer emits icon+text inside a wrapper span (iconLabel in render_style.go),
// so update the span's last text node rather than assigning textContent. A
// custom label prop renders as plain text (no span, no icon) — textContent is
// safe then.
function qormCamLabel(btn, text){
  var span=btn.querySelector('span');
  if(!span){ btn.textContent=text; return; }
  for(var c=span.lastChild;c;c=c.previousSibling){
    if(c.nodeType===3){ c.nodeValue=text; return; }
  }
  span.appendChild(document.createTextNode(text));
}
function qormCameraLive(btn){
  var box=btn.closest('.qorm-camera'); if(!box) return;
  var video=box.querySelector('.qorm-cam-video');
  if(box._stream){
    var c=document.createElement('canvas'); c.width=video.videoWidth||640; c.height=video.videoHeight||480;
    c.getContext('2d').drawImage(video,0,0,c.width,c.height);
    var data=c.toDataURL('image/jpeg',0.9);
    var img=box.querySelector('.qorm-cam-preview'); if(img){ img.src=data; img.style.display='block'; }
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=data; }
    box._stream.getTracks().forEach(function(t){ t.stop(); }); box._stream=null;
    video.style.display='none'; qormCamLabel(btn,'Retake');
    var h=box.getAttribute('data-h'); qorm(h?parseInt(h):-1);
    return;
  }
  navigator.mediaDevices.getUserMedia({video:{facingMode:'environment'}}).then(function(stream){
    box._stream=stream; video.srcObject=stream; video.style.display='block'; video.play(); qormCamLabel(btn,'Capture');
  }).catch(function(e){
    var wrap=box.querySelector('.qorm-cam-file'); var fi=wrap&&wrap.querySelector('input');
    if(wrap){ wrap.style.display='inline-block'; } if(fi){ fi.click(); }
  });
}
function qormCamera(input){
  var f=input.files&&input.files[0]; if(!f) return;
  var box=input.closest('.qorm-camera'); if(!box) return;
  var rd=new FileReader();
  rd.onload=function(){
    var img=box.querySelector('.qorm-cam-preview'); if(img){ img.src=rd.result; img.style.display='block'; }
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=rd.result; }
    var h=box.getAttribute('data-h');
    qorm(h?parseInt(h):-1);
  };
  rd.readAsDataURL(f);
}
// Native hardware bridge (present in the QORM Dev app): call native
// CoreLocation/CoreMotion/etc. — no HTTPS/secure-context needed. Falls back to
// the Web API in a plain browser.
function qormHasNative(){ return !!((window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.qorm) || window.qormAndroid || window.qormDesktop); }
// Mobile bridge only (iOS/Android) — the full hardware bridge. Desktop implements
// just a subset, so camera/mic/location must use the Web API there, not the bridge.
function qormHasMobileNative(){ return !!((window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.qorm) || window.qormAndroid); }
function qormToNative(op,data){
  // The app's OWN Go middle-layer (compiled into the WASM) handles its custom
  // ops first — so one Go file runs on mobile/web WebViews. It returns a line
  // of JS (may itself call qormToNative(...) to reach framework hardware, or a
  // Web API); "" means "not mine"  fall through to the built-in bridge.
  if(window.qormWasmOp){ var r=window.qormWasmOp(op, JSON.stringify(data||{})); if(r){ try{ (0,eval)(r); }catch(e){} return; } }
  var msg = Object.assign({op:op}, data||{});
  if(window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.qorm){ window.webkit.messageHandlers.qorm.postMessage(msg); }
  else if(window.qormAndroid && typeof window.qormAndroid[op]==='function'){ window.qormAndroid[op](JSON.stringify(data||{})); }
  else if(window.qormDesktop){ window.qormDesktop(JSON.stringify(msg)); }
}
function qormOnScreens(list){ var t = (list||[]).map(function(s,i){ return 'Display '+(i+1)+': '+s.w+'×'+s.h+' @'+s.scale+'x'+(s.main?' (main)':''); }).join('\n'); document.querySelectorAll('.qorm-screens-out').forEach(function(o){ o.textContent = t || 'no display info'; }); }
function qormLoginItem(btn){ var box=btn.closest('.qorm-loginitem'); var on=box.getAttribute('data-on')==='1'; if(qormHasNative()){ qormToNative('loginItem',{enabled:!on}); } else { box.querySelector('.qorm-loginitem-out').textContent='desktop only'; } }
function qormOnLoginItem(on,ok){ document.querySelectorAll('.qorm-loginitem').forEach(function(box){ box.setAttribute('data-on', on?'1':'0'); box.querySelector('.qorm-loginitem-out').textContent='Start at Login: '+(on?'ON':'OFF')+(ok?'':' (install the .app first)'); }); }
function qormOnNotifyClick(id){ var box=document.getElementById(id); if(box){ var o=box.querySelector('.qorm-notify-out'); if(o) o.textContent='Notification clicked '; } }
function qormBadge(btn,d){ var box=btn.closest('.qorm-dockbadge'); var n=Math.max(0,(parseInt(box.getAttribute('data-count'))||0)+d); box.setAttribute('data-count',n); box.querySelector('.qorm-dockbadge-out').textContent='Badge: '+n; if(qormHasNative()){ qormToNative('badge',{count:n}); } }
function qormNotify(btn){
  var box=btn.closest('.qorm-notify'), title=box.getAttribute('data-title')||'QORM', body=box.getAttribute('data-body')||'Hello from your QORM app';
  var out=box.querySelector('.qorm-notify-out');
  if(qormHasNative()){ qormToNative('notify',{title:title,body:body,id:box.id}); out.textContent='Sent '; }
  else if('Notification'in window){ Notification.requestPermission().then(function(p){ if(p==='granted'){ new Notification(title,{body:body}); out.textContent='Sent '; } else { out.textContent='permission denied'; } }); }
  else { out.textContent='not supported'; }
}
// Geolocation: read the device GPS and sync "lat, lng" into bound state.
function qormGeo(btn){
  var out=btn.closest('.qorm-location').querySelector('.qorm-loc-out');
  out.textContent='Locating…';
  if(qormHasMobileNative()){ qormToNative('location'); return; }
  if(!navigator.geolocation){ out.textContent='Geolocation not supported (needs the QORM Dev app or https)'; return; }
  navigator.geolocation.getCurrentPosition(function(p){ qormOnLocation(p.coords.latitude, p.coords.longitude, p.coords.accuracy); },
    function(e){ qormOnLocationError(e.message); }, {enableHighAccuracy:true, timeout:10000});
}
function qormOnLocation(lat,lng,acc){
  var s=lat.toFixed(5)+', '+lng.toFixed(5)+'  (±'+Math.round(acc)+'m)';
  document.querySelectorAll('.qorm-location').forEach(function(box){
    box.querySelector('.qorm-loc-out').textContent=s;
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=s; }
  });
  qorm(-1);
}
function qormOnLocationError(msg){ document.querySelectorAll('.qorm-location .qorm-loc-out').forEach(function(o){ o.textContent='Error: '+msg; }); }
// Motion: stream device orientation (accelerometer/gyro) live.
function qormMotion(btn){
  var out=btn.closest('.qorm-motion').querySelector('.qorm-motion-out');
  if(qormHasNative()){ qormToNative('motionStart'); btn.textContent='Motion On'; return; }
  function start(){
    window.addEventListener('deviceorientation', function(e){ qormOnMotion(e.alpha||0, e.beta||0, e.gamma||0); });
    btn.textContent='Motion On';
  }
  if(typeof DeviceOrientationEvent!=='undefined' && typeof DeviceOrientationEvent.requestPermission==='function'){
    DeviceOrientationEvent.requestPermission().then(function(r){ if(r==='granted'){ start(); } else { out.textContent='Permission denied'; } }).catch(function(e){ out.textContent='Error: '+e; });
  } else { start(); }
}
function qormBio(btn){
  var out=btn.closest('.qorm-biometric').querySelector('.qorm-bio-out');
  out.textContent='Authenticating…';
  if(qormHasNative()){ qormToNative('biometric'); return; }
  out.textContent='Biometrics need the QORM Dev app';
}
function qormOnBiometric(ok, msg){
  document.querySelectorAll('.qorm-biometric').forEach(function(box){
    box.querySelector('.qorm-bio-out').textContent=(ok?'Authenticated':'Not authenticated')+(msg?' — '+msg:'');
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=ok?'authenticated':'failed'; }
  });
  qormEmit('biometric', ok);
}
function qormBluetooth(btn){ var out=btn.closest('.qorm-bluetooth').querySelector('.qorm-bluetooth-out'); out.textContent='Scanning…'; if(qormHasNative()){ qormToNative('bluetoothScan'); } else { out.textContent='Bluetooth needs the QORM Dev app'; } }
function qormOnBluetoothState(on){ document.querySelectorAll('.qorm-bluetooth-out').forEach(function(o){ o.textContent='Bluetooth: '+(on?'ON':'OFF'); }); }
function qormOnBluetooth(json){ var list; try{ list=JSON.parse(json); }catch(e){ list=[]; }
  document.querySelectorAll('.qorm-bluetooth-out').forEach(function(o){ o.textContent = list.length ? list.map(function(d){ return (d.name||'(unknown)')+'  '+d.rssi+'dBm'; }).join('\n') : 'No devices found'; }); }
var QORM_CAPS = {
  'qorm-camera':'ios android mac linux windows web','qorm-location':'ios android mac linux windows web',
  'qorm-recorder':'ios android mac linux windows web','qorm-battery':'ios android mac linux web',
  'qorm-motion':'ios android','qorm-biometric':'ios android mac','qorm-bluetooth':'ios android mac',
  'qorm-wifi':'ios android mac','qorm-nfc':'ios android','qorm-vibrate':'ios android web','qorm-torch':'ios android',
  'qorm-volume':'ios android mac linux','qorm-brightness':'ios android mac','qorm-notify':'mac linux web',
  'qorm-dockbadge':'mac','qorm-loginitem':'mac','qorm-screens':'mac linux windows'
};
function qormOnPlatform(p){ window.qormPlatform=p; qormPlatformCheck(p); }
function qormPlatformCheck(platform){
  var missing=[];
  for(var cls in QORM_CAPS){
    if(document.querySelector('.'+cls) && QORM_CAPS[cls].split(' ').indexOf(platform)<0){ missing.push(cls.replace('qorm-','')); }
  }
  if(missing.length) qormPlatformBanner(platform, missing);
}
function qormPlatformBanner(platform, missing){
  if(document.getElementById('qorm-plat-banner')) return;
  var b=document.createElement('div'); b.id='qorm-plat-banner';
  b.style.cssText='position:fixed;top:0;left:0;right:0;z-index:99999;background:#b45309;color:#fff;font-size:13px;line-height:1.4;padding:8px 34px 8px 12px;box-shadow:0 1px 6px rgba(0,0,0,.25);';
  b.textContent='\u26a0 '+missing.length+'feature(s) not available on '+platform+': '+missing.join(', ');
  var x=document.createElement('button'); x.textContent='\u00d7'; x.setAttribute('aria-label','dismiss');
  x.style.cssText='position:absolute;right:6px;top:4px;background:none;border:none;color:#fff;font-size:20px;line-height:1;cursor:pointer;';
  x.onclick=function(){ b.remove(); }; b.appendChild(x); document.body.appendChild(b);
}
// --- native->UI event channel -------------------------------------------------
// The native/lower layer (OS listeners, the Go/WASM middle-layer, another window)
// EMITS a signal; the frontend just SUBSCRIBES. One channel for every push event
// so a widget never polls for something the system can tell it. Built-ins register
// as listeners too, so an app can also listen for the same signals.
window.__qormBus = window.__qormBus || {};
window.__qormQ = window.__qormQ || {};
function qormOn(evt, fn){
  (window.__qormBus[evt] = window.__qormBus[evt] || []).push(fn);
  var q = window.__qormQ[evt]; // deliver events emitted before this listener existed
  if(q && q.length){ window.__qormQ[evt] = []; q.forEach(function(d){ try{ fn(d); }catch(e){} }); }
  return fn;
}
function qormOff(evt, fn){ var a = window.__qormBus[evt]; if(a){ var i = a.indexOf(fn); if(i>=0) a.splice(i,1); } }
function qormEmit(evt, data){
  var a = window.__qormBus[evt];
  if(a && a.length){ a.slice().forEach(function(fn){ try{ fn(data); }catch(e){ if(window.console) console.error('qorm listener '+evt, e); } }); }
  else { var q = (window.__qormQ[evt] = window.__qormQ[evt] || []); q.push(data); if(q.length > 8) q.shift(); } // queue for a late listener
  // surface meaningful events in the Activity log (skip high-frequency sync)
  if(['volume','brightness','mute','tick','insets','hwsync'].indexOf(evt) < 0){
    var det = evt + (data && data.id ? ' ' + data.id : '');
    try{ fetch('/log', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({source:'app', detail:det})}); }catch(e){}
  }
}
function qormHwInit(){
  qormCameraInit();
  if(!window.__qormBusInit){ window.__qormBusInit=1;
    qormOn('volume', function(v){ qormOnVolume(v); });
    qormOn('mute', function(m){ qormOnMute(m); });
    qormOn('brightness', function(v){ qormOnBrightness(v); });
    qormOn('battery', function(d){ if(d&&typeof d==='object') qormOnBattery(d.level, d.charging); });
    qormOn('network', function(d){ qormOnNetwork(typeof d==='string'?d:JSON.stringify(d)); });
  }
  if(!qormHasNative()) return;
  if(document.querySelector('.qorm-volume')) qormToNative('volumeGet');
  if(document.querySelector('.qorm-brightness')) qormToNative('brightnessGet');
  if(document.querySelector('.qorm-battery')) qormToNative('battery');
  if(document.querySelector('.qorm-torch')) qormToNative('torchGet');
  // NOTE: do NOT auto-read bluetoothState on load — CBCentralManager init aborts a packaged .app via TCC on macOS. Bluetooth is click-to-scan.
  if(document.querySelector('.qorm-loginitem')) qormToNative('loginItemGet');
  if(document.querySelector('.qorm-screens')) qormToNative('screens');
  if(qormHasNative()){ qormToNative('platform'); qormToNative('pendingShortcut'); qormToNative('getInsets'); } else { qormPlatformCheck('web'); }
  // keep the externally-mutable readouts in sync (a volume key, a power cable,
  // a Wi-Fi drop) by re-reading them on an interval, not just once on load.
  if(!window.__qormHwSync){ window.__qormHwSync=setInterval(qormHwSync, 3000); }
}
function qormHwSync(){
  if(!qormHasNative() || document.hidden) return;
  if(document.querySelector('.qorm-volume')) qormToNative('volumeGet');
  if(document.querySelector('.qorm-brightness')) qormToNative('brightnessGet');
  if(document.querySelector('.qorm-battery')) qormToNative('battery');
  if(document.querySelector('.qorm-network')) qormToNative('networkStatus');
}
function qormVol(btn,d){ if(qormHasNative()){ qormToNative(d>0?'volumeUp':'volumeDown'); } else { btn.closest('.qorm-volume').querySelector('.qorm-volume-out').textContent='needs the QORM Dev app'; } }
window.__qv={level:0,muted:false};
function qormVolRender(){ document.querySelectorAll('.qorm-volume-out').forEach(function(o){ o.textContent='Volume: '+(window.__qv.muted?'Muted':(Math.round(window.__qv.level*100)+'%')); }); }
function qormOnVolume(level){ window.__qv.level=level; qormVolRender(); }
function qormOnMute(muted){ window.__qv.muted=!!muted; qormVolRender(); }
function qormBright(btn,d){ if(qormHasNative()){ qormToNative(d>0?'brightnessUp':'brightnessDown'); } else { btn.closest('.qorm-brightness').querySelector('.qorm-brightness-out').textContent='needs the QORM Dev app'; } }
function qormOnBrightness(level){ document.querySelectorAll('.qorm-brightness-out').forEach(function(o){ o.textContent='Brightness: '+Math.round(level*100)+'%'; }); }
function qormVibrate(btn){ var out=btn.closest('.qorm-vibrate').querySelector('.qorm-vibrate-out'); if(qormHasNative()){ qormToNative('vibrate'); out.textContent='Vibrated '; } else if(navigator.vibrate){ navigator.vibrate(200); out.textContent='Vibrated '; } else { out.textContent='not supported'; } }
function qormTorch(btn){ var out=btn.closest('.qorm-torch').querySelector('.qorm-torch-out'); if(qormHasNative()){ qormToNative('torchToggle'); } else { out.textContent='needs the QORM Dev app'; } }
function qormOnTorch(on){ document.querySelectorAll('.qorm-torch-out').forEach(function(o){ o.textContent='Flashlight: '+(on?'ON':'OFF'); }); }
function qormBattery(btn){ var out=btn.closest('.qorm-battery').querySelector('.qorm-battery-out'); out.textContent='…'; if(qormHasNative()){ qormToNative('battery'); } else if(navigator.getBattery){ navigator.getBattery().then(function(b){ qormOnBattery(b.level, b.charging); }); } else { out.textContent='needs the QORM Dev app'; } }
function qormOnBattery(level,charging){ document.querySelectorAll('.qorm-battery-out').forEach(function(o){ o.textContent='Battery: '+Math.round(level*100)+'%'+(charging?' ':''); }); }
function qormScreenshot(btn){
  var out=btn.closest('.qorm-screenshot').querySelector('.qorm-screenshot-out'); out.textContent='capturing…';
  if(qormHasNative()){ qormToNative('screenshot'); return; }
  if(navigator.mediaDevices&&navigator.mediaDevices.getDisplayMedia){
    navigator.mediaDevices.getDisplayMedia({video:true}).then(function(stream){
      var v=document.createElement('video'); v.srcObject=stream; v.play();
      v.onloadedmetadata=function(){ var c=document.createElement('canvas'); c.width=v.videoWidth; c.height=v.videoHeight;
        c.getContext('2d').drawImage(v,0,0); stream.getTracks().forEach(function(t){t.stop();});
        qormOnScreenshot(c.toDataURL('image/png')); };
    }).catch(function(e){ out.textContent='denied: '+e.name; });
  } else { out.textContent='not supported here'; }
}
function qormOnScreenshot(dataURL){ document.querySelectorAll('.qorm-screenshot-out').forEach(function(o){ o.innerHTML = dataURL ? '<img src="'+dataURL+'" style="max-width:100%;border-radius:8px;display:block">' : 'capture failed'; }); }
var __qormRec=null, __qormRecChunks=[];
function qormScreenRecord(btn){
  var box=btn.closest('.qorm-screenrecord'), out=box.querySelector('.qorm-screenrecord-out');
  if(qormHasNative()){ var on=box.getAttribute('data-rec')==='1'; box.setAttribute('data-rec',on?'0':'1'); btn.textContent=on?'Start Recording':'Stop Recording'; qormToNative(on?'screenRecordStop':'screenRecordStart'); return; }
  if(!__qormRec){
    if(!(navigator.mediaDevices&&navigator.mediaDevices.getDisplayMedia&&window.MediaRecorder)){ out.textContent='not supported here'; return; }
    navigator.mediaDevices.getDisplayMedia({video:true,audio:true}).then(function(stream){
      __qormRecChunks=[]; __qormRec=new MediaRecorder(stream);
      __qormRec.ondataavailable=function(e){ if(e.data.size) __qormRecChunks.push(e.data); };
      __qormRec.onstop=function(){ stream.getTracks().forEach(function(t){t.stop();});
        var blob=new Blob(__qormRecChunks,{type:'video/webm'}); var url=URL.createObjectURL(blob);
        out.innerHTML='<video src="'+url+'" controls style="max-width:100%;border-radius:8px;display:block"></video>'; __qormRec=null; };
      __qormRec.start(); out.textContent='recording…'; btn.textContent='Stop Recording';
    }).catch(function(e){ out.textContent='denied: '+e.name; });
  } else { __qormRec.stop(); btn.textContent='Start Recording'; }
}
function qormOnScreenRecord(msg){ document.querySelectorAll('.qorm-screenrecord-out').forEach(function(o){ o.textContent=msg||''; }); }
function qormShare(btn){ var out=btn.closest('.qorm-share').querySelector('.qorm-share-out'); var d={text:'Shared from my QORM app',url:location.href};
  if(qormHasNative()){ qormToNative('share',d); out.textContent='opening share sheet…'; }
  else if(navigator.share){ navigator.share(d).then(function(){out.textContent='shared ';}).catch(function(e){out.textContent=e.name==='AbortError'?'cancelled':'error';}); }
  else { out.textContent='share not supported here'; } }
function qormOnShare(ok){ document.querySelectorAll('.qorm-share-out').forEach(function(o){ o.textContent=ok?'shared':'cancelled'; }); }
function qormClipboard(btn){ var out=btn.closest('.qorm-clipboard').querySelector('.qorm-clipboard-out'); var text='QORM  '+new Date().toLocaleTimeString();
  if(qormHasNative()){ qormToNative('clipboardSet',{text:text}); out.textContent='copied: '+text; }
  else if(navigator.clipboard){ navigator.clipboard.writeText(text).then(function(){out.textContent='copied: '+text;}).catch(function(){out.textContent='denied';}); }
  else { out.textContent='clipboard not supported'; } }
function qormOnClipboard(text){ document.querySelectorAll('.qorm-clipboard-out').forEach(function(o){ o.textContent='clipboard: '+text; }); }
function qormDeviceInfo(btn){ var out=btn.closest('.qorm-deviceinfo').querySelector('.qorm-deviceinfo-out'); out.textContent='…';
  if(qormHasNative()){ qormToNative('deviceInfo'); }
  else { qormOnDeviceInfo(JSON.stringify({platform:'web',ua:navigator.platform,lang:navigator.language,screen:screen.width+'x'+screen.height})); } }
function qormOnDeviceInfo(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} var t=Object.keys(d).map(function(k){return k+': '+d[k];}).join('\n'); document.querySelectorAll('.qorm-deviceinfo-out').forEach(function(o){ o.textContent=t||'—'; }); }
function qormNetwork(btn){ var out=btn.closest('.qorm-network').querySelector('.qorm-network-out'); out.textContent='…';
  if(qormHasNative()){ qormToNative('networkStatus'); }
  else { qormOnNetwork(JSON.stringify({online:navigator.onLine,type:(navigator.connection&&navigator.connection.effectiveType)||'unknown'})); } }
function qormOnNetwork(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} document.querySelectorAll('.qorm-network-out').forEach(function(o){ o.textContent=(d.online?'online':'offline')+' · '+(d.type||'?'); }); }
function qormKeepAwake(btn){ var box=btn.closest('.qorm-keepawake'), out=box.querySelector('.qorm-keepawake-out'); var on=box.getAttribute('data-on')==='1'; box.setAttribute('data-on',on?'0':'1'); btn.textContent=on?'Keep Screen Awake':'Allow Sleep';
  if(qormHasNative()){ qormToNative('keepAwake',{on:!on}); out.textContent=on?'sleep allowed':'staying awake '; }
  else if('wakeLock'in navigator){ if(!on){ navigator.wakeLock.request('screen').then(function(w){ window.__qormWake=w; out.textContent='staying awake '; }).catch(function(){out.textContent='denied';}); } else { if(window.__qormWake){window.__qormWake.release();window.__qormWake=null;} out.textContent='sleep allowed'; } }
  else { out.textContent='wake lock not supported'; } }
function qormHaptic(btn){ var out=btn.closest('.qorm-haptics').querySelector('.qorm-haptics-out'); var type=btn.getAttribute('data-type')||'success';
  if(qormHasNative()){ qormToNative('haptic',{type:type}); out.textContent='haptic: '+type; }
  else if(navigator.vibrate){ navigator.vibrate(type==='error'?[80,40,80]:type==='warning'?[40,40]:30); out.textContent='vibrated: '+type; }
  else { out.textContent='haptics not supported'; } }
function qormStorage(btn){ var out=btn.closest('.qorm-storage').querySelector('.qorm-storage-out'); var v='saved@'+new Date().toLocaleTimeString();
  if(qormHasNative()){ qormToNative('storageSet',{key:'qorm_demo',value:v}); qormToNative('storageGet',{key:'qorm_demo'}); out.textContent='saving…'; }
  else { try{ localStorage.setItem('qorm_demo',v); qormOnStorage('qorm_demo', localStorage.getItem('qorm_demo')); }catch(e){ out.textContent='storage denied'; } } }
function qormOnStorage(key,value){ document.querySelectorAll('.qorm-storage-out').forEach(function(o){ o.textContent=key+' = '+value; }); }
var __qormSR=null;
function qormListen(btn){ var out=btn.closest('.qorm-stt').querySelector('.qorm-stt-out'); var lang=btn.getAttribute('data-lang')||navigator.language||'en-US';
  if(qormHasNative()){ qormToNative('listenStart',{lang:lang}); out.textContent='listening'; return; }
  var SR=window.SpeechRecognition||window.webkitSpeechRecognition;
  if(!SR){ out.textContent='STT not supported here'; return; }
  __qormSR=new SR(); __qormSR.interimResults=true; __qormSR.lang=lang;
  __qormSR.onresult=function(e){ var t=''; for(var i=0;i<e.results.length;i++) t+=e.results[i][0].transcript; qormOnSpeech(t); };
  __qormSR.onerror=function(e){ out.textContent='error: '+e.error; };
  __qormSR.start(); out.textContent='listening'; }
function qormOnSpeech(text){ document.querySelectorAll('.qorm-stt-out').forEach(function(o){ o.textContent = text||'(no speech)'; }); }
function qormSecureSave(btn){ var out=btn.closest('.qorm-securestorage').querySelector('.qorm-securestorage-out'); var v='secret@'+new Date().toLocaleTimeString();
  if(qormHasNative()){ qormToNative('secureSet',{key:'qorm_secret',value:v}); qormToNative('secureGet',{key:'qorm_secret'}); out.textContent='saving securely'; }
  else { try{ localStorage.setItem('qorm_secret',v); qormOnSecure('qorm_secret', localStorage.getItem('qorm_secret')); }catch(e){ out.textContent='denied'; } } }
function qormOnSecure(key,value){ document.querySelectorAll('.qorm-securestorage-out').forEach(function(o){ o.textContent='secure['+key+'] = '+value; }); }
function qormPickFile(btn){ var out=btn.closest('.qorm-filepicker').querySelector('.qorm-filepicker-out');
  if(qormHasNative()){ qormToNative('pickFile'); out.textContent='opening picker'; return; }
  var inp=document.createElement('input'); inp.type='file';
  inp.onchange=function(){ var f=inp.files[0]; if(!f) return; var r=new FileReader(); r.onload=function(){ qormOnFile(JSON.stringify({name:f.name,size:f.size,dataURL:r.result})); }; r.readAsDataURL(f); };
  inp.click(); }
function qormOnFile(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} document.querySelectorAll('.qorm-filepicker-out').forEach(function(o){ o.textContent = d.name ? (d.name+' ('+(d.size||0)+' bytes)') : 'no file'; }); }
function qormPickPhoto(btn){ var out=btn.closest('.qorm-photopicker').querySelector('.qorm-photopicker-out');
  if(qormHasNative()){ qormToNative('pickPhoto'); out.textContent='opening picker'; return; }
  var inp=document.createElement('input'); inp.type='file'; inp.accept='image/*';
  inp.onchange=function(){ var f=inp.files[0]; if(!f) return; var r=new FileReader(); r.onload=function(){ qormOnPhoto(r.result); }; r.readAsDataURL(f); };
  inp.click(); }
function qormOnPhoto(dataURL){ document.querySelectorAll('.qorm-photopicker-out').forEach(function(o){ o.innerHTML = dataURL ? '<img src="'+dataURL+'" style="max-width:100%;border-radius:8px;display:block">' : 'no photo'; }); }
function qormOrientation(btn){ var box=btn.closest('.qorm-orientation'), out=box.querySelector('.qorm-orientation-out'); var mode=box.getAttribute('data-mode')==='landscape'?'portrait':'landscape'; box.setAttribute('data-mode',mode); btn.textContent='Lock '+(mode==='portrait'?'Landscape':'Portrait');
  if(qormHasNative()){ qormToNative('lockOrientation',{mode:mode}); out.textContent='locked '+mode; }
  else if(screen.orientation&&screen.orientation.lock){ screen.orientation.lock(mode).then(function(){out.textContent='locked '+mode;}).catch(function(e){out.textContent='needs fullscreen'; }); }
  else { out.textContent='orientation lock not supported'; } }
var __qormVR=null,__qormVRChunks=[];
function qormRecordVideo(btn){ var box=btn.closest('.qorm-videocapture'), out=box.querySelector('.qorm-videocapture-out');
  if(qormHasNative()){ qormToNative('recordVideo'); out.textContent='opening camera'; return; }
  if(!__qormVR){
    if(!(navigator.mediaDevices&&window.MediaRecorder)){ out.textContent='not supported here'; return; }
    navigator.mediaDevices.getUserMedia({video:true,audio:true}).then(function(stream){
      __qormVRChunks=[]; __qormVR=new MediaRecorder(stream);
      __qormVR.ondataavailable=function(e){ if(e.data.size) __qormVRChunks.push(e.data); };
      __qormVR.onstop=function(){ stream.getTracks().forEach(function(t){t.stop();}); qormOnVideo(URL.createObjectURL(new Blob(__qormVRChunks,{type:'video/webm'}))); __qormVR=null; };
      __qormVR.start(); out.textContent='recording'; btn.textContent='Stop';
    }).catch(function(e){ out.textContent='denied'; });
  } else { __qormVR.stop(); btn.textContent='Record Video'; } }
function qormOnVideo(url){ document.querySelectorAll('.qorm-videocapture-out').forEach(function(o){ o.innerHTML = url ? '<video src="'+url+'" controls style="max-width:100%;border-radius:8px;display:block"></video>' : 'no video'; }); }
function qormScanQR(btn){ var out=btn.closest('.qorm-qrscan').querySelector('.qorm-qrscan-out');
  if(qormHasNative()){ qormToNative('scanQR'); out.textContent='scanning'; return; }
  if(!('BarcodeDetector' in window)){ out.textContent='QR scan not supported here'; return; }
  navigator.mediaDevices.getUserMedia({video:{facingMode:'environment'}}).then(function(stream){
    var v=document.createElement('video'); v.srcObject=stream; v.setAttribute('playsinline',''); v.play();
    v.style.cssText='max-width:100%;border-radius:8px'; out.innerHTML=''; out.appendChild(v);
    var det=new BarcodeDetector(); var stop=false;
    (function loop(){ if(stop) return; det.detect(v).then(function(codes){ if(codes.length){ stop=true; stream.getTracks().forEach(function(t){t.stop();}); qormOnScan(codes[0].rawValue); } else setTimeout(loop,300); }).catch(function(){ setTimeout(loop,300); }); })();
  }).catch(function(e){ out.textContent='camera denied'; }); }
function qormOnScan(text){ document.querySelectorAll('.qorm-qrscan-out').forEach(function(o){ o.textContent = text ? ('scanned: '+text) : 'no code'; }); }
function qormSpeak(btn){ var out=btn.closest('.qorm-tts').querySelector('.qorm-tts-out'); var text=btn.getAttribute('data-text')||'Hello from your QORM app.'; var lang=btn.getAttribute('data-lang')||navigator.language||'en-US';
  if(qormHasNative()){ qormToNative('speak',{text:text,lang:lang}); out.textContent='speaking'; }
  else if(window.speechSynthesis){ window.speechSynthesis.cancel(); var u=new SpeechSynthesisUtterance(text); u.lang=lang; window.speechSynthesis.speak(u); out.textContent='speaking'; }
  else { out.textContent='TTS not supported'; } }
// Canonical full-word trigger aliases — the abbreviated qormVol/qormGeo/... stay
// for existing rendered HTML, but qorm<Capability> is the documented, derivable
// name a developer or agent can reach for without memorizing an abbreviation.
var qormVolume=qormVol,qormLocation=qormGeo,qormRecorder=qormRec,qormBiometric=qormBio,qormBrightness=qormBright,qormSensors=qormMotion;
function qormHeading(btn){ var out=btn.closest('.qorm-compass').querySelector('.qorm-compass-out');
  if(qormHasNative()){ qormToNative('headingStart'); out.textContent='reading'; return; }
  function h(e){ var d=e.webkitCompassHeading!=null?e.webkitCompassHeading:(e.alpha!=null?360-e.alpha:null); if(d!=null) qormOnHeading(d); }
  if(window.DeviceOrientationEvent){ window.addEventListener('deviceorientationabsolute',h,{once:false}); window.addEventListener('deviceorientation',h,{once:false}); out.textContent='reading'; }
  else { out.textContent='compass not supported here'; } }
function qormOnHeading(deg){ document.querySelectorAll('.qorm-compass-out').forEach(function(o){ o.textContent=Math.round(deg)+'°'; }); }
function qormProximity(btn){ var out=btn.closest('.qorm-proximity').querySelector('.qorm-proximity-out'); if(qormHasNative()){ qormToNative('proximityStart'); out.textContent='reading'; } else { out.textContent='needs the QORM app'; } }
function qormOnProximity(near){ document.querySelectorAll('.qorm-proximity-out').forEach(function(o){ o.textContent=near?'near':'far'; }); }
function qormPedometer(btn){ var out=btn.closest('.qorm-pedometer').querySelector('.qorm-pedometer-out'); if(qormHasNative()){ qormToNative('pedometerStart'); out.textContent='counting'; } else { out.textContent='needs the QORM app'; } }
function qormOnSteps(n){ document.querySelectorAll('.qorm-pedometer-out').forEach(function(o){ o.textContent=n+' steps'; }); }
function qormBarometer(btn){ var out=btn.closest('.qorm-barometer').querySelector('.qorm-barometer-out'); if(qormHasNative()){ qormToNative('barometerStart'); out.textContent='reading'; } else { out.textContent='needs the QORM app'; } }
function qormOnPressure(kpa){ document.querySelectorAll('.qorm-barometer-out').forEach(function(o){ o.textContent=(+kpa).toFixed(2)+' kPa'; }); }
function qormPickContact(btn){ var out=btn.closest('.qorm-contacts').querySelector('.qorm-contacts-out');
  if(qormHasNative()){ qormToNative('pickContact'); out.textContent='opening picker'; return; }
  if(navigator.contacts&&navigator.contacts.select){ navigator.contacts.select(['name','tel'],{multiple:false}).then(function(cs){ if(cs.length){ qormOnContact(JSON.stringify({name:(cs[0].name||[''])[0],phone:(cs[0].tel||[''])[0]})); } }).catch(function(){ out.textContent='cancelled'; }); }
  else { out.textContent='contact picker not supported here'; } }
function qormOnContact(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} document.querySelectorAll('.qorm-contacts-out').forEach(function(o){ o.textContent=(d.name||'?')+' '+(d.phone||''); }); }
function qormAddEvent(btn){ var out=btn.closest('.qorm-calendar').querySelector('.qorm-calendar-out'); if(qormHasNative()){ qormToNative('addEvent',{title:'QORM Event'}); out.textContent='adding'; } else { out.textContent='needs the QORM app'; } }
function qormOnCalendar(msg){ document.querySelectorAll('.qorm-calendar-out').forEach(function(o){ o.textContent=msg||''; }); }
function qormGetModes(btn){ var out=btn.closest('.qorm-systemmodes').querySelector('.qorm-systemmodes-out');
  if(qormHasNative()){ qormToNative('getModes'); out.textContent='reading'; return; }
  var m={lowPower:null,darkMode:window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches,airplane:null,dnd:null,online:navigator.onLine};
  qormOnModes(JSON.stringify(m)); }
function qormOnModes(json){ var d; try{d=JSON.parse(json);}catch(e){d={};}
  var parts=[]; function add(k,v){ if(v===null||v===undefined) return; parts.push(k+': '+(v===true?'on':v===false?'off':v)); }
  add('low-power',d.lowPower); add('dark',d.darkMode); add('airplane',d.airplane); add('DND',d.dnd);
  document.querySelectorAll('.qorm-systemmodes-out').forEach(function(o){ o.textContent=parts.join('  ·  ')||'no modes readable here'; }); }
function qormUpdateWidget(title, lines){ if(qormHasNative()){ qormToNative('updateWidget',{title:title,lines:lines||[]}); return true; } return false; }
function qormOnWidget(msg){}
function qormReadCSSInsets(){ var d=document.createElement('div'); d.style.cssText='position:fixed;top:0;left:0;padding-top:env(safe-area-inset-top);padding-bottom:env(safe-area-inset-bottom);padding-left:env(safe-area-inset-left);padding-right:env(safe-area-inset-right);visibility:hidden;'; document.body.appendChild(d); var s=getComputedStyle(d); var r={top:parseFloat(s.paddingTop)||0,bottom:parseFloat(s.paddingBottom)||0,left:parseFloat(s.paddingLeft)||0,right:parseFloat(s.paddingRight)||0}; d.parentNode.removeChild(d); return r; }
function qormGetInsets(btn){ var out=btn.closest('.qorm-insets').querySelector('.qorm-insets-out'); if(qormHasNative()){ qormToNative('getInsets'); out.textContent='reading'; return; } qormOnInsets(JSON.stringify(qormReadCSSInsets())); }
function qormOnInsets(json){ var d; try{d=JSON.parse(json);}catch(e){d={};} document.querySelectorAll('.qorm-insets-out').forEach(function(o){ o.textContent='top '+(d.top||0)+' · bottom '+(d.bottom||0)+' · left '+(d.left||0)+' · right '+(d.right||0); });
  document.documentElement.style.setProperty('--safe-top',(d.top||0)+'px'); document.documentElement.style.setProperty('--safe-bottom',(d.bottom||0)+'px'); document.documentElement.style.setProperty('--safe-left',(d.left||0)+'px'); document.documentElement.style.setProperty('--safe-right',(d.right||0)+'px'); }
// Chromeless-window dragging: a [data-qorm-drag] region moves the desktop window.
if (typeof document !== 'undefined') document.addEventListener('mousedown', function(e){
  if (e.button !== 0 || !window.qormDesktop) return;
  var h = e.target.closest && e.target.closest('[data-qorm-drag]');
  if (!h) return;
  var sx = e.screenX, sy = e.screenY;
  qormToNative('winDragStart');
  function mv(ev){ qormToNative('winDragMove', {dx: ev.screenX - sx, dy: ev.screenY - sy}); }
  function up(){ document.removeEventListener('mousemove', mv); document.removeEventListener('mouseup', up); }
  document.addEventListener('mousemove', mv); document.addEventListener('mouseup', up);
});
// Desktop right-click context menu: position at cursor, hover submenus, select.
if (typeof document !== 'undefined') {
  document.addEventListener('contextmenu', function(e){
    var host = e.target.closest && e.target.closest('.qorm-ctxmenu');
    if(!host) return;
    e.preventDefault();
    document.querySelectorAll('.qorm-ctxmenu-panel').forEach(function(p){ p.style.display='none'; });
    var panel = host.querySelector('.qorm-ctxmenu-panel');
    if(!panel) return;
    panel.style.display='block';
    var x=Math.min(e.clientX, window.innerWidth - panel.offsetWidth - 8);
    var y=Math.min(e.clientY, window.innerHeight - panel.offsetHeight - 8);
    panel.style.left=Math.max(4,x)+'px'; panel.style.top=Math.max(4,y)+'px';
  });
  document.addEventListener('click', function(e){
    var item = e.target.closest && e.target.closest('.qorm-ctxmenu-item');
    if(item && !item.parentElement.classList.contains('qorm-ctxmenu-sub')){
      var id=item.getAttribute('data-id'); if(id) qormEmit('context', {id:id});
    }
    if(!(e.target.closest && e.target.closest('.qorm-ctxmenu-sub')))
      document.querySelectorAll('.qorm-ctxmenu-panel').forEach(function(p){ p.style.display='none'; });
  });
  document.addEventListener('mouseover', function(e){
    if(!(e.target.closest && e.target.closest('.qorm-ctxmenu-panel'))) return;
    var sub = e.target.closest('.qorm-ctxmenu-sub');
    document.querySelectorAll('.qorm-ctxmenu-subpanel').forEach(function(p){ if(!sub || !sub.contains(p)) p.style.display='none'; });
    if(sub){ var sp=sub.querySelector('.qorm-ctxmenu-subpanel'); if(sp) sp.style.display='block'; }
  });
  document.addEventListener('keydown', function(e){ if(e.key==='Escape') document.querySelectorAll('.qorm-ctxmenu-panel').forEach(function(p){ p.style.display='none'; }); });
}
function qormOpenUrl(btn){ var out=btn.closest('.qorm-openurl').querySelector('.qorm-openurl-out'); var url=btn.getAttribute('data-url')||'https://example.com';
  if(qormHasNative()){ qormToNative('openURL',{url:url}); out.textContent='opening '+url; }
  else { window.open(url,'_blank'); out.textContent='opened '+url; } }
function qormOnOpenUrl(ok){ document.querySelectorAll('.qorm-openurl-out').forEach(function(o){ o.textContent=ok?'opened':'could not open'; }); }
function qormNfc(btn){ var out=btn.closest('.qorm-nfc').querySelector('.qorm-nfc-out'); out.textContent='Hold a tag near the phone…'; if(qormHasNative()){ qormToNative('nfcRead'); } else { out.textContent='NFC needs the QORM Dev app'; } }
function qormOnNfc(json){ var d; try{ d=JSON.parse(json); }catch(e){ d={}; } document.querySelectorAll('.qorm-nfc-out').forEach(function(o){ o.textContent = d.error ? d.error : ('Tag: '+(d.text||d.id||'read')); }); }
function qormWifi(btn){ var out=btn.closest('.qorm-wifi').querySelector('.qorm-wifi-out'); out.textContent='…'; if(qormHasNative()){ qormToNative('wifiInfo'); } else { out.textContent='Wi-Fi needs the QORM Dev app'; } }
function qormOnWifi(json){ var d; try{ d=JSON.parse(json); }catch(e){ d={}; }
  document.querySelectorAll('.qorm-wifi-out').forEach(function(o){ o.textContent = d.error ? d.error : ('SSID: '+(d.ssid||'unknown')+(typeof d.networks!=='undefined' ? ('\n'+d.networks+'networks nearby') : '')); }); }
function qormOnMotion(a,b,g){ document.querySelectorAll('.qorm-motion .qorm-motion-out').forEach(function(o){ o.textContent='α '+Math.round(a)+'°  β '+Math.round(b)+'°  γ '+Math.round(g)+'°'; }); }
// Audio recorder: getUserMedia + MediaRecorder, toggling record/stop; the clip
// is played inline and synced (data URL) into bound state.
function qormRec(btn){
  var box=btn.closest('.qorm-recorder');
  if(qormHasMobileNative()){
    if(box._recording){ qormToNative('recordStop'); btn.textContent='Record'; btn.style.background='var(--danger)'; box._recording=false; }
    else { qormToNative('recordStart'); btn.textContent='Stop'; btn.style.background='#555'; box._recording=true; }
    return;
  }
  if(box._mr && box._mr.state==='recording'){ box._mr.stop(); btn.textContent='Record'; btn.style.background='var(--danger)'; return; }
  navigator.mediaDevices.getUserMedia({audio:true}).then(function(stream){
    var chunks=[], mr=new MediaRecorder(stream); box._mr=mr;
    mr.ondataavailable=function(e){ if(e.data.size) chunks.push(e.data); };
    mr.onstop=function(){
      stream.getTracks().forEach(function(t){ t.stop(); });
      var blob=new Blob(chunks, {type: mr.mimeType || 'audio/webm'}), rd=new FileReader();
      rd.onload=function(){
        var au=box.querySelector('.qorm-rec-audio'); if(au){ au.src=rd.result; au.style.display='block'; }
        var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=rd.result; } qorm(-1);
      };
      rd.readAsDataURL(blob);
    };
    mr.start(); btn.textContent='Stop'; btn.style.background='#555';
  }).catch(function(e){ alert('Microphone error: '+e); });
}
function qormOnAudio(dataURL){
  document.querySelectorAll('.qorm-recorder').forEach(function(box){
    var au=box.querySelector('.qorm-rec-audio'); if(au){ au.src=dataURL; au.style.display='block'; }
    var hid=box.querySelector('input[type=hidden]'); if(hid){ hid.value=dataURL; }
    var btn=box.querySelector('button'); if(btn){ btn.textContent='Record'; btn.style.background='var(--danger)'; }
    box._recording=false;
  });
  qorm(-1);
}
function qormOnAudioError(msg){ document.querySelectorAll('.qorm-recorder').forEach(function(box){ var b=box.querySelector('button'); if(b){b.textContent='Record';b.style.background='var(--danger)';} box._recording=false; }); alert('Recorder: '+msg); }
// Client-side tab switching (no server round-trip).
function qormTab(btn){
  var bar=btn.parentNode, panels=bar.parentNode.querySelectorAll('.qorm-tabpanel');
  bar.querySelectorAll('.qorm-tab').forEach(function(b){ b.classList.remove('qorm-tab-active'); });
  btn.classList.add('qorm-tab-active');
  var idx=btn.getAttribute('data-tab');
  panels.forEach(function(p){ p.style.display = (p.getAttribute('data-panel')===idx)?'block':'none'; });
  qormTabRevealBar(bar);
}
// ---- tabs: reveal + swipe ----------------------------------------------------
// Two things CSS cannot do for a `tabs` widget, both derived from the live DOM
// at the moment they run, so neither keeps state a re-render could invalidate.
//
// qormTabReveal scrolls a `scrollable` tab bar so the ACTIVE tab is on screen.
// scroll-snap only parks where the user already scrolled to; a tab selected by
// state, by a swipe, or by an agent can therefore sit off-screen with nothing
// to tell the user which one is live. Bars that do not overflow are untouched.
function qormTabRevealBar(bar){
  if(!bar || !bar.classList || !bar.classList.contains('qorm-tabbar')) return;
  if(bar.scrollWidth<=bar.clientWidth+1) return;          // not scrollable: nothing to reveal
  var el=bar.querySelector('.qorm-tab-active'); if(!el) return;
  // Rect deltas, not offsetLeft: the bar is statically positioned, so a tab's
  // offsetParent is some ancestor and offsetLeft would be measured against it.
  var br=bar.getBoundingClientRect(), er=el.getBoundingClientRect();
  var want=bar.scrollLeft+(er.left-br.left)-(br.width-er.width)/2;
  want=Math.max(0, Math.min(bar.scrollWidth-bar.clientWidth, Math.round(want)));
  if(Math.abs(bar.scrollLeft-want)>1) bar.scrollLeft=want;  // write only on a real difference
}
function qormTabReveal(){
  document.querySelectorAll('.qorm-tabbar').forEach(function(bar){ qormTabRevealBar(bar); });
}
// qormTabItems / qormTabActive / qormTabActivate read one tabs widget's live
// tab strip. Activating a tab SYNTHESIZES the tap the user would have made
// rather than reimplementing what a tap does, which is what makes the swipe
// work identically in both modes with no handler bookkeeping of its own:
//   - uncontrolled  -> the <button>'s own onclick (qormTab, or qorm(h) when the
//                      node declares onChange)
//   - controlled    -> the hidden radio inside the <label>, so checking it fires
//                      the same change event (qorm(-1) / qorm(h)) a tap fires,
//                      and the bound state path is written by the normal sync.
// Every index and handler is therefore re-read from the DOM at event time; the
// gesture closes over nothing a re-render can renumber.
function qormTabItems(root){
  var bar=root.querySelector('.qorm-tabbar');
  return bar ? Array.prototype.slice.call(bar.querySelectorAll('.qorm-tab')) : [];
}
function qormTabActive(root){
  var items=qormTabItems(root);
  for(var i=0;i<items.length;i++){ if(items[i].classList.contains('qorm-tab-active')) return i; }
  return 0;
}
function qormTabActivate(root, i){
  var items=qormTabItems(root);
  if(i<0 || i>=items.length) return false;               // at an end: no wrap-around
  var radio=items[i].querySelector('input[type=radio]');
  (radio||items[i]).click();
  return true;
}
// qormTabScrollsX reports whether the swipe started inside something that can
// itself scroll horizontally (a carousel, a wide table, a nested tab bar). That
// content owns the gesture — stealing it would make those widgets unusable
// inside a tab panel.
function qormTabScrollsX(el, root){
  for(var p=el; p && p!==root && p.nodeType===1; p=p.parentElement){
    if(p.scrollWidth>p.clientWidth+1){
      var ox=getComputedStyle(p).overflowX;
      if(ox==='auto'||ox==='scroll') return true;
    }
  }
  return false;
}
// qormTabSwipeInit installs the panel swipe: drag a `tabs` panel sideways to
// move to the neighbouring tab, the gesture every phone user expects and the
// reason `swipe: true` exists on the node.
//
// IDEMPOTENCE: ONE delegated document listener behind __qormTabSwipeReady, so a
// morph (which does not re-run inline scripts, and would otherwise stack a
// second listener per re-render) changes nothing. It owns NO client state —
// the active index is read from .qorm-tab-active at event time — so there is
// nothing for a reconcile pass to repair; only qormTabReveal runs from
// qormMorphInto, and it too recomputes from the live DOM.
function qormTabSwipeInit(){
  if(window.__qormTabSwipeReady) return; window.__qormTabSwipeReady=true;
  document.addEventListener('pointerdown', function(e){
    if(e.button && e.button!==0) return;                 // primary button only
    var t=e.target;
    if(!t || !t.closest) return;
    var panel=t.closest('.qorm-tabpanel'); if(!panel) return;
    var root=panel.closest('[data-qorm-tabs]'); if(!root) return;   // opt-in marker
    if(t.closest('input,textarea,select')) return;       // typing/dragging a control
    if(qormTabScrollsX(t, panel)) return;                // inner scroller owns it
    var x0=e.clientX, y0=e.clientY, done=false;
    function onMove(ev){
      if(done) return;
      var dx=ev.clientX-x0, dy=ev.clientY-y0;
      if(Math.abs(dx)<48) return;                        // travel threshold
      if(Math.abs(dx)<Math.abs(dy)*1.5) return;          // vertical scroll, not a swipe
      done=true; cleanup();
      qormTabActivate(root, qormTabActive(root)+(dx<0?1:-1));
    }
    function cleanup(){ document.removeEventListener('pointermove',onMove); document.removeEventListener('pointerup',onUp); document.removeEventListener('pointercancel',onUp); }
    function onUp(){ cleanup(); }
    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
    document.addEventListener('pointercancel', onUp);
  });
}
// Accordion: toggle the panel following the clicked header.
//
// `single: true` renders data-qorm-acc="single" on the accordion root and makes
// the panels EXCLUSIVE — opening one closes the rest, the classic accordion.
// The mode is read off the live DOM at click time (never captured), and the
// default, where every panel toggles independently, is untouched.
function qormAcc(btn){
  var p=btn.nextElementSibling; if(!p) return;
  var open=(p.style.display==='none');
  var root=btn.closest && btn.closest('[data-qorm-acc="single"]');
  if(open && root){
    root.querySelectorAll('.qorm-acc-panel').forEach(function(q){ if(q!==p) q.style.display='none'; });
  }
  p.style.display = open ? 'block' : 'none';
}
// ---- carousel: autoplay + indicator dots -------------------------------------
// A `carousel` is a CSS scroll-snap track, so paging it is free — what needs JS
// is advancing it on a clock and reflecting the live scroll position on the
// indicator dots.
//
// IDEMPOTENCE, the same shape as the timer registry: __qormCarousels is the
// only client-owned state, qormCarouselSync reconciles it against the live DOM
// after every morph (same track + same interval is never rescheduled, a changed
// interval reschedules, a track that left the DOM is stopped and forgotten),
// and it is called from qormMorphInto. The registry is keyed by the ELEMENT
// rather than an id because morphEl mutates a surviving node in place, so the
// identity is stable exactly as long as the widget is; the interval itself is
// re-read from data-qorm-carousel on every pass, never closed over. The dots
// hold no state at all — the active one is derived from scrollLeft each time.
window.__qormCarousels = window.__qormCarousels || [];
function qormCarouselIndex(el){
  var r=el.getBoundingClientRect(), best=0, bd=Infinity;
  for(var i=0;i<el.children.length;i++){
    var d=Math.abs(el.children[i].getBoundingClientRect().left-r.left);
    if(d<bd){ bd=d; best=i; }
  }
  return best;
}
function qormCarouselGo(el, i){
  var n=el.children.length; if(!n) return;
  if(i>=n) i=0; else if(i<0) i=n-1;                      // autoplay wraps
  var r=el.getBoundingClientRect(), cr=el.children[i].getBoundingClientRect();
  var want=el.scrollLeft+(cr.left-r.left);
  try{ el.scrollTo({left:want, behavior:'smooth'}); }catch(_){ el.scrollLeft=want; }
}
// qormCarouselDots marks the dot matching the track's live scroll position. The
// row is the track's next sibling ([data-qorm-dots]) — the renderer emits it
// only when `indicators` is on, so this is a no-op for a plain carousel.
function qormCarouselDots(el){
  var row=el.nextElementSibling;
  if(!row || !row.getAttribute || row.getAttribute('data-qorm-dots')===null) return;
  var at=qormCarouselIndex(el);
  for(var i=0;i<row.children.length;i++){
    var d=row.children[i], on=(i===at);
    d.setAttribute('aria-current', on?'true':'false');
    d.style.background = on ? 'var(--accent)' : 'var(--sep)';
  }
}
function qormCarouselTick(el){
  if(!document.contains(el)) return;                     // gone; sync will prune it
  if(document.hidden) return;                            // no work in a hidden tab
  if(el.matches && el.matches(':hover')) return;         // pause while pointed at
  qormCarouselGo(el, qormCarouselIndex(el)+1);
}
function qormCarouselEntry(el){
  var l=window.__qormCarousels;
  for(var i=0;i<l.length;i++){ if(l[i].el===el) return l[i]; }
  return null;
}
function qormCarouselSync(){
  var l=window.__qormCarousels, i;
  for(i=l.length-1;i>=0;i--){                            // forget tracks that left the DOM
    if(!document.contains(l[i].el)){ clearInterval(l[i].h); l.splice(i,1); }
  }
  document.querySelectorAll('[data-qorm-carousel]').forEach(function(el){
    var ms=parseInt(el.getAttribute('data-qorm-carousel'),10)||0;
    if(ms>0&&ms<250) ms=250;                             // same floor as declarative timers
    var e=qormCarouselEntry(el);
    if(!e || e.ms!==ms){                                 // unchanged: never double-schedule
      if(e){ clearInterval(e.h); l.splice(l.indexOf(e),1); }
      if(ms>0) l.push({el:el, ms:ms, h:setInterval(function(){ qormCarouselTick(el); }, ms)});
    }
  });
  document.querySelectorAll('[data-qorm-dots]').forEach(function(row){
    var el=row.previousElementSibling; if(el) qormCarouselDots(el);
  });
}
function qormCarouselInit(){
  if(window.__qormCarouselReady) return; window.__qormCarouselReady=true;
  // Scroll does not bubble but does propagate through the capture phase, so one
  // document listener follows every track without wiring any of them.
  document.addEventListener('scroll', function(e){
    var el=e.target;
    if(el && el.nodeType===1 && el.nextElementSibling &&
       el.nextElementSibling.getAttribute && el.nextElementSibling.getAttribute('data-qorm-dots')!==null){
      qormCarouselDots(el);
    }
  }, true);
  // Tapping a dot jumps to that slide; the index comes from the dot's position
  // in the live row, so it survives any re-render.
  document.addEventListener('click', function(e){
    var t=e.target; if(!t || !t.closest) return;
    var row=t.closest('[data-qorm-dots]'); if(!row) return;
    var dot=t.closest('[data-qorm-dot]'); if(!dot) return;
    var el=row.previousElementSibling; if(!el) return;
    qormCarouselGo(el, Array.prototype.indexOf.call(row.children, dot));
  });
  qormCarouselSync();
}
// Menu: toggle the dropdown panel; close others.
function qormMenu(btn){
  var panel=btn.nextElementSibling;
  document.querySelectorAll('.qorm-menu-panel').forEach(function(p){ if(p!==panel) p.style.display='none'; });
  if(panel){ panel.style.display = (panel.style.display==='none')?'block':'none'; }
}
// Default dismiss: Escape closes the topmost dismissable overlay. Overlays
// with a plainly state-bound `open` carry data-dismiss-h — the handler index
// of the runtime's built-in __dismiss action (registered by the renderer).
document.addEventListener('keydown',function(e){
  if(e.key!=='Escape') return;
  var els=document.querySelectorAll('[data-dismiss-h]');
  if(!els.length) return;
  qorm(parseInt(els[els.length-1].getAttribute('data-dismiss-h'),10));
});
// SearchBar: while the input is focused and non-empty, show the anchored panel
// with the entries whose label contains the query (client-side filtering).
function qormSearch(inp){
  var box=inp.closest('.qorm-search'), panel=box&&box.querySelector('.qorm-search-panel'); if(!panel) return;
  var q=(inp.value||'').toLowerCase(), any=false;
  panel.querySelectorAll('.qorm-search-item').forEach(function(it){
    var show=q!=='' && (it.getAttribute('data-label')||'').toLowerCase().indexOf(q)>=0;
    it.style.display=show?'':'none'; if(show) any=true;
  });
  panel.style.display=any?'block':'none';
}
// SearchBar: close the panel on blur (deferred so an entry click lands first).
function qormSearchBlur(inp){
  var box=inp.closest('.qorm-search'), panel=box&&box.querySelector('.qorm-search-panel');
  setTimeout(function(){ if(panel) panel.style.display='none'; },120);
}
// SearchBar: Escape closes the panel.
function qormSearchKey(inp,e){
  if(e.key!=='Escape') return;
  var box=inp.closest('.qorm-search'), panel=box&&box.querySelector('.qorm-search-panel');
  if(panel) panel.style.display='none';
}
// SearchBar: pick a result entry — fill the input (qorm(h) then syncs the
// bound value too), close the panel, and dispatch onSelect with {label}.
function qormSearchPick(item,h){
  var box=item.closest('.qorm-search'); if(!box){ qorm(h); return; }
  var inp=box.querySelector('input'); if(inp) inp.value=item.getAttribute('data-label')||'';
  var panel=box.querySelector('.qorm-search-panel'); if(panel) panel.style.display='none';
  qorm(h);
}
// Context menu (CupertinoContextMenu): long-press to reveal the action panel.
function qormCtx(el){
  var t=null, panel=el.querySelector('.qorm-ctx-panel');
  el.addEventListener('pointerdown',function(){ t=setTimeout(function(){ if(panel){ panel.style.display='flex'; } },480); });
  ['pointerup','pointerleave','pointermove'].forEach(function(ev){ el.addEventListener(ev,function(){ if(t){ clearTimeout(t); t=null; } }); });
}
// Pull-to-refresh (RefreshIndicator): drag down from the top past threshold to
// fire handler h.
function qormRefresh(el,h){
  var y0=null, dy=0, sp=el.querySelector('.qorm-refresh-spin');
  el.addEventListener('pointerdown',function(e){ if(el.scrollTop<=0){ y0=e.clientY; } });
  el.addEventListener('pointermove',function(e){ if(y0===null) return; dy=Math.max(0,e.clientY-y0);
    if(sp){ sp.style.height=Math.min(dy,60)+'px'; sp.style.opacity=Math.min(1,dy/60); } });
  var end=function(){ if(y0===null) return; var go=dy>70; if(sp){ sp.style.height=''; sp.style.opacity=''; }
    y0=null; dy=0; if(go) qorm(h); };
  el.addEventListener('pointerup',end); el.addEventListener('pointerleave',end);
}
// Swipe-to-dismiss (Dismissible): drag the content left; past threshold,
// collapse the row and fire handler h (onDismissed).
function qormSwipe(el,h){
  var c=el.querySelector('.qorm-dismiss-content'); if(!c) return;
  var x0=null,dx=0;
  el.addEventListener('pointerdown',function(e){ x0=e.clientX; c.style.transition='none'; });
  el.addEventListener('pointermove',function(e){ if(x0===null) return; dx=Math.min(0,e.clientX-x0); c.style.transform='translateX('+dx+'px)'; });
  var end=function(){ if(x0===null) return; c.style.transition='transform .2s';
    if(dx<-100){ el.style.height=el.offsetHeight+'px'; el.style.overflow='hidden';
      requestAnimationFrame(function(){ el.style.height='0'; el.style.opacity='0'; }); setTimeout(function(){ qorm(h); },210); }
    else { c.style.transform='translateX(0)'; } x0=null; dx=0; };
  el.addEventListener('pointerup',end); el.addEventListener('pointerleave',end);
}
// Long-press: fire handler h after 500ms of a sustained press (GestureDetector).
function qormPostReorder(h, from, to){
  fetch('/event',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},body:JSON.stringify({h:h,rev:__rev,inputs:{_reorderFrom:from,_reorderTo:to}})})
    .then(function(r){ var rv=parseInt(r.headers.get('X-Qorm-Rev'))||0; qormTheme(r.headers.get('X-Qorm-Theme')); return r.text().then(function(html){ return {rv:rv,html:html}; }); })
    .then(function(o){ if(o.rv && o.rv<=__rev) return; if(o.rv) __rev=o.rv; qormMorphInto(document.getElementById('qorm-root'), o.html); });
}
// qormReorder makes a list drag-to-reorder: press-hold an item to pick it up, drag
// it while siblings slide aside to show where it will land, release to commit the
// new order (persisted via a state.move step, so the AI sees it and it survives).
function qormReorder(list, h){
  if(!list) return;
  function items(){ return Array.prototype.filter.call(list.children, function(c){ return c.nodeType===1 && c.tagName!=='SCRIPT'; }); }
  list.addEventListener('pointerdown', function(e){
    var its=items(), item=null, from=-1;
    for(var i=0;i<its.length;i++){ if(its[i].contains(e.target)){ item=its[i]; from=i; break; } }
    if(!item) return;
    if(e.button && e.button!==0) return;   // primary button only
    var y0=e.clientY, started=false, to=from, itemH=item.offsetHeight||44;
    function start(){
      started=true; to=from;
      item.style.transition='none'; item.style.zIndex='20'; item.style.position='relative';
      item.style.boxShadow='0 10px 30px rgba(0,0,0,.22)'; item.style.opacity='.97';
      document.body.style.userSelect='none';
      try{ item.setPointerCapture(e.pointerId); }catch(_){}
    }
    function onMove(ev){
      var dy=ev.clientY-y0;
      if(!started){ if(Math.abs(dy)>5){ start(); } else { return; } }
      ev.preventDefault();
      item.style.transform='translateY('+dy+'px)';
      var nt=Math.max(0, Math.min(its.length-1, from+Math.round(dy/itemH)));
      if(nt!==to){ to=nt;
        its.forEach(function(el,idx){ if(el===item) return; var t=0;
          if(from<to && idx>from && idx<=to) t=-itemH;
          else if(from>to && idx>=to && idx<from) t=itemH;
          el.style.transition='transform .18s'; el.style.transform=t?('translateY('+t+'px)'):''; });
      }
    }
    function onUp(){
      cleanup();
      if(!started) return;
      document.body.style.userSelect='';
      its.forEach(function(el){ el.style.transition=''; el.style.transform=''; el.style.zIndex=''; el.style.boxShadow=''; el.style.opacity=''; el.style.position=''; });
      if(to!==from) qormPostReorder(h, from, to);
    }
    function cleanup(){ document.removeEventListener('pointermove', onMove); document.removeEventListener('pointerup', onUp); }
    document.addEventListener('pointermove', onMove, {passive:false});
    document.addEventListener('pointerup', onUp);
  });
}
function qormLong(el,h){
  var t=null;
  var start=function(){ t=setTimeout(function(){ t=null; qorm(h); },500); };
  var cancel=function(){ if(t){ clearTimeout(t); t=null; } };
  el.addEventListener('pointerdown',start);
  el.addEventListener('pointerup',cancel);
  el.addEventListener('pointerleave',cancel);
}
// Draggable/DragTarget via pointer events (works in the desktop WebView + touch,
// unlike HTML5 drag-and-drop): press a .qorm-draggable (data-qorm-drag=payload),
// drag it onto a .qorm-droptarget (data-qorm-drop=handler), release to fire that
// handler with {_dragData}. One delegated document listener, set up once, so it
// keeps working across re-render morphs (which don't re-run inline scripts).
function qormDragInit(){
  if(window.__qormDragReady) return; window.__qormDragReady=true;
  document.addEventListener('pointerdown', function(e){
    var el=e.target && e.target.closest && e.target.closest('.qorm-draggable'); if(!el) return;
    if(e.button && e.button!==0) return;
    var x0=e.clientX, y0=e.clientY, started=false, cur=null;
    var data=el.getAttribute('data-qorm-drag')||'';
    function start(){ started=true; el.classList.add('qorm-dragging');
      el.style.transition='none'; el.style.position='relative'; el.style.zIndex='1000';
      document.body.style.userSelect='none';
      try{ el.setPointerCapture(e.pointerId); }catch(_){} }
    function targetAt(x,y){ var v=el.style.visibility; el.style.visibility='hidden';
      var n=document.elementFromPoint(x,y); el.style.visibility=v;
      return (n&&n.closest)? n.closest('.qorm-droptarget[data-qorm-drop]') : null; }
    function onMove(ev){ var dx=ev.clientX-x0, dy=ev.clientY-y0;
      if(!started){ if(Math.abs(dx)>4||Math.abs(dy)>4) start(); else return; }
      ev.preventDefault();
      el.style.transform='translate('+dx+'px,'+dy+'px)';
      var t=targetAt(ev.clientX,ev.clientY);
      if(t!==cur){ if(cur) cur.classList.remove('qorm-dragover'); cur=t; if(cur) cur.classList.add('qorm-dragover'); } }
    function onUp(){ cleanup(); if(!started) return;
      document.body.style.userSelect='';
      el.classList.remove('qorm-dragging');
      el.style.transition=''; el.style.transform=''; el.style.position=''; el.style.zIndex='';
      var t=cur; if(cur) cur.classList.remove('qorm-dragover');
      if(t){ var h=parseInt(t.getAttribute('data-qorm-drop')); if(!isNaN(h)) qormPostDrop(h,data); } }
    function cleanup(){ document.removeEventListener('pointermove',onMove); document.removeEventListener('pointerup',onUp); }
    document.addEventListener('pointermove', onMove, {passive:false});
    document.addEventListener('pointerup', onUp);
  });
}
function qormPostDrop(h,data){
  fetch('/event',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},body:JSON.stringify({h:h,rev:__rev,inputs:{_dragData:data}})})
    .then(function(r){ var rv=parseInt(r.headers.get('X-Qorm-Rev'))||0; qormTheme(r.headers.get('X-Qorm-Theme')); return r.text().then(function(html){ return {rv:rv,html:html}; }); })
    .then(function(o){ if(o.rv && o.rv<=__rev) return; if(o.rv) __rev=o.rv; qormMorphInto(document.getElementById('qorm-root'), o.html); });
}
// Live-sync: observe out-of-band changes (e.g. an AI agent editing the same
// session over /mcp) and swap in the new UI. Prefer Server-Sent Events for
// instant multi-client push; fall back to polling.
var __rev=__QORM_REV__;   // revision of the render that produced this page
var __tok='__QORM_TOKEN__';
function qormHighlightNode(nodeId){
  document.querySelectorAll('.qorm-inspect-highlight').forEach(function(el){
    el.style.outline = '';
    el.style.outlineOffset = '';
    el.classList.remove('qorm-inspect-highlight');
  });
  if(!nodeId) return;
  var target = document.getElementById(nodeId);
  if(target){
    target.classList.add('qorm-inspect-highlight');
    target.style.outline = '3px solid #0a84ff';
    target.style.outlineOffset = '-3px';
    target.scrollIntoView({behavior: 'smooth', block: 'nearest'});
  }
}
function qormTheme(t){ if(!t) return; var st=document.getElementById('qorm-stage'); if(st) st.className='qorm-theme-'+t; }
function qormApply(d){
  if(d&&typeof d.inspectNode!=='undefined'){ qormHighlightNode(d.inspectNode); }
  if(d&&d.theme) qormTheme(d.theme);
  // URL routing: mirror the current deep-link path into the address bar. Done
  // before the rev guard so the human's OWN navigation (whose rev the POST
  // /event response already applied) still updates the URL.
  if(d&&typeof d.route!=='undefined' && window.__qormApplyRoute){ window.__qormApplyRoute(d.route); }
  if(!d||typeof d.rev==='undefined') return;
  if(d.rev<=__rev) return;   // already applied (e.g. via the POST /event response) — no double morph
  __rev=d.rev;
  window.__qormEditSrc=d.source;   // so morph can flag AI-touched nodes for a flash
  window.__qormNav=d.nav||'';      // page-transition direction, if a navigation
  if(typeof d.html!=='undefined'){ qormMorphInto(document.getElementById('qorm-root'), d.html); }
  window.__qormEditSrc=null;
  if(d.source==='agent') qormPresence(d.detail);   // a collaborator (AI) edited — show it live
  __rev=d.rev;
}
// Live edit attribution: when the AI edits the shared app, the human sees it.
function qormPresence(detail){
  var el=document.getElementById('qorm-presence');
  if(!el){ el=document.createElement('div'); el.id='qorm-presence'; document.body.appendChild(el); }
  el.innerHTML='<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2L4 14h7l-1 8 9-12h-7z"/></svg><span>AI edited'+(detail?' · '+detail:'')+'</span>';
  el.classList.add('show');
  clearTimeout(el._t); el._t=setTimeout(function(){ el.classList.remove('show'); }, 2600);
}
if(window.EventSource){
  // Open the stream carrying the revision this page was rendered at (?rev=), so
  // the server can resync us if a mutation landed between the page render and
  // this connection opening. On the auto-reconnect the browser replays the same
  // URL plus the last frame's id; the server takes the newer of the two.
  var es=new EventSource('/events?rev='+__rev);
  es.onmessage=function(e){ try{ qormApply(JSON.parse(e.data)); }catch(_){} };
}else{
  setInterval(function(){
    fetch('/poll?rev='+__rev).then(function(r){return r.json();}).then(qormApply).catch(function(){});
  }, 800);
}
// Human presence: report the element the human focuses or taps, so the agent can
// see (via qorm_activity) what the human is attending to — the reverse direction
// of the "AI edited" flash. Only the nearest interactive element, deduped.
(function(){
  var last='';
  function ping(el){
    var t=el&&el.closest&&el.closest('button,a,input,textarea,select,[data-state]');
    if(!t) return;
    var isPw=(t.tagName==='INPUT' && t.type==='password');
    var lab=(t.getAttribute('aria-label')||(isPw?'password':t.getAttribute('placeholder'))||t.textContent||'').replace(/\s+/g,' ').trim().slice(0,40);
    var d=t.tagName.toLowerCase()+(lab?': '+lab:'');
    // include what the human typed — but a password's value is never sent, only
    // a "(hidden)" marker so the agent knows the field was filled, not its content
    if(isPw){ if(t.value) d+=' = (hidden)'; }
    else if((t.tagName==='INPUT'||t.tagName==='TEXTAREA'||t.tagName==='SELECT') && t.value){ d+=' = '+String(t.value).slice(0,60); }
    if(d===last) return; last=d;
    fetch('/presence',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},body:JSON.stringify({element:d})}).catch(function(){});
  }
  document.addEventListener('focusin',function(e){ ping(e.target); });
  document.addEventListener('pointerdown',function(e){ ping(e.target); });
  document.addEventListener('input',function(e){ ping(e.target); });   // live typing
})();
// ---- collapsing large title: fallback only -----------------------------------
// A `largetitle` collapses with NO JS: its compact bar is position:sticky, so
// the big title scrolls away behind it, and the cross-fade between the two
// titles is a CSS scroll-driven animation over the big title's view() timeline
// (the .qorm-lt rules in the shell stylesheet). This code runs ONLY where the
// browser has no scroll-driven animations: it derives the same collapse
// progress from a scroll listener and toggles .qorm-lt-stuck, which drives the
// identical declarations via a plain transition. A DOM morph resets class
// attributes to the server's markup, so qormSheetSync's sibling here is re-run
// from qormMorphInto — recomputing from the live scroll position, never from
// remembered state, which makes it idempotent however often it runs.
function qormLargeTitleNative(){
  return typeof CSS!=='undefined' && !!CSS.supports && CSS.supports('animation-timeline','view()');
}
function qormScrollParent(el){
  for(var p=el.parentNode; p && p.nodeType===1; p=p.parentNode){
    var oy=getComputedStyle(p).overflowY;
    if((oy==='auto'||oy==='scroll') && p.scrollHeight>p.clientHeight+1) return p;
  }
  return null;
}
function qormLargeTitleSync(){
  if(qormLargeTitleNative()) return;
  document.querySelectorAll('[data-qorm-largetitle]').forEach(function(el){
    var big=el.querySelector('.qorm-lt-big'), bar=el.querySelector('.qorm-lt-bar');
    if(!big||!bar) return;
    var sc=qormScrollParent(el);
    var top=sc?sc.getBoundingClientRect().top:0;
    // Measure the WRAPPER's rect and the two rows' LAYOUT heights: the big
    // title carries the collapse transform, so reading its own rect would feed
    // the result back into its own input and make the threshold oscillate.
    var scrolled=top-el.getBoundingClientRect().top;
    var h=big.offsetHeight||1;
    el.classList.toggle('qorm-lt-stuck', (scrolled-bar.offsetHeight)/h > 0.55);
  });
}
function qormLargeTitleInit(){
  if(window.__qormLTReady) return; window.__qormLTReady=true;
  // Scroll does not bubble, but it DOES propagate through the capture phase, so
  // one document-level capturing listener covers every nested scroll container
  // (the scaffold body, a scrollview, a list) without wiring any of them.
  document.addEventListener('scroll', qormLargeTitleSync, true);
  qormLargeTitleSync();
}
// ---- draggable sheet (DraggableScrollableSheet) -------------------------------
// A `sheet` renders a bottom panel whose height is one of its snap points; its
// grab row is dragged with pointer events and the panel settles on the nearest
// stop, dispatching that stop's onSnap handler (the renderer registers one per
// stop, so this stays an ordinary qorm(h) with no new event plumbing).
//
// IDEMPOTENCE: the gesture is ONE delegated document listener installed once
// behind __qormSheetReady, and every parameter it needs (the ladder, the
// per-stop handlers, the close handler) is read from data attributes on the
// live panel at event time — so a re-render that renumbers handlers can never
// leave a stale closure behind. The only client-owned state is the live stop
// index, kept in a registry keyed by sheet id: a morph resets the panel's
// inline height to the server-rendered stop, so qormSheetSync re-applies the
// live one after every re-render and forgets sheets that left the DOM.
window.__qormSheets = window.__qormSheets || {};
function qormSheetSnaps(p){
  return (p.getAttribute('data-snaps')||'').split(',')
    .map(function(s){ return parseFloat(s); })
    .filter(function(f){ return f>0; });
}
// qormSheetStop clamps a stop index against the panel's live ladder and records
// it as that sheet's current stop.
function qormSheetStop(p, snaps, i){
  if(!(i>=0)) i=0;                       // also catches undefined/NaN
  if(i>=snaps.length) i=snaps.length-1;
  var id=p.getAttribute('data-qorm-sheet'); if(id) window.__qormSheets[id]=i;
  return i;
}
// qormSheetSet settles the panel on a stop, animating the height change.
function qormSheetSet(p, i, animate){
  var snaps=qormSheetSnaps(p); if(!snaps.length) return -1;
  i=qormSheetStop(p, snaps, i);
  p.style.transition = animate ? 'height .28s cubic-bezier(.32,.72,0,1)' : 'none';
  p.style.height = snaps[i]+'%';
  return i;
}
function qormSheetSync(){
  var live={};
  document.querySelectorAll('[data-qorm-sheet]').forEach(function(p){
    var id=p.getAttribute('data-qorm-sheet'); live[id]=1;
    var snaps=qormSheetSnaps(p); if(!snaps.length) return;
    var i=window.__qormSheets[id];
    if(i===undefined) i=parseInt(p.getAttribute('data-snap'),10)||0;   // first paint: the server's stop
    var want=snaps[qormSheetStop(p, snaps, i)]+'%';
    // Write only on a real difference, so re-running this (which happens after
    // every morph) never restarts a settle animation that is still playing.
    if(p.style.height!==want) p.style.height=want;
  });
  Object.keys(window.__qormSheets).forEach(function(id){ if(!live[id]) delete window.__qormSheets[id]; });
}
function qormSheetInit(){
  if(window.__qormSheetReady) return; window.__qormSheetReady=true;
  document.addEventListener('pointerdown', function(e){
    var grab=e.target && e.target.closest && e.target.closest('.qorm-dsheet-grab'); if(!grab) return;
    var p=grab.closest('[data-qorm-sheet]'); if(!p) return;
    if(e.button && e.button!==0) return;             // primary button only
    var snaps=qormSheetSnaps(p); if(!snaps.length) return;
    var y0=e.clientY, h0=p.offsetHeight, moved=false;
    var box=(p.parentNode && p.parentNode.offsetHeight) || window.innerHeight || 1;
    p.style.transition='none';
    function pct(ev){ return Math.max(2, Math.min(100, (h0-(ev.clientY-y0))/box*100)); }
    function onMove(ev){ moved=true; ev.preventDefault(); p.style.height=pct(ev)+'%'; }
    function onUp(ev){
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      if(!moved){ p.style.transition=''; return; }
      var cur=pct(ev), best=0, i;
      for(i=1;i<snaps.length;i++){ if(Math.abs(snaps[i]-cur)<Math.abs(snaps[best]-cur)) best=i; }
      // Flung below the lowest stop: close instead of snapping back up.
      var closeH=parseInt(p.getAttribute('data-close-h'),10);
      if(cur<snaps[0]*0.6 && !isNaN(closeH) && closeH>=0){ qorm(closeH); return; }
      qormSheetSet(p, best, true);
      var h=parseInt((p.getAttribute('data-snap-h')||'').split(',')[best],10);
      if(!isNaN(h) && h>=0) qorm(h);
    }
    document.addEventListener('pointermove', onMove, {passive:false});
    document.addEventListener('pointerup', onUp);
  });
  qormSheetSync();
}
// ---- windowed lists (list virtualize:"window") --------------------------------
// TRUE virtualization: the server renders only the rows around the scroll
// position and holds the rest of the scroll height open with two spacer divs
// (.qorm-vpad), so a 100k-row list is a few dozen DOM nodes. All this side does
// is MEASURE and REPORT — the window itself is decided in Go (windowSlice in
// internal/render/render_data.go), which is what keeps the rendered frame, the
// agent-visible state and the HTML export in agreement.
//
// The report travels on the ORDINARY event channel: each windowed list carries
// a hidden [data-state] input, so writing "<scrollTop>,<clientHeight>" into it
// and calling qorm(-1) is the same POST /event state-sync a typed character
// makes. No second transport, no new endpoint, and every unrelated dispatch
// carries the live position along for free.
//
// IDEMPOTENCE: ONE delegated (capturing) scroll listener installed once behind
// __qormVListReady — scroll does not bubble but does propagate through capture,
// so this follows every nested scroll container without wiring any of them. The
// listener is rAF-throttled (at most one measurement per frame, however many
// scroll events fire). The only client-owned state is __qormVLists: the last
// metrics reported per list, which exists purely to keep a round trip that is
// still in flight from being sent again, and which qormVListReport CLEARS the
// moment the server's window covers the view again. qormVListSync re-runs from
// qormMorphInto (a fling can outrun a frame, so the newly rendered window has
// to be re-checked) and forgets lists that left the DOM.
window.__qormVLists = window.__qormVLists || {};
// qormVListMetrics measures a list against its scroll port: how far the list's
// top has travelled ABOVE the port's top edge, and how tall that port is. Three
// cases, because a list may be its own scroller (onRefresh gives it overflow),
// may sit inside one (a `scroll` container), or may just ride the page.
function qormVListMetrics(el){
  var oy='';
  try{ oy=getComputedStyle(el).overflowY; }catch(e){}
  if(oy==='auto'||oy==='scroll') return {top:el.scrollTop||0, port:el.clientHeight||0};
  var sp=qormScrollParent(el), r=el.getBoundingClientRect(), top, port;
  if(sp){ top=sp.getBoundingClientRect().top-r.top; port=sp.clientHeight||0; }
  else { top=-r.top; port=window.innerHeight||0; }
  return {top:top>0?top:0, port:port||0};
}
// qormVListReport measures ONE list and reports only when the window the server
// rendered no longer covers the rows the human can see — that is what the
// overscan is for, and it is why scrolling within the rendered window costs
// nothing at all. Every parameter is re-read off the live DOM here, never
// captured, so a re-render that changes the row height, the overscan or the
// window can never be dispatched against stale numbers.
function qormVListReport(el){
  var key=el.getAttribute('data-qorm-vlist'); if(!key) return false;
  var ih=parseFloat(el.getAttribute('data-item-h')); if(!(ih>0)) return false;
  var at=parseInt(el.getAttribute('data-qorm-vstart'),10)||0;
  var count=parseInt(el.getAttribute('data-qorm-vcount'),10)||0;
  var total=parseInt(el.getAttribute('data-qorm-vtotal'),10)||0;
  var m=qormVListMetrics(el);
  var first=Math.floor(m.top/ih); if(!(first>=0)) first=0;
  var last=first+Math.ceil((m.port||0)/ih);
  if(last>total-1) last=total-1;
  if(first>=at && last<at+count){          // still covered: nothing to ask for
    delete window.__qormVLists[key];       // …and the in-flight guard is spent
    return false;
  }
  var val=Math.round(m.top)+','+Math.round(m.port), now=Date.now(), last1=window.__qormVLists[key];
  // Two floors on the round trips a fling can generate: never re-ask for a
  // position already asked for (which also stops a server whose window cannot
  // cover the view from being asked forever), and never more than one request
  // per 100ms per list however many frames the fling paints.
  if(last1 && (last1.val===val || now-last1.t<100)) return false;
  var inp=el.querySelector('input[data-qorm-vscroll]'); if(!inp) return false;
  window.__qormVLists[key]={val:val, t:now};
  inp.value=val;
  qorm(-1);                                 // the ordinary state-sync round trip
  return true;
}
function qormVListSync(){
  var live={};
  document.querySelectorAll('[data-qorm-vlist]').forEach(function(el){
    var key=el.getAttribute('data-qorm-vlist'); if(!key) return;
    live[key]=1;
    qormVListReport(el);
  });
  Object.keys(window.__qormVLists).forEach(function(k){ if(!live[k]) delete window.__qormVLists[k]; });
}
function qormVListInit(){
  if(window.__qormVListReady) return; window.__qormVListReady=true;
  document.addEventListener('scroll', qormVListScroll, true);
  window.addEventListener('resize', qormVListScroll);   // a taller port needs more rows
  qormVListSync();
}
function qormVListScroll(){
  if(window.__qormVListRAF) return;          // one measurement per animation frame
  var raf=window.requestAnimationFrame||function(f){ return setTimeout(f,16); };
  window.__qormVListRAF=raf(function(){ window.__qormVListRAF=0; qormVListSync(); });
}
// ---- input debounce (onChange + debounce: 300) --------------------------------
// A search box that dispatches on every keystroke floods the backend; `debounce`
// makes the control dispatch once, N ms after the human stops typing.
//
// IDEMPOTENCE: ONE delegated document `input` listener behind
// __qormDebounceReady (an inline oninput would re-arm per rendered control and a
// morph would strand the old timers). __qormDebounces is the only client state —
// the pending timer per element, keyed by the ELEMENT, because morphEl mutates a
// surviving node in place, so its identity lives exactly as long as the control
// does. qormDebounceSync runs from qormMorphInto and cancels the timers of
// controls that left the DOM, so a field removed mid-typing never dispatches.
// The interval AND the handler index are read from data attributes when the
// timer fires, so a re-render that renumbers the handler table cannot dispatch a
// stale action.
window.__qormDebounces = window.__qormDebounces || [];
function qormDebounced(el){ return el && el.getAttribute && el.getAttribute('data-qorm-debounce')!==null; }
function qormDebounceClear(el){
  var l=window.__qormDebounces;
  for(var i=l.length-1;i>=0;i--){ if(l[i].el===el){ clearTimeout(l[i].h); l.splice(i,1); } }
}
function qormDebouncePending(el){
  var l=window.__qormDebounces;
  for(var i=0;i<l.length;i++){ if(l[i].el===el) return true; }
  return false;
}
function qormDebounceFire(el){
  qormDebounceClear(el);
  if(!document.contains(el)) return;         // the field is gone; do not dispatch
  var h=parseInt(el.getAttribute('data-qorm-debounce-h'),10);
  if(isNaN(h)) h=-1;
  qorm(h);
}
function qormDebounceArm(el){
  var ms=parseInt(el.getAttribute('data-qorm-debounce'),10);
  qormDebounceClear(el);                     // restart on every keystroke — that is the debounce
  if(!(ms>0)){ qormDebounceFire(el); return; }
  window.__qormDebounces.push({el:el, h:setTimeout(function(){ qormDebounceFire(el); }, ms)});
}
function qormDebounceSync(){
  var l=window.__qormDebounces;
  for(var i=l.length-1;i>=0;i--){ if(!document.contains(l[i].el)){ clearTimeout(l[i].h); l.splice(i,1); } }
}
function qormDebounceInit(){
  if(window.__qormDebounceReady) return; window.__qormDebounceReady=true;
  document.addEventListener('input', function(e){ if(qormDebounced(e.target)) qormDebounceArm(e.target); });
  // Leaving the field flushes a pending timer: a human who types and immediately
  // tabs away (or presses the submit button) must not lose those keystrokes.
  document.addEventListener('focusout', function(e){
    if(qormDebounced(e.target) && qormDebouncePending(e.target)) qormDebounceFire(e.target);
  });
}
// ---- native validation: custom messages, first-invalid focus, Enter submit -----
// Three things the browser's own constraint validation does not do by itself,
// all delegated-listener-only (no per-control wiring, nothing a morph can
// duplicate) and all behind __qormValidityReady.
//
// 1. `requiredMessage` — the renderer emits data-qorm-error, and qormValidity
//    projects it with setCustomValidity ONLY while validity.valueMissing is
//    true. Doing it unconditionally would pin the field invalid forever (a
//    non-empty custom message IS a validity failure), so the message is
//    recomputed on every input and re-applied after every morph (which resets
//    attributes to the server's markup and, with them, the custom validity).
// 2. first-invalid focus — `invalid` does NOT bubble, so it is caught in the
//    capture phase; the first offender of a submit burst is scrolled to the
//    middle of its scroller and focused, which is the difference between a long
//    form that refuses to submit for no visible reason and one that shows you
//    which field it means.
// 3. Enter-to-submit — clicks the form's data-qorm-enter button (see enterAttr
//    in internal/render/render_input.go), re-read from the live DOM at keypress
//    time so the handler index in its onclick is the current frame's.
function qormValidity(el){
  if(!el || !el.setCustomValidity) return;
  var msg=el.getAttribute('data-qorm-error')||'';
  el.setCustomValidity('');                  // recompute the NATIVE verdict first
  if(msg && el.validity && el.validity.valueMissing) el.setCustomValidity(msg);
}
function qormValiditySync(){
  document.querySelectorAll('[data-qorm-error]').forEach(function(el){ qormValidity(el); });
}
function qormValidityInit(){
  if(window.__qormValidityReady) return; window.__qormValidityReady=true;
  document.addEventListener('input', function(e){
    var el=e.target;
    if(el && el.getAttribute && el.getAttribute('data-qorm-error')!==null) qormValidity(el);
  });
  document.addEventListener('invalid', function(e){
    var el=e.target; if(!el) return;
    if(window.__qormInvalidSeen) return;      // one burst, one field: the FIRST one
    window.__qormInvalidSeen=1;
    setTimeout(function(){ window.__qormInvalidSeen=0; }, 0);
    try{ el.scrollIntoView({behavior:'smooth', block:'center'}); }catch(_){}
    if(el.focus){ try{ el.focus({preventScroll:true}); }catch(_){ el.focus(); } }
  }, true);
  document.addEventListener('keydown', function(e){
    if(e.key!=='Enter' || e.shiftKey || e.ctrlKey || e.metaKey || e.altKey) return;
    var t=e.target; if(!t || !t.closest) return;
    if(t.tagName==='TEXTAREA') return;        // Enter is a newline there
    var form=t.form || t.closest('form'); if(!form) return;
    var btn=form.querySelector('[data-qorm-enter]'); if(!btn || btn.disabled) return;
    e.preventDefault();
    btn.click();                              // the button's OWN wiring: gate included
  });
  qormValiditySync();
}
function qormOverlayInit(){ qormLargeTitleInit(); qormSheetInit(); qormTabSwipeInit(); qormCarouselInit(); qormDebounceInit(); qormValidityInit(); qormVListInit(); qormTabReveal(); }
if(document.readyState!=='loading'){ qormTimersSync(); qormOverlayInit(); setTimeout(qormMeasure,60); setTimeout(qormHwInit,300); } else { window.addEventListener('load',function(){ qormTimersSync(); qormOverlayInit(); setTimeout(qormMeasure,60); setTimeout(qormHwInit,300); }); }
// qormSwipeActions: swipe a row left to reveal trailing action buttons; tap an
// action to fire it and close, tap the content or swipe back to close.
function qormSwipeActions(el){
  if(!el) return;
  var content=el.querySelector('.qorm-swa-content'), acts=el.querySelector('.qorm-swa-actions');
  if(!content||!acts) return;
  var x0=null, base=0, open=false;
  function w(){ return acts.offsetWidth||0; }
  function set(x, anim){ content.style.transition=anim?'transform .24s cubic-bezier(.32,.72,0,1)':'none'; content.style.transform='translateX('+x+'px)'; }
  content.addEventListener('pointerdown', function(e){ x0=e.clientX; base=open?-w():0; set(base,false); });
  content.addEventListener('pointermove', function(e){ if(x0===null) return; var dx=Math.max(-w()-24, Math.min(0, base+(e.clientX-x0))); set(dx,false); });
  function end(e){ if(x0===null) return; var dx=base+((e&&e.clientX||x0)-x0); open = dx < -w()/2; set(open?-w():0, true); x0=null; }
  content.addEventListener('pointerup', end);
  content.addEventListener('pointercancel', end);
  content.addEventListener('click', function(e){ if(open){ e.preventDefault(); e.stopPropagation(); open=false; set(0,true); } }, true);
  Array.prototype.forEach.call(acts.children, function(b){ b.addEventListener('click', function(){ open=false; setTimeout(function(){ set(0,true); }, 0); }); });
}
// Viewport push: report the window size to the server on load and on resize
// (debounced 200ms), so responsive `when` nodes ({{ viewport.width >= 768 }})
// render against the real client viewport. The server re-renders + broadcasts
// on change, so the matching branch swaps in live. Offline/WASM builds have no
// server — the fetch fails silently (the WASM runtime reads the size itself).
(function(){
  if(typeof window==='undefined'||typeof fetch==='undefined') return;
  var t=null, last='';
  function qormViewportSend(){
    var w=window.innerWidth||0, h=window.innerHeight||0, k=w+'x'+h;
    if(!w||!h||k===last) return;
    last=k;
    try{
      fetch('/viewport',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},
        body:JSON.stringify({w:w,h:h})}).catch(function(){});
    }catch(e){}
  }
  window.addEventListener('resize',function(){ if(t) clearTimeout(t); t=setTimeout(qormViewportSend,200); });
  if(document.readyState!=='loading'){ qormViewportSend(); }
  else { window.addEventListener('load',qormViewportSend); }
})();
// URL routing / deep-linking: keep the browser address bar and history in sync
// with the navigation stack, and honor Back/Forward. The server ships the
// current deep-link path with every update (X-Qorm-Route header on /event, a
// `route` field on the SSE/poll payload read by qormApply above). __qormApplyRoute
// pushes a new history entry when the path changes; a popstate reports the URL's
// scene+params back to the server via /navigate so it drives the runtime. Initial
// load needs nothing here — the server already renders the URL's scene. Offline
// packages have no server (fetch fails silently) and no SSE, so this is inert.
(function(){
  if(typeof window==='undefined'||typeof history==='undefined'||!history.pushState) return;
  window.__qormApplyRoute=function(route){
    if(typeof route!=='string'||!route) return;
    var cur=location.pathname+location.search;
    if(route===cur) return;                 // already there (e.g. after a popstate) — no dup entry
    try{ history.pushState(null,'',route); }catch(e){}
  };
  window.addEventListener('popstate',function(){
    var scene='', params={};
    if(location.search){
      var sp=new URLSearchParams(location.search);
      sp.forEach(function(v,k){ if(k==='scene'){ scene=v; } else { params[k]=v; } });
    }
    try{
      fetch('/navigate',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},
        body:JSON.stringify({scene:scene,params:params})}).catch(function(){});
    }catch(e){}
  });
})();

// ---- Scene-level key + swipe bindings (scene JSON `keys` / `keyReleases` / `swipes`)
// The canvas engine has built-in HandleKey / swipe recognition that consult
// rt.KeyAction / KeyReleaseAction / SwipeAction without a focused widget; the
// HTML path had no equivalent, so a `keys:{"left":"moveLeft"}` or
// `swipes:{"up":"jump"}` declaration was invisible in the browser. server.go
// Page() emits window.__qormKeys, __qormKeyReleases, and __qormSwipes (the
// declarative control scheme for the current scene) plus __qormKeyToIdx.
// Keys normalise DOM KeyboardEvent.key into the same lowercase names the
// runtime uses; swipes classify press→release travel with the same distance
// floor (24px) and axis dominance (1.3) as the canvas engine. Both dispatch
// by action name via /event {action} (live) or qormAction / qormKeyDown /
// qormSwipe (offline WASM) — outside the positional handler table.
(function(){
  // Match canvas swipeMinDist / swipeAxisDominance (internal/render/canvas).
  var SWIPE_MIN = 24;
  var SWIPE_AXIS = 1.3;
  function normKey(e){
    var k = e.key;
    if (k === ' ') return 'space';
    if (k === 'Escape') return 'escape';
    if (k === 'Tab') return 'tab';
    if (k === 'Enter') return 'return';
    if (k === 'Backspace') return 'backspace';
    if (k === 'Shift') return 'shift';
    if (k === 'Control') return 'control';
    if (k === 'Alt') return 'alt';
    if (k === 'Meta') return 'meta';
    if (k === 'ArrowLeft') return 'left';
    if (k === 'ArrowRight') return 'right';
    if (k === 'ArrowUp') return 'up';
    if (k === 'ArrowDown') return 'down';
    if (k && k.length === 1) return k.toLowerCase();
    return (k || '').toLowerCase();
  }
  function isTypingTarget(t){
    if (!t) return false;
    var tag = t.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true;
    if (t.isContentEditable) return true;
    return false;
  }
  // swipeDirection classifies press→release travel into left/right/up/down,
  // or "" when too short or diagonal (no dominant axis). Mirrors the canvas
  // engine's swipeDirection — keep SWIPE_MIN / SWIPE_AXIS in lockstep.
  function swipeDirection(dx, dy){
    var ax = Math.abs(dx), ay = Math.abs(dy);
    if (Math.max(ax, ay) < SWIPE_MIN) return '';
    if (ax > ay * SWIPE_AXIS) return dx > 0 ? 'right' : 'left';
    if (ay > ax * SWIPE_AXIS) return dy > 0 ? 'down' : 'up';
    return '';
  }
  function applyFrame(res){
    if (!res) return;
    if (res.theme) qormTheme(res.theme);
    if (res.dir && typeof qormDir === 'function') qormDir(res.dir);
    if (res.html != null) qormMorphInto(document.getElementById('qorm-root'), res.html);
    if (typeof qormMeasure !== 'undefined') setTimeout(qormMeasure, 30);
  }
  function dispatchAction(name){
    // Scene-level key/swipe bindings live outside the rendered handler table
    // (no element invokes them) — server.go /event accepts an `action` name
    // to dispatch by name. Offline WASM exposes qormAction(name) for the
    // same path (and qormKeyDown/qormSwipe for key/dir entry points).
    if (typeof qormAction === 'function') {
      try { applyFrame(qormAction(name)); } catch (err) {}
      return;
    }
    fetch('/event', {method:'POST', headers:{'Content-Type':'application/json', 'X-Qorm-Token': __tok},
      body: JSON.stringify({action: name, rev: __rev, inputs: {}})})
      .then(function(r){ var rv=parseInt(r.headers.get('X-Qorm-Rev'))||0; var nav=r.headers.get('X-Qorm-Nav')||''; qormTheme(r.headers.get('X-Qorm-Theme')); return r.text().then(function(html){ return {rv:rv,html:html,nav:nav}; }); })
      .then(function(o){ if(o.rv && o.rv<=__rev) return; if(o.rv) __rev=o.rv; window.__qormNav=o.nav; qormMorphInto(document.getElementById('qorm-root'), o.html); });
  }
  // Expose the pure classifier for unit-style checks (and keep the algorithm
  // in one place for both pointer and touch paths below).
  window.__qormSwipeDirection = swipeDirection;
  document.addEventListener('keydown', function(e){
    if (e.repeat) return;                          // ignore key-held autorepeat; the action's "key is held" semantic is its own concern
    if (isTypingTarget(e.target)) return;          // let focused inputs / text fields have their keys
    if (!window.__qormKeys) return;
    var k = normKey(e);
    // Offline WASM can resolve keys in-process without the action map.
    if (typeof qormKeyDown === 'function' && typeof qormAction !== 'function') {
      try {
        var r = qormKeyDown(k);
        if (r && r.html) { applyFrame(r); e.preventDefault(); return; }
      } catch (err) {}
    }
    var name = window.__qormKeys[k];
    if (!name) return;
    dispatchAction(name);
    e.preventDefault();
  });
  document.addEventListener('keyup', function(e){
    if (isTypingTarget(e.target)) return;
    if (!window.__qormKeyReleases) return;
    var name = window.__qormKeyReleases[normKey(e)];
    if (!name) return;
    dispatchAction(name);
    e.preventDefault();
  });
  // Scene swipe: press on non-interactive surface, drag past the distance
  // floor in one dominant direction, release → bound action. Skips targets
  // that own their own gesture (inputs, buttons, tabs, swipe-actions rows,
  // scroll chains) so we do not steal their pointer.
  if (window.__qormSceneSwipeReady) return;
  window.__qormSceneSwipeReady = true;
  var track = null; // {x0,y0,pid} while a candidate is armed
  function isSwipeBlocked(t){
    if (!t || !t.closest) return true;
    if (isTypingTarget(t)) return true;
    if (t.closest('button, a, input, textarea, select, option, [contenteditable="true"], [data-h], [data-qorm-tabs], .qorm-swa-content, .qorm-swa-actions, .qorm-slider, .qorm-sheet, .qorm-dsheet, .qorm-drawer, .qorm-menu-panel, .qorm-ctxmenu-panel')) return true;
    return false;
  }
  function fireSwipe(dir){
    if (!dir) return;
    if (typeof qormSwipe === 'function' && typeof qormAction !== 'function') {
      try {
        var r = qormSwipe(dir);
        if (r && r.html) { applyFrame(r); return; }
      } catch (err) {}
    }
    if (!window.__qormSwipes) return;
    var name = window.__qormSwipes[dir];
    if (!name) return;
    dispatchAction(name);
  }
  document.addEventListener('pointerdown', function(e){
    if (e.button != null && e.button !== 0) return;
    if (!window.__qormSwipes && typeof qormSwipe !== 'function') return;
    if (isSwipeBlocked(e.target)) return;
    track = {x0: e.clientX, y0: e.clientY, pid: e.pointerId};
  }, true);
  document.addEventListener('pointerup', function(e){
    if (!track || e.pointerId !== track.pid) return;
    var dx = e.clientX - track.x0, dy = e.clientY - track.y0;
    track = null;
    fireSwipe(swipeDirection(dx, dy));
  }, true);
  document.addEventListener('pointercancel', function(e){
    if (track && e.pointerId === track.pid) track = null;
  }, true);
})();
