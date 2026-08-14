//go:build !(darwin && desktop)

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/qorm/platform/internal/render/canvas"
)

const (
	maxCanvasPNGDimension = 4096
	maxCanvasPNGPixels    = 16 * 1024 * 1024
)

func validateCanvasPNGSize(width, height int) error {
	if width < 1 || height < 1 {
		return fmt.Errorf("width and height must be positive")
	}
	if width > maxCanvasPNGDimension || height > maxCanvasPNGDimension {
		return fmt.Errorf("canvas PNG dimensions exceed %d px per edge", maxCanvasPNGDimension)
	}
	if int64(width)*int64(height) > maxCanvasPNGPixels {
		return fmt.Errorf("canvas PNG exceeds the %d-pixel safety limit", maxCanvasPNGPixels)
	}
	return nil
}

func renderCanvasPNG(appDir string, width, height int, out string) error {
	if err := validateCanvasPNGSize(width, height); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(out), ".png") {
		return fmt.Errorf("output path must end in .png")
	}
	if info, err := os.Lstat(out); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("output path must be a regular file, not %s", info.Mode().Type())
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output: %w", err)
	}
	parent := filepath.Dir(out)
	if info, err := os.Stat(parent); err != nil {
		return fmt.Errorf("output parent: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("output parent is not a directory")
	}

	rt, err := loadRuntime(appDir, "", "")
	if err != nil {
		return err
	}
	surface := canvas.NewHeadlessSurface(image.Pt(width, height))
	engine := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	engine.DrawFrame(surface)
	engine.SettleEntrances()
	engine.DrawFrame(surface)

	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(f, surface.Frame())
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}
