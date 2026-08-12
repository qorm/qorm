package audio

import "testing"

func TestResolveWebSrcJoinsBase(t *testing.T) {
	got, err := ResolveWebSrc("/games/mario/", "audio/coin.wav")
	if err != nil {
		t.Fatalf("ResolveWebSrc: %v", err)
	}
	if got != "/games/mario/audio/coin.wav" {
		t.Errorf("got %q, want /games/mario/audio/coin.wav", got)
	}
	// No trailing slash on base.
	got, err = ResolveWebSrc("/games/mario", "audio/music.wav")
	if err != nil {
		t.Fatalf("ResolveWebSrc: %v", err)
	}
	if got != "/games/mario/audio/music.wav" {
		t.Errorf("got %q", got)
	}
	// Absolute URL base.
	got, err = ResolveWebSrc("https://cdn.example/app/", "sfx/hit.wav")
	if err != nil {
		t.Fatalf("ResolveWebSrc: %v", err)
	}
	if got != "https://cdn.example/app/sfx/hit.wav" {
		t.Errorf("got %q", got)
	}
}

func TestResolveWebSrcRejects(t *testing.T) {
	cases := []struct {
		base, src string
	}{
		{"/games/mario/", ""},
		{"/games/mario/", "https://evil/x.wav"},
		{"/games/mario/", "data:audio/wav;base64,AA"},
		{"/games/mario/", "/etc/passwd"},
		{"/games/mario/", "../../etc/passwd"},
		{"/games/mario/", "audio/../../../etc/passwd"},
		{"", "audio/coin.wav"},
	}
	for _, c := range cases {
		if _, err := ResolveWebSrc(c.base, c.src); err == nil {
			t.Errorf("ResolveWebSrc(%q, %q) should error", c.base, c.src)
		}
	}
}

func TestResolveWebSrcCleansDotDotInside(t *testing.T) {
	// Nested .. that stays under the logical root is OK after Clean.
	got, err := ResolveWebSrc("/g/", "audio/../sfx/a.wav")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/g/sfx/a.wav" {
		t.Errorf("got %q, want /g/sfx/a.wav", got)
	}
}
