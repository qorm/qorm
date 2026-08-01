package canvas

import "testing"

func TestParseNativeCallback(t *testing.T) {
	cases := []struct {
		js   string
		name string
		arg  any
		ok   bool
	}{
		{`qormOnNetwork("{\"online\":true}")`, "qormOnNetwork", `{"online":true}`, true},
		{`qormOnOpenUrl(true)`, "qormOnOpenUrl", true, true},
		{`qormOnVolume("42")`, "qormOnVolume", "42", true},
		{`qormOnNotify(false)`, "qormOnNotify", false, true},
		{`qormOnSecure("k", "saved")`, "", nil, false}, // multi-arg is v2
		{`garbage`, "", nil, false},
		{`qormOnX()`, "qormOnX", nil, true},
	}
	for _, c := range cases {
		name, arg, ok := ParseNativeCallback(c.js)
		if name != c.name || arg != c.arg || ok != c.ok {
			t.Errorf("ParseNativeCallback(%q) = %q,%v,%v want %q,%v,%v", c.js, name, arg, ok, c.name, c.arg, c.ok)
		}
	}
}
