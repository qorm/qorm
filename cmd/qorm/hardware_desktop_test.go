//go:build desktop

package main

import (
	"strings"
	"testing"
)

func TestParseXrandrScreens(t *testing.T) {
	sample := `Screen 0: minimum 320 x 200, current 3840 x 1080, maximum 16384 x 16384
HDMI-1 connected primary 1920x1080+0+0 (normal left inverted right x axis y axis) 527mm x 296mm
DP-1 connected 1920x1080+1920+0 (normal left inverted right x axis y axis) 527mm x 296mm
`
	got := formatScreensJSON(parseXrandrScreens(sample))
	if !strings.Contains(got, `"w":1920`) || !strings.Contains(got, `"main":true`) {
		t.Fatalf("primary HDMI missing: %s", got)
	}
	if !strings.Contains(got, `"main":false`) {
		t.Fatalf("secondary DP missing: %s", got)
	}
	if strings.Count(got, `"w":1920`) != 2 {
		t.Fatalf("expected two displays: %s", got)
	}
}
