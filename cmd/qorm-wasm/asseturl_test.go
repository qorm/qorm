package main

import "testing"

func TestCanonicalAssetURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"/games/raiden/assets/player.png", "/games/raiden/assets/player.png"},
		{"/games/raiden/assets/player.png?v=123", "/games/raiden/assets/player.png"},
		{"/games/mario/assets/ground.png?v=1&x=2", "/games/mario/assets/ground.png"},
		{"/games/raiden/themes/raiden.json?v=9#frag", "/games/raiden/themes/raiden.json"},
		{"assets/player.png?v=1", "assets/player.png"},
		{"https://cdn.example/a.png?v=1", "https://cdn.example/a.png"},
	}
	for _, tc := range cases {
		if got := canonicalAssetURL(tc.in); got != tc.want {
			t.Errorf("canonicalAssetURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
