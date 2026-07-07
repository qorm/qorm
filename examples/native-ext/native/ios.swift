func qormUserOp(_ op: String, _ body: [String: Any]) {
    switch op {
    case "myBankSDK":
        // your real native SDK call goes here; body["amount"] etc. available
        js("qormOnBankSDK(\"paid $9.99 via native SDK (iOS)\")")
    default:
        break
    }
}
