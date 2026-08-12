package canvas

import (
	"testing"
	"time"
)

func TestRaidenPerf(t *testing.T) {
	e, surf, rt := raidenFixture(t)
	keys, _ := rt.State["keys"].(map[string]any)
	keys["fire"] = true
	// Warm up.
	for i := 0; i < 5; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = nowMinus(time.Millisecond)
		}
		e.MarkDirty()
		e.DrawFrame(surf)
	}
	// Time 30 frames.
	start := time.Now()
	for i := 0; i < 30; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = nowMinus(time.Millisecond)
		}
		e.MarkDirty()
		e.DrawFrame(surf)
	}
	dur := time.Since(start)
	msPerFrame := float64(dur) / float64(30) / 1e6
	t.Logf("30 frames in %v = %.1f ms/frame (%.0f fps)", dur, msPerFrame, 30*float64(time.Second)/float64(dur))
	if msPerFrame > 50 {
		t.Errorf("frame time %.1f ms exceeds 50ms limit (rendering too slow)", msPerFrame)
	}
	bullets, _ := rt.State["bullets"].([]any)
	if len(bullets) == 0 {
		t.Error("no bullets fired after 35 ticks — fire logic broken")
	}
	enemies, _ := rt.State["enemies"].([]any)
	if len(enemies) == 0 {
		t.Error("no enemies spawned after 35 ticks — spawn logic broken")
	}
}
