package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/qorm/qorm/internal/updates"
)

// cmdUpdates runs the OTA publish server: serves bundles from a directory with
// staged (canary) rollout from <dir>/rollout.json.
func cmdUpdates(args []string) int {
	dir := "."
	port := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			if i+1 < len(args) {
				i++
				port, _ = strconv.Atoi(args[i])
			}
		default:
			dir = args[i]
		}
	}
	srv, err := updates.New(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("QORM update server on http://%s  (dir %s)\n", ln.Addr().String(), dir)
	fmt.Printf("  clients fetch:  /resolve?app=<id>&client=<id>\n")
	if err := http.Serve(ln, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
