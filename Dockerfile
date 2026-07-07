# QORM execution environment: the qorm CLI + the Go toolchain + the QORM module,
# so `qorm package` (which builds the client WASM) works out of the box — no local
# Go install needed. The container runs from the module dir (/qorm) so the WASM
# build resolves; mount your app and use absolute paths.
#
#   docker run --rm -v "$PWD:/app" ghcr.io/qorm/qorm run /app
#   docker run --rm -v "$PWD:/app" ghcr.io/qorm/qorm package /app -p web -o /app/out
FROM golang:1.24-bookworm

WORKDIR /qorm
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build the CLI, and warm the WASM build cache so first `package` is fast.
RUN go build -trimpath -ldflags "-s -w" -o /usr/local/bin/qorm ./cmd/qorm \
 && GOOS=js GOARCH=wasm go build -o /dev/null ./cmd/qorm-wasm

# Run from the module dir so `go build .../cmd/qorm-wasm` resolves during packaging.
WORKDIR /qorm
ENTRYPOINT ["qorm"]
CMD ["--help"]
