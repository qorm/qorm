package mcp

import (
	"os"
	"testing"
)

// TestMCPDocInSync keeps docs/agent/mcp-tools.md generated from the live tool
// registry, so the agent-facing MCP reference never drifts from the server.
// Regenerate with: QORM_UPDATE_DOCS=1 go test ./internal/mcp/
func TestMCPDocInSync(t *testing.T) {
	const path = "../../docs/agent/mcp-tools.md"
	want := ToolsMarkdown()
	if os.Getenv("QORM_UPDATE_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("docs/agent/mcp-tools.md is out of sync with the tool registry — run: QORM_UPDATE_DOCS=1 go test ./internal/mcp/")
	}
}
