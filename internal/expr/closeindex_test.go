package expr

import "testing"

// TestCloseIndex pins the quote-aware "}}" scan both the loader's static
// checks and the runtime's binding evaluation rely on: the first "}}" outside
// a string literal closes the binding, and one inside a literal never does.
func TestCloseIndex(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"}}", 0},
		{" state.count }} tail", 13},
		{" '}}' }}", 6},           // }} inside single quotes is skipped
		{` "}}" }}`, 6},           // }} inside double quotes is skipped
		{` '\'}}' }}`, 8},         // escaped quote does not close the literal
		{" 'a' + '}}' }}", 12},    // literal closes, scan resumes outside
		{" } } }}", 5},            // lone } chars are not a delimiter
		{"", -1},                  // nothing to close
		{" state.count", -1},      // no delimiter at all
		{" 'unterminated }}", -1}, // the only }} sits inside an open literal
		{` "x\`, -1},              // trailing backslash consumes the end
		{" 'a\\'", -1},            // escape then string runs out
	}
	for _, c := range cases {
		if got := CloseIndex(c.in); got != c.want {
			t.Errorf("CloseIndex(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
