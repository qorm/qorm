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
      if(el.id==='qorm-root'||el.id==='qorm-stage') return;
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
    fetch('/measure',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(out)});
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
  setTimeout(qormMeasure, 30);
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
function morphEl(from, to){
  // sync attributes
  var changed=false;
  var ta=to.attributes, i, a;
  for(i=ta.length-1;i>=0;i--){ a=ta[i]; if(from.getAttribute(a.name)!==a.value){ from.setAttribute(a.name,a.value); changed=true; } }
  var fa=from.attributes;
  for(i=fa.length-1;i>=0;i--){ a=fa[i]; if(!to.hasAttribute(a.name)){ from.removeAttribute(a.name); changed=true; } }
  if(changed) qormFlash(from);
  var focused=(document.activeElement===from);
  // form controls: keep the user's live value/checked unless they're not focused
  if(from.nodeName==='INPUT'){
    if(!focused){ if(to.hasAttribute('checked')!==from.checked) from.checked=to.hasAttribute('checked');
      if(to.getAttribute('value')!=null && from.value!==to.getAttribute('value')) from.value=to.getAttribute('value'); }
    return;
  }
  if(from.nodeName==='TEXTAREA'){ if(!focused) from.value=to.textContent; return; }
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
  fetch('/event',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},
    body:JSON.stringify({h:h,inputs:inputs})})
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
    video.style.display='none'; btn.textContent='Retake';
    var h=box.getAttribute('data-h'); qorm(h?parseInt(h):-1);
    return;
  }
  navigator.mediaDevices.getUserMedia({video:{facingMode:'environment'}}).then(function(stream){
    box._stream=stream; video.srcObject=stream; video.style.display='block'; video.play(); btn.textContent='Capture';
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
  var box=btn.closest('.qorm-notify'), title=box.getAttribute('data-title')||'QORM', body=box.getAttribute('data-body')||'Hello from your QORM app ';
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
}
// Accordion: toggle the panel following the clicked header.
function qormAcc(btn){
  var p=btn.nextElementSibling;
  if(p){ p.style.display = (p.style.display==='none')?'block':'none'; }
}
// Menu: toggle the dropdown panel; close others.
function qormMenu(btn){
  var panel=btn.nextElementSibling;
  document.querySelectorAll('.qorm-menu-panel').forEach(function(p){ if(p!==panel) p.style.display='none'; });
  if(panel){ panel.style.display = (panel.style.display==='none')?'block':'none'; }
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
  fetch('/event',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},body:JSON.stringify({h:h,inputs:{_reorderFrom:from,_reorderTo:to}})})
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
// Draggable/DragTarget: pick up a qorm-draggable (carrying string payload `data`)
// and drop it on a qorm-droptarget, which fires its handler with {_dragData}.
var __qormDrag=null;
function qormDraggable(el,data){
  if(!el) return;
  el.setAttribute('draggable','true');
  el.addEventListener('dragstart',function(e){ __qormDrag=data;
    el.classList.add('qorm-dragging');
    try{ e.dataTransfer.setData('text/plain',data); e.dataTransfer.effectAllowed='move'; }catch(_){} });
  el.addEventListener('dragend',function(){ __qormDrag=null; el.classList.remove('qorm-dragging'); });
}
function qormDragTarget(el,h){
  if(!el) return;
  el.addEventListener('dragover',function(e){ e.preventDefault();
    el.classList.add('qorm-dragover'); try{ e.dataTransfer.dropEffect='move'; }catch(_){} });
  el.addEventListener('dragleave',function(){ el.classList.remove('qorm-dragover'); });
  el.addEventListener('drop',function(e){ e.preventDefault(); el.classList.remove('qorm-dragover');
    var data=__qormDrag; if(data===null){ try{ data=e.dataTransfer.getData('text/plain'); }catch(_){ data=''; } }
    qormPostDrop(h,data); });
}
function qormPostDrop(h,data){
  fetch('/event',{method:'POST',headers:{'Content-Type':'application/json','X-Qorm-Token':__tok},body:JSON.stringify({h:h,inputs:{_dragData:data}})})
    .then(function(r){ var rv=parseInt(r.headers.get('X-Qorm-Rev'))||0; qormTheme(r.headers.get('X-Qorm-Theme')); return r.text().then(function(html){ return {rv:rv,html:html}; }); })
    .then(function(o){ if(o.rv && o.rv<=__rev) return; if(o.rv) __rev=o.rv; qormMorphInto(document.getElementById('qorm-root'), o.html); });
}
// Live-sync: observe out-of-band changes (e.g. an AI agent editing the same
// session over /mcp) and swap in the new UI. Prefer Server-Sent Events for
// instant multi-client push; fall back to polling.
var __rev=__QORM_REV__;
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
  var es=new EventSource('/events');
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
if(document.readyState!=='loading'){ setTimeout(qormMeasure,60); setTimeout(qormHwInit,300); } else { window.addEventListener('load',function(){ setTimeout(qormMeasure,60); setTimeout(qormHwInit,300); }); }
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
