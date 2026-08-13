#!/bin/bash
PORT=59260
ARTIFACTS_DIR="/Users/dmy/.gemini/antigravity/brain/1221b638-99fe-457b-b5ba-fca3ff10b34e"

# 1. Take screenshot of Tab 1 (Video)
screencapture "$ARTIFACTS_DIR/video_tab.png"
echo "Captured Video Tab"

# 2. Switch to Typography Tab
curl -s -X POST http://127.0.0.1:$PORT/mcp -H "Content-Type: application/json" -d '{
  "jsonrpc": "2.0", "id": 1, "method": "qorm_dispatch", "params": { "action": "switchTab", "args": { "tab": "typography" } }
}'
sleep 2

# 3. Take screenshot of Tab 2 (Typography)
screencapture "$ARTIFACTS_DIR/typography_tab.png"
echo "Captured Typography Tab"

# 4. Switch to Vector Tab
curl -s -X POST http://127.0.0.1:$PORT/mcp -H "Content-Type: application/json" -d '{
  "jsonrpc": "2.0", "id": 2, "method": "qorm_dispatch", "params": { "action": "switchTab", "args": { "tab": "vector" } }
}'
sleep 1

# 5. Trigger morph animation
curl -s -X POST http://127.0.0.1:$PORT/mcp -H "Content-Type: application/json" -d '{
  "jsonrpc": "2.0", "id": 3, "method": "qorm_dispatch", "params": { "action": "triggerMorph" }
}'
# Wait 0.3s to capture mid-animation
sleep 0.3
screencapture "$ARTIFACTS_DIR/vector_tab.png"
echo "Captured Vector Tab"

