package integration

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// allWidgetTypes is every widget type string handled by the render switch,
// extracted from the case labels. TestAllWidgetTypesRender renders each in
// isolation to prove none is broken (renders, no unknown marker, no panic on
// minimal input) — a regression gate for the whole 160+ widget catalog.
var allWidgetTypes = []string{
	"absorbpointer", "accordion", "actionsheet", "activityindicator", "alert", "alertdialog", "animatedalign", "animatedcontainer", "animatedopacity",
	"animatedpadding", "animatedpositioned", "appbar", "aspectratio", "audiorecorder", "autocomplete", "avatar", "badge",
	"backbutton", "banner", "battery", "biometric", "bluetooth", "bottomappbar", "bottomnav", "bottomnavigationbar", "breadcrumb", "brightness",
	"closebutton", "limitedbox", "navigationdrawer",
	"button", "camera", "carousel", "chart", "checkbox", "checkboxlisttile", "chip", "choicechip",
	"circularprogress", "circularprogressindicator", "clipboard", "contextmenu", "cupertinoactionsheet", "cupertinoactivityindicator", "cupertinoalertdialog", "cupertinocontextmenu",
	"cupertinodatepicker", "cupertinolargetitle", "cupertinolistsection", "cupertinopicker", "cupertinoslidingsegmentedcontrol", "cupertinotimepicker", "datatable", "datepicker", "descriptions",
	"deviceinfo", "dialog", "dismissible", "displays", "divider", "dockbadge", "draggable", "dragtarget", "drawer", "droptarget", "dropdown",
	"dropdownbutton", "empty", "expansionpanel", "expansiontile", "fab", "faceid", "field", "file",
	"filepicker", "filterchip", "fingerprint", "flashlight", "floatingactionbutton", "form", "formfield", "geolocation", "gesture",
	"gesturedetector", "gridview", "haptics", "icon", "ignorepointer", "image", "inkwell", "input", "inputchip",
	"indexedstack", "keepawake", "keychain", "keyvalue", "largetitle", "link", "list", "listitem", "listsection",
	"listtile", "location", "loginitem", "materialstepper", "menu", "metric", "modal", "navigationbar",
	"navigationrail", "network", "nfc", "notify", "offstage", "openlink", "openurl", "orientation", "pageview",
	"pagination", "photo", "photopicker", "picker", "progress", "radio", "radiolisttile", "rangeslider",
	"rating", "recorder", "refreshindicator", "richtext", "rotatedbox", "scaffold", "screencapture", "screenrecord",
	"screenrecording", "screens", "screenshot", "searchbar", "securestorage", "segmented", "select", "selectabletext", "sensors",
	"share", "skeleton", "slider", "slidingsegmentedcontrol", "sliverappbar", "snackbar", "spacer", "speak",
	"speechinput", "spinner", "startatlogin", "stat", "stepper", "steps", "storage", "stt",
	"switch", "switchlisttile", "table", "tabs", "tag", "text", "textarea", "textformfield",
	"timeline", "timepicker", "torch", "transform", "tree", "tts", "verticaldivider", "vibrate", "video",
	"volume", "wakelock", "when", "wifi", "wrap",
}

func TestAllWidgetTypesRender(t *testing.T) {
	for _, wt := range allWidgetTypes {
		wt := wt
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("widget %q panicked on minimal input: %v", wt, r)
				}
			}()
			root := &model.Node{Type: "scaffold", ID: "r", Children: []*model.Node{{Type: wt, ID: wt + "_w"}}}
			app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
			html := render.Render(qrt.New(app)).HTML
			if strings.Contains(html, `data-qorm-unknown="`+wt+`"`) {
				t.Errorf("widget type %q renders as the unknown marker — not wired in render.go", wt)
			}
		}()
	}
}
