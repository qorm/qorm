function qormBanner(msg){
  var b = document.getElementById('qorm-banner');
  if(!b){ b = document.createElement('div'); b.id='qorm-banner';
    b.style.cssText='position:fixed;top:0;left:0;right:0;z-index:99999;background:#0a84ff;color:#fff;padding:16px;font-size:16px;font-weight:600;text-align:center;'; document.body.appendChild(b); }
  b.textContent = msg; b.style.display='block';
  setTimeout(function(){ b.style.display='none'; }, 3500);
}
// App-icon quick actions -> real native actions.
if (typeof qormOn === 'function') {
  qormOn('shortcut', function(id){
    if (id === 'scan')  { qormBanner('Opening QR scanner…');  qormToNative('scanQR'); }
    else if (id === 'photo') { qormBanner('Opening photo picker…'); qormToNative('pickPhoto'); }
    else qormBanner('Quick action: ' + id);
  });
}
// Show the action's result in the banner too.
if (typeof qormOn === 'function') {
  var _s = window.qormOnScan;  window.qormOnScan  = function(v){ if(_s) _s(v); if(v) qormBanner('Scanned: ' + v); };
  var _p = window.qormOnPhoto; window.qormOnPhoto = function(u){ if(_p) _p(u); if(u) qormBanner('Photo selected'); };
}
// Live widget update (paid team + App Group only).
function qormPushWidget(){
  if (typeof qormUpdateWidget === 'function') {
    qormUpdateWidget('QORM Status', [{label:'Steps',value:'5,102'},{label:'Battery',value:'79%'},{label:'Synced',value:'live'}]);
  } else { setTimeout(qormPushWidget, 500); }
}
setTimeout(qormPushWidget, 1800);
