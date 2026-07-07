//go:build ignore

// The app's OWN middle-layer, written in Go — this ONE file is compiled into the
// desktop binary AND the mobile/web WASM, so your custom logic runs everywhere.
// Register(op, fn): fn receives the qormToNative payload and returns a line of JS
// to run back in the app (usually a qormOn<X> callback).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/qorm/qorm/pkg/qormext"
)

// State that lives in Go and persists across calls within the session.
var visits int

func jsStr(s string) string { // minimal JSON-string quoting for the callbacks
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			out = append(out, '\\')
		}
		out = append(out, c)
	}
	return string(append(out, '"'))
}

func init() {
	// (1) Real Go stdlib logic — SHA-256 — that the declarative JSON can't do.
	qormext.Register("hash", func(d map[string]any) string {
		text, _ := d["text"].(string)
		sum := sha256.Sum256([]byte(text))
		return "qormOnHash(" + jsStr(hex.EncodeToString(sum[:])) + ")"
	})

	// (2) Stateful backend: a counter kept in Go memory.
	qormext.Register("visit", func(d map[string]any) string {
		visits++
		return fmt.Sprintf("qormOnVisits(%d)", visits)
	})

	// (3) Reach the framework's hardware bridge from Go, then push an event onto
	// the UI event bus — the native→UI channel, driven from your own Go code.
	qormext.Register("celebrate", func(d map[string]any) string {
		qormext.Native("vibrate", `{"ms":200}`)     // Go → framework hardware
		qormext.Emit("celebrated", `{"from":"Go"}`) // Go → UI event bus
		return `qormOnCelebrate("buzzed the device + emitted an event, all from Go")`
	})
}
