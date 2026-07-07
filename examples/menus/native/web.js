function qormShowMenuAction(kind, id){ var el=document.getElementById('menu-out'); if(el) el.textContent = kind+': '+id; }
if (typeof qormOn === 'function') {
  qormOn('menu', function(d){ qormShowMenuAction('menu', d && d.id); });
  qormOn('tray', function(d){ qormShowMenuAction('tray', d && d.id); });
  qormOn('context', function(d){ qormShowMenuAction('right-click', d && d.id); });
}
