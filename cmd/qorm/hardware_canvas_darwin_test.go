//go:build darwin && !desktop

package main

import (
	"strings"
	"testing"
)

// Smoke tests for the pure-Go canvas hardware bridge: real exec calls on the
// host Mac (each op must answer with the JS-bridge callback contract).

func TestCanvasBridgeNetworkStatus(t *testing.T) {
	var cbName string
	var cbArg string
	canvasHardwareDarwin("networkStatus", nil, func(js string) {
		cbName = js
	})
	if !strings.HasPrefix(cbName, `qormOnNetwork("`) || !strings.Contains(cbName, `online`) {
		t.Errorf("networkStatus callback = %q", cbName)
	}
	_ = cbArg
}

func TestCanvasBridgeDeviceInfo(t *testing.T) {
	var got string
	canvasHardwareDarwin("deviceInfo", nil, func(js string) { got = js })
	if !strings.HasPrefix(got, `qormOnDeviceInfo("`) || !strings.Contains(got, "macOS") {
		t.Errorf("deviceInfo callback = %q", got)
	}
}

func TestCanvasBridgeBattery(t *testing.T) {
	var got string
	canvasHardwareDarwin("battery", nil, func(js string) { got = js })
	if !strings.HasPrefix(got, `qormOnBattery("`) || !strings.Contains(got, `state`) {
		t.Errorf("battery callback = %q", got)
	}
}

func TestCanvasBridgeClipboardRoundTrip(t *testing.T) {
	canvasHardwareDarwin("clipboardSet", map[string]interface{}{"text": "qorm-bridge-test"}, func(string) {})
	var got string
	canvasHardwareDarwin("clipboardGet", nil, func(js string) { got = js })
	if !strings.Contains(got, "qorm-bridge-test") {
		t.Errorf("clipboard round trip = %q", got)
	}
}
