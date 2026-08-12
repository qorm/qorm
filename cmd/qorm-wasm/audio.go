//go:build js && wasm

package main

import "github.com/qorm/qorm/internal/audio"

// Wire the browser HTMLAudioElement sink so qscript playSound / playMusic /
// stopMusic emit real audio on the games page and other WASM hosts. Desktop
// cmd/qorm registers StdoutSink instead; without this init WASM stays silent
// (nopSink).
func init() { audio.RegisterSink(&audio.WebSink{}) }
