//go:build !desktop

package main

import (
	"strings"
	"testing"
)

func TestUsageDescribesPureCanvasMeasureAndPhysicalFlag(t *testing.T) {
	out := captureStderr(t, usage)
	for _, want := range []string{
		"qorm measure <app-dir> [--width N] [--physical]",
		"pure-Go canvas by default",
		"qorm check <app-dir> (--checks c.json | --audit) [--width N] [--physical]",
		"interactive flows drive the pure-Go canvas by default",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("usage missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "self-measure layout & styles (needs -tags desktop)") {
		t.Fatal("usage still claims headless canvas measure needs desktop")
	}
}
