package main

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// printDeviceConnect prints how a physical phone joins the SAME live session as
// the dev machine + agent — the on-device app then hot-updates, is inspectable
// over MCP, and posts its real-screen self-measurement back to /measure.
func printDeviceConnect(port int, scheme string) {
	fmt.Println("\n  物理机联调 · connect a physical device to this live session:")
	ips := lanIPv4s()
	if len(ips) == 0 {
		fmt.Println("    (no LAN address found — is Wi-Fi/Ethernet up?)")
	}
	for _, ip := range ips {
		fmt.Printf("    Wi-Fi   %s://%s:%d/            open in the phone's browser (same network)\n", scheme, ip, port)
	}
	if devs := adbReverseAll(port); len(devs) > 0 {
		fmt.Printf("    USB     adb reverse set for %d Android device(s) → open %s://localhost:%d/ on the phone\n", len(devs), scheme, port)
	} else if adbAvailable() {
		fmt.Println("    USB     (no Android device detected; plug in + enable USB debugging, then re-run)")
	}
	fmt.Println("    The device shares the live app: agent edits hot-reload on it, and qorm_measure/qorm_check_layout")
	fmt.Println("    read the REAL device's rendering (screen size, fonts, WebView) — true on-device 联调.")
}

// lanIPv4s returns this host's usable non-loopback IPv4 addresses, real home/
// office LAN ranges first (so the printed URL is the one a phone can reach) and
// virtual/VPN ranges (CGNAT, benchmark) dropped.
func lanIPv4s() []string {
	var real, other []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil || ip.IsLinkLocalUnicast() {
				continue
			}
			s := ip.String()
			b := ip.To4()
			// drop 100.64/10 (CGNAT) and 198.18/15 (benchmarking) — not a real LAN
			if b[0] == 100 && b[1]&0xc0 == 64 || b[0] == 198 && (b[1] == 18 || b[1] == 19) {
				continue
			}
			// classic private LAN ranges first
			if b[0] == 192 && b[1] == 168 || b[0] == 10 || b[0] == 172 && b[1] >= 16 && b[1] <= 31 {
				real = append(real, s)
			} else {
				other = append(other, s)
			}
		}
	}
	return append(real, other...)
}

func adbAvailable() bool {
	_, err := exec.LookPath("adb")
	return err == nil
}

// adbReverseAll runs `adb reverse tcp:port tcp:port` on every connected Android
// device so the phone's localhost:port maps to this dev server over USB.
func adbReverseAll(port int) []string {
	if !adbAvailable() {
		return nil
	}
	out, err := exec.Command("adb", "devices").Output()
	if err != nil {
		return nil
	}
	var devices []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "device" {
			devices = append(devices, fields[0])
		}
	}
	spec := fmt.Sprintf("tcp:%d", port)
	var ok []string
	for _, d := range devices {
		if exec.Command("adb", "-s", d, "reverse", spec, spec).Run() == nil {
			ok = append(ok, d)
		}
	}
	return ok
}
