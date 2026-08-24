#!/usr/bin/env sh
set -e
export PATH="$HOME/.local/lib/go/bin:$PATH"
GO=go1.27rc1
cd "$(dirname "$0")/../.."
echo "── client → server ──"
bun run examples/encoding/ts-client/main.ts
echo "── server ──"
"$GO" run ./examples/encoding/go-server
echo "── server → client ──"
bun run examples/encoding/ts-client/decode-response.ts
