// Wire the app's own native op: clicking #payBtn calls the custom native op,
// and the native side calls back qormOnBankSDK — the SAME contract as built-ins.
document.addEventListener('click', function (e) {
 if (e.target.closest('#payBtn')) qormToNative('myBankSDK', { amount: 9.99 });
});
function qormOnBankSDK(msg) {
 var el = document.getElementById('result');
 if (el) el.textContent = ' ' + msg;
}
