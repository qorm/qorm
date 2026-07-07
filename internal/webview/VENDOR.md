# Vendored: webview

`internal/webview` is vendored from [webview/webview_go](https://github.com/webview/webview_go)
(commit 6173450, 2024-08-31) and the bundled [webview/webview](https://github.com/webview/webview)
C/C++ header, both MIT-licensed (see LICENSE and libs/*/LICENSE). Copyright (c) 2017
Serge Zaitsev and contributors.

It is vendored — not imported as a module — because the upstream Go binding has
been unmaintained for ~2 years; keeping it in-tree lets QORM fix and update it.
Local modifications are noted at the top of the changed files.
