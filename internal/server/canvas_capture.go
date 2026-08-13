package server

import "github.com/qorm/qorm/internal/mcp"

// SetCanvasCaptureProvider wires the native host's last-presented frame into
// MCP. Keeping the provider on Server makes it survive hot reload/OTA runtime
// swaps, which rebuild the shared MCP handler.
func (s *Server) SetCanvasCaptureProvider(provider mcp.CanvasCaptureProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.canvasCapture = provider
	if s.agent != nil {
		s.agent.SetCanvasCaptureProvider(provider)
	}
}
