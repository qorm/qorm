package integration

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

// TestHardwareWidgetsRender verifies every hardware widget renders its
// container, control, and bound-state sync — the framework-side wiring.
func TestHardwareWidgetsRender(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "hardware"))
	if err != nil {
		t.Fatal(err)
	}
	html := render.Render(qrt.New(app)).HTML
	cases := map[string][]string{
		"camera":     {`class="qorm-camera"`, `capture="environment"`, `qormCamera(this)`},
		"location":   {`class="qorm-location"`, `class="qorm-loc-out"`, `qormGeo(this)`},
		"sensors":    {`class="qorm-motion"`, `class="qorm-motion-out"`, `qormMotion(this)`},
		"recorder":   {`class="qorm-recorder"`, `class="qorm-rec-audio"`, `qormRec(this)`},
		"biometric":  {`class="qorm-biometric"`, `class="qorm-bio-out"`, `qormBio(this)`},
		"bluetooth":  {`class="qorm-bluetooth"`, `class="qorm-bluetooth-out"`, `qormBluetooth(this)`},
		"wifi":       {`class="qorm-wifi"`, `class="qorm-wifi-out"`, `qormWifi(this)`},
		"volume":     {`class="qorm-volume"`, `qormVol(this,1)`, `qormVol(this,-1)`},
		"brightness": {`class="qorm-brightness"`, `qormBright(this,1)`},
		"vibrate":    {`class="qorm-vibrate"`, `qormVibrate(this)`},
		"torch":      {`class="qorm-torch"`, `qormTorch(this)`},
		"battery":    {`class="qorm-battery"`, `qormBattery(this)`},
	}
	for name, markers := range cases {
		for _, m := range markers {
			if !strings.Contains(html, m) {
				t.Errorf("hardware widget %q should render %q", name, m)
			}
		}
	}
	// each hardware widget syncs to bound state via a hidden input
	if strings.Count(html, `type="hidden"`) < 4 {
		t.Errorf("hardware widgets should each sync a hidden state field")
	}
}

// TestHardwareBridgeJS verifies the page carries the native-bridge routing and
// every op's request + result callbacks, so the framework can drive native
// hardware (iOS WKScriptMessageHandler / Android JavascriptInterface) with a
// Web-API fallback.
func TestHardwareBridgeJS(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "hardware"))
	page := server.Page(qrt.New(app), render.Render(qrt.New(app)).HTML, 0)
	for _, fn := range []string{
		"qormHasNative", "qormToNative", // cross-platform bridge routing
		"window.webkit", "window.qormAndroid", // both native targets
		"qormGeo", "qormOnLocation", "qormOnLocationError", // GPS
		"qormMotion", "qormOnMotion", // motion
		"qormRec", "qormOnAudio", // audio
		"qormBio", "qormOnBiometric", // biometric
		"qormBluetooth", "qormOnBluetooth", // bluetooth
		"qormWifi", "qormOnWifi", // wifi
		"qormVol", "qormOnVolume", "qormBright", "qormOnBrightness", // volume + brightness
		"qormVibrate", "qormTorch", "qormOnTorch", "qormBattery", "qormOnBattery", // vibrate/torch/battery
		"qormCamera", // camera
	} {
		if !strings.Contains(page, fn) {
			t.Errorf("page should define bridge hook %q", fn)
		}
	}
}
