package capability

import "testing"

func has(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// TestPermsFor: a packaged app declares only the permissions its used
// capabilities require — nothing for a UI that touches no capability.
func TestPermsFor(t *testing.T) {
	used := map[string]bool{"camera": true, "location": true}
	ios := PermsFor(used, IOS)
	if !has(ios, "NSCameraUsageDescription") || !has(ios, "NSLocationWhenInUseUsageDescription") {
		t.Errorf("ios perms missing camera/location: %v", ios)
	}
	if has(ios, "NSBluetoothAlwaysUsageDescription") {
		t.Errorf("ios perms should NOT include bluetooth (unused): %v", ios)
	}
	android := PermsFor(used, Android)
	if !has(android, "android.permission.CAMERA") || !has(android, "android.permission.ACCESS_FINE_LOCATION") {
		t.Errorf("android perms missing camera/location: %v", android)
	}
	// a capability-free UI needs no permissions
	if p := PermsFor(map[string]bool{"button": true, "text": true, "column": true}, IOS); len(p) != 0 {
		t.Errorf("capability-free app should need no perms, got %v", p)
	}
	// videocapture pulls BOTH camera + mic; deduped against an also-used camera
	vc := PermsFor(map[string]bool{"videocapture": true, "camera": true}, IOS)
	n := 0
	for _, p := range vc {
		if p == "NSCameraUsageDescription" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("camera perm should be deduped once, got %d in %v", n, vc)
	}
}
