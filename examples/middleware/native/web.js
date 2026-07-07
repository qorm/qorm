// Callbacks the Go middle-layer calls back into (same contract as built-ins).
function qormOnHash(hex){ var el=document.getElementById('hash-out'); if(el) el.textContent = hex; }
function qormOnVisits(n){ var el=document.getElementById('visit-out'); if(el) el.textContent = 'visits: ' + n; }
function qormOnCelebrate(msg){ var el=document.getElementById('celeb-out'); if(el) el.textContent = msg; }

// Listen for the event the Go layer emits onto the bus.
if (typeof qormOn === 'function') {
  qormOn('celebrated', function(d){
    var el=document.getElementById('event-out'); if(el) el.textContent = 'event received from Go: ' + (d ? JSON.stringify(d) : '');
  });
}

// Wire the buttons to the app's custom native ops.
document.addEventListener('click', function(e){
  if (e.target.closest('#hashBtn')) {
    var inp = document.querySelector('#txt input, #txt') ;
    var val = (inp && inp.value != null) ? inp.value : 'hello qorm';
    qormToNative('hash', { text: val });
  }
  if (e.target.closest('#visitBtn'))  qormToNative('visit');
  if (e.target.closest('#celebBtn'))  qormToNative('celebrate');
});
