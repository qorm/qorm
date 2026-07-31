package purego

import (
	"testing"
)

func TestDlopenAndDlsym(t *testing.T) {
	// Try to open libSystem which should always exist
	handle, err := Dlopen("/usr/lib/libSystem.B.dylib", RTLD_LAZY|RTLD_GLOBAL)
	if err != nil {
		t.Fatalf("Dlopen failed: %v", err)
	}
	if handle == 0 {
		t.Fatalf("Dlopen returned 0 handle without error")
	}

	// Find the address of 'printf'
	sym, err := Dlsym(handle, "printf")
	if err != nil {
		t.Fatalf("Dlsym failed: %v", err)
	}
	if sym == 0 {
		t.Fatalf("Dlsym returned 0 symbol without error")
	}

	t.Logf("Successfully loaded printf at %x", sym)
}
