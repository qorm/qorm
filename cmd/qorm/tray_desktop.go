//go:build desktop

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"time"
)

// terminateParent asks the parent (the app-window process) to exit. SIGTERM
// where the platform supports it; Windows can only hard-kill.
func terminateParent() {
	p := os.Getppid()
	if p <= 1 {
		return
	}
	proc, err := os.FindProcess(p)
	if err != nil {
		return
	}
	if proc.Signal(syscall.SIGTERM) != nil {
		proc.Kill()
	}
}

// traySelected routes a tray click: a "quit" item terminates the app, anything
// else fires qormEmit('tray', {id}) in the app window via the local server.
func traySelected(id string) {
	if id == "quit" {
		terminateParent()
		os.Exit(0)
	}
	js := "qormEmit('tray',{id:" + strconv.Quote(id) + "})"
	body, _ := json.Marshal(map[string]string{"id": "main", "op": "eval", "js": js})
	http.Post(gTrayURL+"window", "application/json", bytes.NewReader(body))
}

func runTray(url, title, trayJSON string) {
	gTrayURL = url
	// exit with the app (the WebView process is our parent)
	parent := os.Getppid()
	go func() {
		for {
			time.Sleep(400 * time.Millisecond)
			if os.Getppid() != parent {
				os.Exit(0)
			}
		}
	}()
	icon, _ := iconFS.ReadFile("icons/tray.png")
	if trayJSON != "" {
		nativeTrayJSON(icon, trayJSON, title)
		return
	}
	items := []string{"Activity Log", "Open in Browser", "Quit QORM"}
	nativeTray(icon, items, title, func(i int) {
		switch i {
		case 0:
			openBrowser(url + "logwindow")
		case 1:
			openBrowser(url)
		case 2:
			terminateParent() // quit the app window too
			os.Exit(0)
		}
	})
}
