package canvas

import (
	"testing"
)

func TestParseSVGPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		fill        bool
		stroke      bool
		strokeWidth float64
		expectedOps int
	}{
		{
			name:        "Empty path",
			path:        "",
			fill:        true,
			stroke:      false,
			strokeWidth: 0,
			expectedOps: 0,
		},
		{
			name:        "Simple line fill",
			path:        "M 0 0 L 10 0 L 10 10 Z",
			fill:        true,
			stroke:      false,
			strokeWidth: 0,
			expectedOps: 4, // Save, Clip, Paint, Restore
		},
		{
			name:        "Simple line stroke",
			path:        "M 0 0 L 10 0",
			fill:        false,
			stroke:      true,
			strokeWidth: 2,
			expectedOps: 12, // 1 segment (4 ops) + 2 joints (8 ops) = 12 ops
		},
		{
			name:        "Cubic Bezier curve fill",
			path:        "M 0 0 C 5 0 5 10 10 10 Z",
			fill:        true,
			stroke:      false,
			strokeWidth: 0,
			expectedOps: 4, // 1 subpath fill
		},
		{
			name:        "Multiple subpaths",
			path:        "M 0 0 L 10 0 Z M 20 20 L 30 20 Z",
			fill:        true,
			stroke:      false,
			strokeWidth: 0,
			expectedOps: 8, // 2 subpaths filled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := ParseSVGPath(tt.path, tt.fill, tt.stroke, tt.strokeWidth)
			if len(ops) != tt.expectedOps {
				t.Errorf("ParseSVGPath() generated %d ops, expected %d", len(ops), tt.expectedOps)
			}
		})
	}
}

func TestTokenizeSVGPath(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"M10,20", []string{"M", "10", "20"}},
		{"M 10 20", []string{"M", "10", "20"}},
		{"M-10-20", []string{"M", "-10", "-20"}},
		{"M 10.5, -20.5", []string{"M", "10.5", "-20.5"}},
		{"c1,2 3,4", []string{"c", "1", "2", "3", "4"}},
		{"m 1e-5 2", []string{"m", "1e-5", "2"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens := tokenizeSVGPath(tt.input)
			if len(tokens) != len(tt.expected) {
				t.Fatalf("Expected %d tokens, got %d: %v", len(tt.expected), len(tokens), tokens)
			}
			for i, tok := range tokens {
				if tok != tt.expected[i] {
					t.Errorf("Token %d: expected %q, got %q", i, tt.expected[i], tok)
				}
			}
		})
	}
}
