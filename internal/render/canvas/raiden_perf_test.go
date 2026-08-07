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
		for tm := range e.timers { e.timers[tm].nextFire = nowMinus(time.Millisecond) }
		e.MarkDirty(); e.DrawFrame(surf)
	}
	// Time 30 frames.
	start := time.Now()
	for i := 0; i < 30; i++ {
		for tm := range e.timers { e.timers[tm].nextFire = nowMinus(time.Millisecond) }
		e.MarkDirty()
		e.DrawFrame(surf)
	}
	dur := time.Since(start)
	t.Logf("30 frames in %v = %.1f ms/frame (%.0f fps)", dur, float64(dur)/float64(30)/1e6, 30*float64(time.Second)/float64(dur))
	// Count nodes measured.
	rt2 := rt
	bullets, _ := rt2.State["bullets"].([]any)
	enemies, _ := rt2.State["enemies"].([]any)
	starsFar, _ := rt2.State["starsFar"].([]any)
	t.Logf("entities: bullets=%d enemies=%d starsFar=%d", len(bullets), len(enemies), len(starsFar))
}
