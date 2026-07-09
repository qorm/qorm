#!/usr/bin/env bash
# Record the QORM human-AI demo GIF on macOS 15+/26, where the in-process capture
# APIs are dead (takeSnapshot -> white; CGWindowListCreateImage -> removed;
# ScreenCaptureKit -> SIGBUS for a bare CLI). Uses the fixed `qorm shot --live`,
# which captures the REAL running windows via Apple's screencapture.
#
# Prereq: launch the app first (in its own terminal) and grant that terminal
# Screen Recording (System Settings > Privacy):
#     qorm run examples/counter --app
# Then run this. QORM=/path/to/qorm and OUT=/path/to.gif override the defaults.
