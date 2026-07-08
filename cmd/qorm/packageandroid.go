package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/model"
)

// scaffoldAndroid generates a complete WebView Android project around the
// offline web payload (out/www) and builds an APK if the toolchain is present.
// The payload is served to the WebView over https via WebViewAssetLoader so the
// WASM runtime's fetch() works from packaged assets.
func scaffoldAndroid(out, name, appName, dev, appDir string, rel releaseOpts) error {
	_ = rel // release signing/AAB lands with v0.2.1 A2
	id := pkgID(name)
	ns := "com.qorm." + id
	awidgets := appWidgets(appDir)
	hasWidget := len(awidgets) > 0 && dev == ""
	wName := appName
	if hasWidget && awidgets[0].Name != "" {
		wName = awidgets[0].Name
	}
	nsPath := filepath.Join(out, "app", "src", "main", "java", "com", "qorm", id)
	assets := filepath.Join(out, "app", "src", "main", "assets", "www")
	res := filepath.Join(out, "app", "src", "main", "res")
	for _, d := range []string{nsPath, assets, filepath.Join(res, "mipmap-mdpi"), filepath.Join(res, "values"), filepath.Join(res, "layout"), filepath.Join(res, "xml")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// offline mode bundles the web payload; dev mode connects to a server.
	if dev == "" {
		if err := copyTree(filepath.Join(out, "www"), assets); err != nil {
			return err
		}
	}
	writeIconFor(appDir, filepath.Join(res, "mipmap-mdpi", "ic_launcher.png"), 192)

	files := map[string]string{
		"settings.gradle": `pluginManagement {
    repositories { google(); mavenCentral(); gradlePluginPortal() }
}
dependencyResolutionManagement {
    repositories { google(); mavenCentral() }
}
rootProject.name = "` + id + `"
include(":app")
`,
		"build.gradle": `plugins {
    id 'com.android.application' version '8.7.2' apply false
}
`,
		"gradle.properties": "android.useAndroidX=true\norg.gradle.jvmargs=-Xmx2048m\n",
		"app/build.gradle": `plugins { id 'com.android.application' }
android {
    namespace '` + ns + `'
    compileSdk 34
    defaultConfig {
        applicationId "` + ns + `"
        minSdk 24
        targetSdk 34
        versionCode 1
        versionName "1.0"
    }
    buildTypes { release { minifyEnabled false } }
    compileOptions {
        sourceCompatibility JavaVersion.VERSION_17
        targetCompatibility JavaVersion.VERSION_17
    }
}
dependencies {
    implementation 'androidx.webkit:webkit:1.12.1'
    implementation 'androidx.biometric:biometric:1.1.0'
    implementation 'androidx.security:security-crypto:1.1.0-alpha06'
}
`,
		"app/src/main/AndroidManifest.xml": `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">` + androidPerms(dev, appDir) + `
    <application
        android:label="` + xmlEsc(appName) + `"
        android:icon="@mipmap/ic_launcher"
        android:hardwareAccelerated="true"` + androidCleartext(dev) + `
        android:theme="@android:style/Theme.Material.Light.NoActionBar">
        <activity android:name=".MainActivity" android:exported="true"
            android:configChanges="orientation|screenSize|keyboardHidden">
            <intent-filter>
                <action android:name="android.intent.action.MAIN"/>
                <category android:name="android.intent.category.LAUNCHER"/>
            </intent-filter>
        </activity>
` + androidWidgetReceiver(hasWidget) + `    </application>
</manifest>
`,
		"app/src/main/java/com/qorm/" + id + "/MainActivity.java": spliceUser(androidMainActivity(ns, dev, androidScFor(dev, appDir)), "//__QORM_USER_ANDROID__", appDir, "android.java", ""),
	}
	// always ship the provider + resources so the updateWidget op compiles; the
	// manifest <receiver> (added only when the app declares a widget) surfaces it.
	files["app/src/main/java/com/qorm/"+id+"/QormWidget.java"] = androidWidgetProvider(ns, awProvider(awidgets, wName))
	files["app/src/main/res/layout/qorm_widget.xml"] = androidWidgetLayout()
	files["app/src/main/res/xml/qorm_widget_info.xml"] = androidWidgetInfo()
	for rel, content := range files {
		p := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}

	fmt.Printf("generated Android project -> %s\n", out)
	return buildAndroid(out)
}

// buildAndroid runs a Gradle debug build if a toolchain is available, else
// prints how to finish the build.
func buildAndroid(dir string) error {
	gradle, err := exec.LookPath("gradle")
	sdk := os.Getenv("ANDROID_HOME")
	if sdk == "" {
		sdk = os.Getenv("ANDROID_SDK_ROOT")
	}
	if err != nil || sdk == "" {
		fmt.Printf("  Android SDK/Gradle not both present — project is ready to build:\n    cd %s && gradle assembleDebug\n", dir)
		return nil
	}
	// Pin a Gradle wrapper to a version the Android plugin supports (a newer
	// system Gradle can be incompatible with AGP), then build via the wrapper.
	fmt.Fprintf(os.Stderr, "preparing Gradle wrapper (8.9)…\n")
	wrap := exec.Command(gradle, "wrapper", "--gradle-version", "8.9", "--no-daemon", "--console=plain")
	wrap.Dir = dir
	wrap.Stdout, wrap.Stderr = os.Stderr, os.Stderr
	builder := "./gradlew"
	if err := wrap.Run(); err != nil {
		builder = gradle // fall back to system gradle
	}
	fmt.Fprintf(os.Stderr, "building APK (first run downloads Gradle + the Android plugin)…\n")
	cmd := exec.Command(builder, "assembleDebug", "--no-daemon", "--console=plain")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("  Gradle build did not complete. Project is ready:\n    cd %s && ./gradlew assembleDebug\n", dir)
		return nil
	}
	apk := filepath.Join(dir, "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	if _, e := os.Stat(apk); e == nil {
		fmt.Printf("  [ok] APK: %s\n", apk)
	}
	return nil
}

// androidWidgetReceiver returns the manifest <receiver> for the home-screen
// widget, or "" when the app declares none.
func androidWidgetReceiver(hasWidget bool) string {
	if !hasWidget {
		return ""
	}
	return `        <receiver android:name=".QormWidget" android:exported="false">
            <intent-filter>
                <action android:name="android.appwidget.action.APPWIDGET_UPDATE"/>
            </intent-filter>
            <meta-data android:name="android.appwidget.provider" android:resource="@xml/qorm_widget_info"/>
        </receiver>
`
}

// androidWidgetProvider generates the AppWidgetProvider that renders a title +
// label/value lines from SharedPreferences the app keeps updated.
func androidWidgetProvider(ns string, w model.Widget) string {
	title := w.Title
	if title == "" {
		title = w.Name
	}
	baked := "[]"
	if len(w.Lines) > 0 {
		var b strings.Builder
		b.WriteString("[")
		for i, ln := range w.Lines {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{\"label\":\"` + ln.Label + `\",\"value\":\"` + ln.Value + `\"}`)
		}
		b.WriteString("]")
		baked = b.String()
	}
	return `package ` + ns + `;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.Context;
import android.widget.RemoteViews;
public class QormWidget extends AppWidgetProvider {
    @Override public void onUpdate(Context ctx, AppWidgetManager mgr, int[] ids) {
        android.content.SharedPreferences p = ctx.getSharedPreferences("qorm_widget", Context.MODE_PRIVATE);
        String title = p.getString("widget_title", "` + title + `");
        StringBuilder sb = new StringBuilder();
        try {
            org.json.JSONArray arr = new org.json.JSONArray(p.getString("widget_lines", "` + baked + `"));
            for (int i = 0; i < arr.length(); i++) {
                org.json.JSONObject o = arr.getJSONObject(i);
                if (i > 0) sb.append("\n");
                sb.append(o.optString("label")).append(": ").append(o.optString("value"));
            }
        } catch (Exception e) {}
        for (int id : ids) {
            RemoteViews rv = new RemoteViews(ctx.getPackageName(), R.layout.qorm_widget);
            rv.setTextViewText(R.id.qorm_widget_title, title);
            rv.setTextViewText(R.id.qorm_widget_body, sb.toString());
            mgr.updateAppWidget(id, rv);
        }
    }
}
`
}

// androidWidgetLayout is the RemoteViews layout for the widget.
func androidWidgetLayout() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<LinearLayout xmlns:android="http://schemas.android.com/apk/res/android"
    android:layout_width="match_parent" android:layout_height="match_parent"
    android:orientation="vertical" android:padding="12dp" android:background="#FFFFFF">
    <TextView android:id="@+id/qorm_widget_title" android:layout_width="wrap_content"
        android:layout_height="wrap_content" android:textStyle="bold" android:textSize="16sp" android:textColor="#000000"/>
    <TextView android:id="@+id/qorm_widget_body" android:layout_width="wrap_content"
        android:layout_height="wrap_content" android:textSize="13sp" android:textColor="#333333" android:layout_marginTop="6dp"/>
</LinearLayout>
`
}

// androidWidgetInfo is the appwidget-provider metadata.
func androidWidgetInfo() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<appwidget-provider xmlns:android="http://schemas.android.com/apk/res/android"
    android:minWidth="110dp" android:minHeight="40dp" android:updatePeriodMillis="0"
    android:initialLayout="@layout/qorm_widget" android:resizeMode="horizontal|vertical"
    android:widgetCategory="home_screen"/>
`
}

// androidPerms adds INTERNET permission for the dev client (it reaches a server).
func androidPerms(dev, appDir string) string {
	var perms []string
	if dev == "" {
		// a packaged app declares only what its capabilities use
		perms = append([]string{"android.permission.INTERNET"}, capability.PermsFor(usedWidgets(appDir), capability.Android)...)
	} else {
		perms = []string{
			"android.permission.INTERNET",
			"android.permission.CAMERA",
			"android.permission.READ_CONTACTS",
			"android.permission.RECORD_AUDIO",
			"android.permission.ACCESS_FINE_LOCATION",
			"android.permission.ACCESS_COARSE_LOCATION",
			"android.permission.READ_MEDIA_IMAGES",
			"android.permission.READ_MEDIA_VIDEO",
			"android.permission.READ_MEDIA_AUDIO",
			"android.permission.READ_EXTERNAL_STORAGE",
			"android.permission.BLUETOOTH_CONNECT",
			"android.permission.BLUETOOTH_SCAN",
			"android.permission.NFC",
			"android.permission.VIBRATE",
			"android.permission.POST_NOTIFICATIONS",
		}
	}
	var b strings.Builder
	for _, p := range perms {
		b.WriteString("\n    <uses-permission android:name=\"" + p + "\"/>")
	}
	// camera/mic hardware not required (so it installs on devices without them)
	b.WriteString("\n    <uses-feature android:name=\"android.hardware.camera\" android:required=\"false\"/>")
	return b.String()
}

// androidCleartext allows plain-HTTP to the dev server on the LAN.
func androidCleartext(dev string) string {
	if dev == "" {
		return ""
	}
	return "\n        android:usesCleartextTraffic=\"true\""
}

// androidMainActivity returns the Activity: a thin dev client that loads the
// live server URL, or the offline asset-loader client.
// androidScFor returns an app's shortcuts, or none for the dev client.
func androidScFor(dev, appDir string) []model.Shortcut {
	if dev != "" {
		return nil
	}
	return appShortcuts(appDir)
}

// androidShortcutRegister renders the onCreate Java that registers the app's
// icon quick actions with ShortcutManager (each launches MainActivity carrying a
// qorm_shortcut extra).
func androidShortcutRegister(scs []model.Shortcut) string {
	if len(scs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("        if (android.os.Build.VERSION.SDK_INT >= 25) {\n")
	b.WriteString("            android.content.pm.ShortcutManager scMgr = getSystemService(android.content.pm.ShortcutManager.class);\n")
	b.WriteString("            if (scMgr != null) {\n")
	b.WriteString("                java.util.List<android.content.pm.ShortcutInfo> qscs = new java.util.ArrayList<>();\n")
	for _, sc := range scs {
		b.WriteString("                qscs.add(new android.content.pm.ShortcutInfo.Builder(this, " + strconv.Quote(sc.ID) + ").setShortLabel(" + strconv.Quote(sc.Title) + ")")
		if sc.Subtitle != "" {
			b.WriteString(".setLongLabel(" + strconv.Quote(sc.Subtitle) + ")")
		}
		b.WriteString(".setIntent(new android.content.Intent(android.content.Intent.ACTION_VIEW, null, this, MainActivity.class).putExtra(\"qorm_shortcut\", " + strconv.Quote(sc.ID) + ")).build());\n")
	}
	b.WriteString("                try { scMgr.setDynamicShortcuts(qscs); } catch (Exception e) {}\n")
	b.WriteString("            }\n")
	b.WriteString("        }\n")
	b.WriteString("        String qsc0 = getIntent().getStringExtra(\"qorm_shortcut\");\n")
	b.WriteString("        if (qsc0 != null) qormPendingShortcut = qsc0;\n")
	return b.String()
}

func androidMainActivity(ns, dev string, scs []model.Shortcut) string {
	// Both the dev client and the offline (WASM) package embed the SAME native
	// hardware Bridge (window.qormAndroid @JavascriptInterface) so qormToNative
	// reaches native hardware on either. They differ only in how the WebView is
	// fed content: the dev client loads the live server URL over HTTP(S); the
	// offline build serves the bundled WASM/assets over an https virtual host via
	// WebViewAssetLoader so fetch()/WebAssembly.instantiate work from app assets.
	clientSetup := `        wv.setWebViewClient(new android.webkit.WebViewClient() {
            @Override public void onReceivedSslError(WebView view, android.webkit.SslErrorHandler handler, android.net.http.SslError error) {
                handler.proceed();
            }
        });
        wv.addJavascriptInterface(new Bridge(), "qormAndroid");
        setContentView(wv);
        wv.loadUrl("` + dev + `");`
	if dev == "" {
		clientSetup = `        final androidx.webkit.WebViewAssetLoader loader = new androidx.webkit.WebViewAssetLoader.Builder()
            .addPathHandler("/assets/", new androidx.webkit.WebViewAssetLoader.AssetsPathHandler(this))
            .build();
        wv.setWebViewClient(new android.webkit.WebViewClient() {
            @Override public android.webkit.WebResourceResponse shouldInterceptRequest(WebView view, android.webkit.WebResourceRequest request) {
                return loader.shouldInterceptRequest(request.getUrl());
            }
        });
        wv.addJavascriptInterface(new Bridge(), "qormAndroid");
        setContentView(wv);
        wv.loadUrl("https://appassets.androidplatform.net/assets/www/index.html");`
	}
	return `package ` + ns + `;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebView;
import android.webkit.JavascriptInterface;
import android.hardware.Sensor;
import android.hardware.SensorEvent;
import android.hardware.SensorEventListener;
import android.hardware.SensorManager;
import android.location.Location;
import android.location.LocationListener;
import android.location.LocationManager;
import android.content.Context;
import android.media.MediaRecorder;
import androidx.biometric.BiometricPrompt;
import androidx.core.content.ContextCompat;
import java.util.concurrent.Executor;
import android.util.Base64;
import android.bluetooth.BluetoothManager;
import android.bluetooth.BluetoothAdapter;
import android.bluetooth.le.BluetoothLeScanner;
import android.bluetooth.le.ScanCallback;
import android.bluetooth.le.ScanResult;
import android.net.wifi.WifiManager;
import android.net.wifi.WifiInfo;
import android.nfc.NfcAdapter;
import android.nfc.Tag;
import android.media.AudioManager;
import android.os.Vibrator;
import android.os.VibrationEffect;
import android.os.BatteryManager;
import android.hardware.camera2.CameraManager;
import android.content.IntentFilter;
import android.content.Intent;
import android.view.WindowManager;

// QORM client + native hardware bridge (dev AND offline): window.qormAndroid.<op>()
// calls native SensorManager/LocationManager etc.; results are pushed back with
// evaluateJavascript (qormOnLocation / qormOnMotion). Full native access.
public class MainActivity extends androidx.fragment.app.FragmentActivity implements SensorEventListener, LocationListener {
    WebView wv;
    android.speech.tts.TextToSpeech qormTts;
    android.speech.SpeechRecognizer qormSpeechRec;
    android.hardware.SensorEventListener qormCompassL, qormProxL, qormStepL, qormPressL;
    androidx.activity.result.ActivityResultLauncher<String> qormPhotoPick;
    androidx.activity.result.ActivityResultLauncher<String> qormFilePick;
    androidx.activity.result.ActivityResultLauncher<Void> qormContactPick;
    String qormPendingShortcut;
    SensorManager sm;
    Sensor rot;
    LocationManager lm;
    MediaRecorder mrec;
    String recPath;

    @Override protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        qormContactPick = registerForActivityResult(new androidx.activity.result.contract.ActivityResultContracts.PickContact(), uri -> {
            if (uri == null) { js("qormOnContact('{}')"); return; }
            try {
                android.database.Cursor c = getContentResolver().query(uri, null, null, null, null);
                String name = "";
                if (c != null && c.moveToFirst()) {
                    int idx = c.getColumnIndex(android.provider.ContactsContract.Contacts.DISPLAY_NAME);
                    if (idx >= 0) name = c.getString(idx);
                }
                if (c != null) c.close();
                js("qormOnContact('{\"name\":\"" + name + "\",\"phone\":\"\"}')");
            } catch (Exception e) { js("qormOnContact('{}')"); }
        });
        qormFilePick = registerForActivityResult(new androidx.activity.result.contract.ActivityResultContracts.GetContent(), uri -> {
            if (uri == null) { js("qormOnFile('{}')"); return; }
            try {
                java.io.InputStream ins = getContentResolver().openInputStream(uri);
                java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
                byte[] buf = new byte[8192]; int n; while ((n = ins.read(buf)) > 0) baos.write(buf, 0, n);
                String name = uri.getLastPathSegment();
                js("qormOnFile('{\"name\":\"" + name + "\",\"size\":" + baos.size() + ",\"dataURL\":\"data:application/octet-stream;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP) + "\"}')");
            } catch (Exception e) { js("qormOnFile('{}')"); }
        });
        qormPhotoPick = registerForActivityResult(new androidx.activity.result.contract.ActivityResultContracts.GetContent(), uri -> {
            if (uri == null) { js("qormOnPhoto('')"); return; }
            try {
                java.io.InputStream ins = getContentResolver().openInputStream(uri);
                android.graphics.Bitmap bmp = android.graphics.BitmapFactory.decodeStream(ins);
                java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
                bmp.compress(android.graphics.Bitmap.CompressFormat.JPEG, 80, baos);
                js("qormOnPhoto('data:image/jpeg;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP) + "')");
            } catch (Exception e) { js("qormOnPhoto('')"); }
        });
        try {
            requestPermissions(new String[]{
                "android.permission.CAMERA", "android.permission.RECORD_AUDIO",
                "android.permission.ACCESS_FINE_LOCATION", "android.permission.ACCESS_COARSE_LOCATION"}, 1);
        } catch (Exception e) {}
        wv = new WebView(this);
        wv.getSettings().setJavaScriptEnabled(true);
        wv.getSettings().setDomStorageEnabled(true);
` + clientSetup + `
        sm = (SensorManager) getSystemService(Context.SENSOR_SERVICE);
        rot = sm.getDefaultSensor(Sensor.TYPE_ROTATION_VECTOR);
        lm = (LocationManager) getSystemService(Context.LOCATION_SERVICE);
` + androidShortcutRegister(scs) + `    }

    class Bridge {
        @JavascriptInterface public void location(String a) { runOnUiThread(() -> getLoc()); }
        @JavascriptInterface public void motionStart(String a) { runOnUiThread(() -> sm.registerListener(MainActivity.this, rot, SensorManager.SENSOR_DELAY_UI)); }
        @JavascriptInterface public void motionStop(String a) { runOnUiThread(() -> sm.unregisterListener(MainActivity.this)); }
        @JavascriptInterface public void recordStart(String a) { runOnUiThread(() -> startRec()); }
        @JavascriptInterface public void recordStop(String a) { runOnUiThread(() -> stopRec()); }
        @JavascriptInterface public void biometric(String a) { runOnUiThread(() -> doBio()); }
        @JavascriptInterface public void bluetoothScan(String a) { runOnUiThread(() -> btScan()); }
        @JavascriptInterface public void wifiInfo(String a) { runOnUiThread(() -> wifiImpl()); }
        @JavascriptInterface public void nfcRead(String a) { runOnUiThread(() -> nfcEnable()); }
        @JavascriptInterface public void screenshot(String a) {
            runOnUiThread(() -> {
                try {
                    android.graphics.Bitmap bmp = android.graphics.Bitmap.createBitmap(Math.max(1, wv.getWidth()), Math.max(1, wv.getHeight()), android.graphics.Bitmap.Config.ARGB_8888);
                    wv.draw(new android.graphics.Canvas(bmp));
                    java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
                    bmp.compress(android.graphics.Bitmap.CompressFormat.JPEG, 80, baos);
                    js("qormOnScreenshot('data:image/jpeg;base64," + Base64.encodeToString(baos.toByteArray(), Base64.NO_WRAP) + "')");
                } catch (Exception e) { js("qormOnScreenshot('')"); }
            });
        }
        @JavascriptInterface public void screenRecordStart(String a) { runOnUiThread(() -> js("qormOnScreenRecord('Android screen recording needs MediaProjection (not yet wired)')")); }
        @JavascriptInterface public void screenRecordStop(String a) { runOnUiThread(() -> js("qormOnScreenRecord('')")); }
        @JavascriptInterface public void share(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                String text = o.optString("text",""), url = o.optString("url","");
                android.content.Intent i = new android.content.Intent(android.content.Intent.ACTION_SEND);
                i.setType("text/plain");
                i.putExtra(android.content.Intent.EXTRA_TEXT, text + (url.isEmpty()?"":" "+url));
                startActivity(android.content.Intent.createChooser(i, "Share"));
                js("qormOnShare(true)");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void clipboardSet(String a) {
            runOnUiThread(() -> { try {
                String text = new org.json.JSONObject(a).optString("text","");
                android.content.ClipboardManager cb = (android.content.ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
                cb.setPrimaryClip(android.content.ClipData.newPlainText("qorm", text));
                js("qormOnClipboard(" + jsStr(text) + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void clipboardGet(String a) {
            runOnUiThread(() -> {
                android.content.ClipboardManager cb = (android.content.ClipboardManager) getSystemService(Context.CLIPBOARD_SERVICE);
                String text = "";
                if (cb.hasPrimaryClip() && cb.getPrimaryClip().getItemCount() > 0) text = String.valueOf(cb.getPrimaryClip().getItemAt(0).getText());
                js("qormOnClipboard(" + jsStr(text) + ")");
            });
        }
        @JavascriptInterface public void deviceInfo(String a) {
            runOnUiThread(() -> {
                String info = "{\"model\":\"" + android.os.Build.MODEL + "\",\"name\":\"" + android.os.Build.MANUFACTURER + "\",\"os\":\"Android " + android.os.Build.VERSION.RELEASE + "\",\"screen\":\"" + getResources().getDisplayMetrics().widthPixels + "x" + getResources().getDisplayMetrics().heightPixels + "\"}";
                js("qormOnDeviceInfo(" + jsStr(info) + ")");
            });
        }
        @JavascriptInterface public void networkStatus(String a) {
            runOnUiThread(() -> {
                android.net.ConnectivityManager cm = (android.net.ConnectivityManager) getSystemService(Context.CONNECTIVITY_SERVICE);
                android.net.NetworkCapabilities nc = cm.getNetworkCapabilities(cm.getActiveNetwork());
                boolean online = nc != null;
                String type = nc == null ? "none" : (nc.hasTransport(android.net.NetworkCapabilities.TRANSPORT_WIFI) ? "wifi" : nc.hasTransport(android.net.NetworkCapabilities.TRANSPORT_CELLULAR) ? "cellular" : "other");
                js("qormOnNetwork(" + jsStr("{\"online\":" + online + ",\"type\":\"" + type + "\"}") + ")");
            });
        }
        @JavascriptInterface public void keepAwake(String a) {
            runOnUiThread(() -> { try {
                boolean on = new org.json.JSONObject(a).optBoolean("on", false);
                if (on) getWindow().addFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
                else getWindow().clearFlags(android.view.WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void haptic(String a) {
            runOnUiThread(() -> { try {
                String type = new org.json.JSONObject(a).optString("type", "success");
                android.os.Vibrator v = (android.os.Vibrator) getSystemService(Context.VIBRATOR_SERVICE);
                long ms = type.equals("error") ? 100 : type.equals("heavy") ? 80 : type.equals("light") ? 15 : 40;
                if (android.os.Build.VERSION.SDK_INT >= 26) v.vibrate(android.os.VibrationEffect.createOneShot(ms, android.os.VibrationEffect.DEFAULT_AMPLITUDE));
                else v.vibrate(ms);
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void storageSet(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                getSharedPreferences("qorm", Context.MODE_PRIVATE).edit().putString(o.optString("key",""), o.optString("value","")).apply();
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void storageGet(String a) {
            runOnUiThread(() -> { try {
                String key = new org.json.JSONObject(a).optString("key","");
                String val = getSharedPreferences("qorm", Context.MODE_PRIVATE).getString(key, "");
                js("qormOnStorage(" + jsStr(key) + ", " + jsStr(val) + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void listenStart(String a) {
            runOnUiThread(() -> {
                if (qormSpeechRec == null) qormSpeechRec = android.speech.SpeechRecognizer.createSpeechRecognizer(MainActivity.this);
                qormSpeechRec.setRecognitionListener(new android.speech.RecognitionListener() {
                    public void onResults(android.os.Bundle b) {
                        java.util.ArrayList<String> r = b.getStringArrayList(android.speech.SpeechRecognizer.RESULTS_RECOGNITION);
                        if (r != null && !r.isEmpty()) js("qormOnSpeech(" + jsStr(r.get(0)) + ")");
                    }
                    public void onReadyForSpeech(android.os.Bundle b) {}
                    public void onBeginningOfSpeech() {}
                    public void onRmsChanged(float v) {}
                    public void onBufferReceived(byte[] b) {}
                    public void onEndOfSpeech() {}
                    public void onError(int e) {}
                    public void onPartialResults(android.os.Bundle b) {}
                    public void onEvent(int e, android.os.Bundle b) {}
                });
                android.content.Intent i = new android.content.Intent(android.speech.RecognizerIntent.ACTION_RECOGNIZE_SPEECH);
                i.putExtra(android.speech.RecognizerIntent.EXTRA_LANGUAGE_MODEL, android.speech.RecognizerIntent.LANGUAGE_MODEL_FREE_FORM);
                try { String lang = new org.json.JSONObject(a).optString("lang", ""); if (!lang.isEmpty()) i.putExtra(android.speech.RecognizerIntent.EXTRA_LANGUAGE, lang); } catch (Exception e) {}
                qormSpeechRec.startListening(i);
            });
        }
        @JavascriptInterface public void listenStop(String a) { runOnUiThread(() -> { if (qormSpeechRec != null) qormSpeechRec.stopListening(); }); }
        @JavascriptInterface public void lockOrientation(String a) {
            runOnUiThread(() -> { try {
                String mode = new org.json.JSONObject(a).optString("mode", "portrait");
                setRequestedOrientation(mode.equals("landscape") ? android.content.pm.ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE : android.content.pm.ActivityInfo.SCREEN_ORIENTATION_PORTRAIT);
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void speak(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                String text = o.optString("text", "");
                String lang = o.optString("lang", "");
                if (qormTts == null) qormTts = new android.speech.tts.TextToSpeech(MainActivity.this, st -> {});
                if (!lang.isEmpty()) qormTts.setLanguage(java.util.Locale.forLanguageTag(lang));
                qormTts.speak(text, android.speech.tts.TextToSpeech.QUEUE_FLUSH, null, "qorm");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void speakStop(String a) {
            runOnUiThread(() -> { if (qormTts != null) qormTts.stop(); });
        }
        @JavascriptInterface public void secureSet(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                qormSecurePrefs().edit().putString(o.optString("key",""), o.optString("value","")).apply();
                js("qormOnSecure(" + jsStr(o.optString("key","")) + ", " + jsStr("saved") + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void secureGet(String a) {
            runOnUiThread(() -> { try {
                String key = new org.json.JSONObject(a).optString("key", "");
                String val = qormSecurePrefs().getString(key, "");
                js("qormOnSecure(" + jsStr(key) + ", " + jsStr(val) + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void pickContact(String a) { runOnUiThread(() -> { if (qormContactPick != null) qormContactPick.launch(null); }); }
        @JavascriptInterface public void addEvent(String a) {
            runOnUiThread(() -> { try {
                String title = new org.json.JSONObject(a).optString("title", "QORM Event");
                android.content.Intent i = new android.content.Intent(android.content.Intent.ACTION_INSERT)
                    .setData(android.provider.CalendarContract.Events.CONTENT_URI)
                    .putExtra(android.provider.CalendarContract.Events.TITLE, title);
                startActivity(i);
                js("qormOnCalendar('opened')");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void pickFile(String a) { runOnUiThread(() -> { if (qormFilePick != null) qormFilePick.launch("*/*"); }); }
        @JavascriptInterface public void pickPhoto(String a) { runOnUiThread(() -> { if (qormPhotoPick != null) qormPhotoPick.launch("image/*"); }); }
        @JavascriptInterface public void openURL(String a) {
            runOnUiThread(() -> { try {
                String url = new org.json.JSONObject(a).optString("url", "");
                startActivity(new android.content.Intent(android.content.Intent.ACTION_VIEW, android.net.Uri.parse(url)));
                js("qormOnOpenUrl(true)");
            } catch (Exception e) { js("qormOnOpenUrl(false)"); } });
        }
        android.hardware.SensorManager qormSM() { return (android.hardware.SensorManager) getSystemService(Context.SENSOR_SERVICE); }
        void qormListen(int type, android.hardware.SensorEventListener[] slot, java.util.function.Function<android.hardware.SensorEvent, String> fn) {
            android.hardware.Sensor sensor = qormSM().getDefaultSensor(type);
            if (sensor == null) return;
            slot[0] = new android.hardware.SensorEventListener() {
                public void onSensorChanged(android.hardware.SensorEvent e) { String out = fn.apply(e); if (out != null) js(out); }
                public void onAccuracyChanged(android.hardware.Sensor sn, int ac) {}
            };
            qormSM().registerListener(slot[0], sensor, android.hardware.SensorManager.SENSOR_DELAY_UI);
        }
        @JavascriptInterface public void headingStart(String a) {
            runOnUiThread(() -> { android.hardware.SensorEventListener[] slot = new android.hardware.SensorEventListener[1];
                qormListen(android.hardware.Sensor.TYPE_ROTATION_VECTOR, slot, e -> {
                    float[] R = new float[9]; android.hardware.SensorManager.getRotationMatrixFromVector(R, e.values);
                    float[] o = new float[3]; android.hardware.SensorManager.getOrientation(R, o);
                    double az = Math.toDegrees(o[0]); if (az < 0) az += 360; return "qormOnHeading(" + az + ")";
                }); qormCompassL = slot[0]; });
        }
        @JavascriptInterface public void headingStop(String a) { runOnUiThread(() -> { if (qormCompassL != null) qormSM().unregisterListener(qormCompassL); }); }
        @JavascriptInterface public void proximityStart(String a) {
            runOnUiThread(() -> { android.hardware.SensorEventListener[] slot = new android.hardware.SensorEventListener[1];
                qormListen(android.hardware.Sensor.TYPE_PROXIMITY, slot, e -> "qormOnProximity(" + (e.values[0] < 5 ? "true" : "false") + ")"); qormProxL = slot[0]; });
        }
        @JavascriptInterface public void proximityStop(String a) { runOnUiThread(() -> { if (qormProxL != null) qormSM().unregisterListener(qormProxL); }); }
        @JavascriptInterface public void pedometerStart(String a) {
            runOnUiThread(() -> { android.hardware.SensorEventListener[] slot = new android.hardware.SensorEventListener[1];
                qormListen(android.hardware.Sensor.TYPE_STEP_COUNTER, slot, e -> "qormOnSteps(" + (int) e.values[0] + ")"); qormStepL = slot[0]; });
        }
        @JavascriptInterface public void pedometerStop(String a) { runOnUiThread(() -> { if (qormStepL != null) qormSM().unregisterListener(qormStepL); }); }
        @JavascriptInterface public void barometerStart(String a) {
            runOnUiThread(() -> { android.hardware.SensorEventListener[] slot = new android.hardware.SensorEventListener[1];
                qormListen(android.hardware.Sensor.TYPE_PRESSURE, slot, e -> "qormOnPressure(" + (e.values[0] / 10.0) + ")"); qormPressL = slot[0]; });
        }
        @JavascriptInterface public void barometerStop(String a) { runOnUiThread(() -> { if (qormPressL != null) qormSM().unregisterListener(qormPressL); }); }
        @JavascriptInterface public void getModes(String a) {
            runOnUiThread(() -> { try {
                boolean airplane = android.provider.Settings.Global.getInt(getContentResolver(), android.provider.Settings.Global.AIRPLANE_MODE_ON, 0) != 0;
                android.os.PowerManager pm = (android.os.PowerManager) getSystemService(Context.POWER_SERVICE);
                boolean low = pm != null && pm.isPowerSaveMode();
                boolean dark = (getResources().getConfiguration().uiMode & android.content.res.Configuration.UI_MODE_NIGHT_MASK) == android.content.res.Configuration.UI_MODE_NIGHT_YES;
                android.app.NotificationManager nm = (android.app.NotificationManager) getSystemService(Context.NOTIFICATION_SERVICE);
                boolean dnd = nm != null && nm.getCurrentInterruptionFilter() != android.app.NotificationManager.INTERRUPTION_FILTER_ALL;
                String json = "{"lowPower":" + low + ","darkMode":" + dark + ","airplane":" + airplane + ","dnd":" + dnd + "}";
                js("qormOnModes(" + org.json.JSONObject.quote(json) + ")");
            } catch (Exception e) {} });
        }
        @JavascriptInterface public void pendingShortcut(String a) { runOnUiThread(() -> { if (qormPendingShortcut != null) { String s = qormPendingShortcut; qormPendingShortcut = null; qormFireShortcut(s); } }); }
        @JavascriptInterface public void updateWidget(String a) {
            runOnUiThread(() -> { try {
                org.json.JSONObject o = new org.json.JSONObject(a);
                android.content.SharedPreferences.Editor e = getSharedPreferences("qorm_widget", Context.MODE_PRIVATE).edit();
                e.putString("widget_title", o.optString("title", ""));
                e.putString("widget_lines", o.optJSONArray("lines") != null ? o.optJSONArray("lines").toString() : "[]");
                e.apply();
                android.content.Intent i = new android.content.Intent(this, QormWidget.class);
                i.setAction(android.appwidget.AppWidgetManager.ACTION_APPWIDGET_UPDATE);
                int[] ids = android.appwidget.AppWidgetManager.getInstance(this).getAppWidgetIds(new android.content.ComponentName(this, QormWidget.class));
                i.putExtra(android.appwidget.AppWidgetManager.EXTRA_APPWIDGET_IDS, ids);
                sendBroadcast(i);
                js("qormOnWidget('updated')");
            } catch (Exception ex) {} });
        }
        @JavascriptInterface public void getInsets(String a) {
            runOnUiThread(() -> {
                int t=0,b=0,l=0,r=0;
                android.view.WindowInsets wi = wv.getRootWindowInsets();
                if (wi != null) {
                    float dn = getResources().getDisplayMetrics().density;
                    if (android.os.Build.VERSION.SDK_INT >= 30) {
                        android.graphics.Insets si = wi.getInsets(android.view.WindowInsets.Type.systemBars() | android.view.WindowInsets.Type.displayCutout());
                        t=(int)(si.top/dn); b=(int)(si.bottom/dn); l=(int)(si.left/dn); r=(int)(si.right/dn);
                    } else {
                        t=(int)(wi.getSystemWindowInsetTop()/dn); b=(int)(wi.getSystemWindowInsetBottom()/dn); l=(int)(wi.getSystemWindowInsetLeft()/dn); r=(int)(wi.getSystemWindowInsetRight()/dn);
                    }
                }
                js("qormOnInsets('{"top":"+t+","bottom":"+b+","left":"+l+","right":"+r+"}')");
            });
        }
        //__QORM_USER_ANDROID__
        @JavascriptInterface public void platform(String a) { runOnUiThread(() -> js("qormOnPlatform('android')")); }
        @JavascriptInterface public void volumeGet(String a) { runOnUiThread(() -> volGet()); }
        @JavascriptInterface public void brightnessGet(String a) { runOnUiThread(() -> js("qormOnBrightness(" + curBright() + ")")); }
        @JavascriptInterface public void torchGet(String a) { runOnUiThread(() -> js("qormOnTorch(" + torchOn + ")")); }
        @JavascriptInterface public void bluetoothState(String a) { runOnUiThread(() -> {
            BluetoothManager bm = (BluetoothManager) getSystemService(Context.BLUETOOTH_SERVICE);
            boolean on = bm.getAdapter() != null && bm.getAdapter().isEnabled();
            js("qormOnBluetoothState(" + on + ")");
        }); }
        @JavascriptInterface public void volumeUp(String a) { runOnUiThread(() -> vol(1)); }
        @JavascriptInterface public void volumeDown(String a) { runOnUiThread(() -> vol(-1)); }
        @JavascriptInterface public void brightnessUp(String a) { runOnUiThread(() -> bright(0.1f)); }
        @JavascriptInterface public void brightnessDown(String a) { runOnUiThread(() -> bright(-0.1f)); }
        @JavascriptInterface public void vibrate(String a) { runOnUiThread(() -> doVibrate()); }
        @JavascriptInterface public void torchToggle(String a) { runOnUiThread(() -> torch()); }
        @JavascriptInterface public void battery(String a) { runOnUiThread(() -> batteryImpl()); }
    }

    java.util.HashMap<String, Object[]> btMap = new java.util.HashMap<>();
    BluetoothLeScanner scanner;
    ScanCallback scanCb;

    void btScan() {
        try {
            BluetoothManager bm = (BluetoothManager) getSystemService(Context.BLUETOOTH_SERVICE);
            scanner = bm.getAdapter().getBluetoothLeScanner();
            btMap.clear();
            scanCb = new ScanCallback() {
                @Override public void onScanResult(int t, ScanResult r) {
                    String name = r.getDevice().getName(); if (name == null) name = "(unknown)";
                    btMap.put(r.getDevice().getAddress(), new Object[]{name, r.getRssi()});
                }
            };
            scanner.startScan(scanCb);
            wv.postDelayed(() -> { try { scanner.stopScan(scanCb); } catch (Exception e) {} reportBt(); }, 5000);
        } catch (Exception e) { js("qormOnBluetooth('[]')"); }
    }
    void reportBt() {
        StringBuilder sb = new StringBuilder("[");
        boolean first = true;
        for (Object[] v : btMap.values()) {
            if (!first) sb.append(","); first = false;
            sb.append("{\"name\":\"").append(((String) v[0]).replace("\"", "")).append("\",\"rssi\":").append(v[1]).append("}");
        }
        sb.append("]");
        js("qormOnBluetooth('" + sb.toString() + "')");
    }
    void wifiImpl() {
        try {
            WifiManager wm = (WifiManager) getApplicationContext().getSystemService(Context.WIFI_SERVICE);
            WifiInfo info = wm.getConnectionInfo();
            String ssid = info.getSSID().replace("\"", "");
            int n = 0;
            try { n = wm.getScanResults().size(); } catch (Exception e) {}
            js("qormOnWifi('{\"ssid\":\"" + ssid + "\",\"networks\":" + n + "}')");
        } catch (Exception e) { js("qormOnWifi('{\"error\":\"wifi unavailable\"}')"); }
    }
    NfcAdapter nfc;
    void nfcEnable() {
        nfc = NfcAdapter.getDefaultAdapter(this);
        if (nfc == null) { js("qormOnNfc('{\"error\":\"no NFC on this device\"}')"); return; }
        nfc.enableReaderMode(this, (Tag tag) -> {
            String id = bytesToHex(tag.getId());
            js("qormOnNfc('{\"id\":\"" + id + "\"}')");
            runOnUiThread(() -> nfc.disableReaderMode(this));
        }, NfcAdapter.FLAG_READER_NFC_A | NfcAdapter.FLAG_READER_NFC_B | NfcAdapter.FLAG_READER_NFC_F | NfcAdapter.FLAG_READER_NFC_V, null);
    }
    static String bytesToHex(byte[] b) {
        StringBuilder sb = new StringBuilder();
        for (byte x : b) sb.append(String.format("%02X", x));
        return sb.toString();
    }
    void vol(int dir) {
        AudioManager am = (AudioManager) getSystemService(Context.AUDIO_SERVICE);
        am.adjustStreamVolume(AudioManager.STREAM_MUSIC, dir > 0 ? AudioManager.ADJUST_RAISE : AudioManager.ADJUST_LOWER, 0);
        int cur = am.getStreamVolume(AudioManager.STREAM_MUSIC), max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC);
        js("qormOnVolume(" + ((float) cur / max) + ")");
    }
    void volGet() {
        AudioManager am = (AudioManager) getSystemService(Context.AUDIO_SERVICE);
        int cur = am.getStreamVolume(AudioManager.STREAM_MUSIC), max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC);
        js("qormOnVolume(" + ((float) cur / max) + ")");
    }
    float curBright() {
        try {
            int b = android.provider.Settings.System.getInt(getContentResolver(), android.provider.Settings.System.SCREEN_BRIGHTNESS);
            brightness = b / 255f;
        } catch (Exception e) {}
        return brightness;
    }
    float brightness = 0.5f;
    void bright(float d) {
        brightness = Math.max(0.05f, Math.min(1f, brightness + d));
        WindowManager.LayoutParams lp = getWindow().getAttributes();
        lp.screenBrightness = brightness;
        getWindow().setAttributes(lp);
        js("qormOnBrightness(" + brightness + ")");
    }
    void doVibrate() {
        Vibrator v = (Vibrator) getSystemService(Context.VIBRATOR_SERVICE);
        try { v.vibrate(VibrationEffect.createOneShot(200, VibrationEffect.DEFAULT_AMPLITUDE)); } catch (Exception e) {}
    }
    boolean torchOn = false;
    void torch() {
        try {
            CameraManager cm = (CameraManager) getSystemService(Context.CAMERA_SERVICE);
            String id = cm.getCameraIdList()[0];
            torchOn = !torchOn;
            cm.setTorchMode(id, torchOn);
            js("qormOnTorch(" + torchOn + ")");
        } catch (Exception e) { js("qormOnTorch(false)"); }
    }
    void batteryImpl() {
        Intent i = registerReceiver(null, new IntentFilter(Intent.ACTION_BATTERY_CHANGED));
        int lvl = i.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
        int scale = i.getIntExtra(BatteryManager.EXTRA_SCALE, 100);
        int st = i.getIntExtra(BatteryManager.EXTRA_STATUS, -1);
        boolean charging = st == BatteryManager.BATTERY_STATUS_CHARGING || st == BatteryManager.BATTERY_STATUS_FULL;
        js("qormOnBattery(" + ((float) lvl / scale) + "," + charging + ")");
    }

    void doBio() {
        try {
            Executor ex = ContextCompat.getMainExecutor(this);
            BiometricPrompt bp = new BiometricPrompt(this, ex, new BiometricPrompt.AuthenticationCallback() {
                @Override public void onAuthenticationSucceeded(BiometricPrompt.AuthenticationResult r) { js("qormOnBiometric(true,'authenticated')"); }
                @Override public void onAuthenticationError(int code, CharSequence msg) { js("qormOnBiometric(false,'" + msg + "')"); }
            });
            BiometricPrompt.PromptInfo info = new BiometricPrompt.PromptInfo.Builder()
                .setTitle("Authenticate").setNegativeButtonText("Cancel").build();
            bp.authenticate(info);
        } catch (Exception e) { js("qormOnBiometric(false,'" + e + "')"); }
    }

    void startRec() {
        try {
            recPath = getCacheDir().getAbsolutePath() + "/qorm-rec.m4a";
            mrec = new MediaRecorder();
            mrec.setAudioSource(MediaRecorder.AudioSource.MIC);
            mrec.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4);
            mrec.setAudioEncoder(MediaRecorder.AudioEncoder.AAC);
            mrec.setOutputFile(recPath);
            mrec.prepare();
            mrec.start();
        } catch (Exception e) { js("qormOnAudioError('rec: " + e + "')"); }
    }

    void stopRec() {
        try {
            mrec.stop(); mrec.release(); mrec = null;
            java.io.File f = new java.io.File(recPath);
            byte[] bytes = new byte[(int) f.length()];
            java.io.FileInputStream in = new java.io.FileInputStream(f);
            in.read(bytes); in.close();
            String b64 = Base64.encodeToString(bytes, Base64.NO_WRAP);
            js("qormOnAudio('data:audio/mp4;base64," + b64 + "')");
        } catch (Exception e) { js("qormOnAudioError('stop: " + e + "')"); }
    }

    void getLoc() {
        try {
            Location l = lm.getLastKnownLocation(LocationManager.GPS_PROVIDER);
            if (l == null) l = lm.getLastKnownLocation(LocationManager.NETWORK_PROVIDER);
            if (l != null) js("qormOnLocation(" + l.getLatitude() + "," + l.getLongitude() + "," + l.getAccuracy() + ")");
            else lm.requestSingleUpdate(LocationManager.NETWORK_PROVIDER, this, null);
        } catch (SecurityException e) { js("qormOnLocationError('permission needed')"); }
    }

    @Override public void onSensorChanged(SensorEvent e) {
        float[] R = new float[9]; float[] o = new float[3];
        SensorManager.getRotationMatrixFromVector(R, e.values);
        SensorManager.getOrientation(R, o);
        double d = 180.0 / Math.PI;
        js("qormOnMotion(" + (o[0]*d) + "," + (o[1]*d) + "," + (o[2]*d) + ")");
    }
    @Override public void onAccuracyChanged(Sensor s, int a) {}
    @Override public void onLocationChanged(Location l) { js("qormOnLocation(" + l.getLatitude() + "," + l.getLongitude() + "," + l.getAccuracy() + ")"); }
    @Override public void onProviderEnabled(String p) {}
    @Override public void onProviderDisabled(String p) {}
    @Override public void onStatusChanged(String p, int s, Bundle b) {}

    android.content.SharedPreferences qormSecurePrefs() throws Exception {
        androidx.security.crypto.MasterKey mk = new androidx.security.crypto.MasterKey.Builder(this).setKeyScheme(androidx.security.crypto.MasterKey.KeyScheme.AES256_GCM).build();
        return androidx.security.crypto.EncryptedSharedPreferences.create(this, "qorm_secure", mk,
            androidx.security.crypto.EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            androidx.security.crypto.EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM);
    }
    void qormFireShortcut(String id) { String p = org.json.JSONObject.quote(id); js("(function(){var f=function(){if(window.qormEmit){qormEmit('shortcut'," + p + ");}else{setTimeout(f,300);}};f();})()"); }
    @Override protected void onNewIntent(android.content.Intent intent) { super.onNewIntent(intent); setIntent(intent); String q = intent.getStringExtra("qorm_shortcut"); if (q != null) qormFireShortcut(q); }
    void js(String s) { runOnUiThread(() -> wv.evaluateJavascript(s, null)); }
    String jsStr(String s) { return "\"" + s.replace("\\","\\\\").replace("\"","\\\"").replace("\n","\\n") + "\""; }
}
`
}
