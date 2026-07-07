@JavascriptInterface public void myBankSDK() {
    runOnUiThread(() -> js("qormOnBankSDK(\"paid $9.99 via native SDK (Android)\")"));
}
