package render_test

import (
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func benchApp(b *testing.B, dir string) *qrt.Runtime {
	app, err := loader.LoadDir(dir)
	if err != nil {
		b.Fatal(err)
	}
	return qrt.New(app)
}

func BenchmarkRenderDashboard(b *testing.B) {
	rt := benchApp(b, "../../examples/dashboard")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = render.Render(rt)
	}
}

func BenchmarkRenderGallery(b *testing.B) {
	rt := benchApp(b, "../../examples/gallery")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = render.Render(rt)
	}
}
