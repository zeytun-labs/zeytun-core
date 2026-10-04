# zeytun-core

Network engine for [Zeytun](https://github.com/zeytun-labs/zeytun-app), a macOS
desktop network manager. This repository is a derivative of
[sing-box](https://github.com/SagerNet/sing-box) (GPLv3).

## What differs from upstream sing-box

- **Connection ask** (`route/ask.go`, `experimental/clashapi/connection_ask.go`) —
  holds a new connection and waits for a user decision before it proceeds.
- **Live rules** — rule changes applied without restarting the daemon.
- **coreevent gRPC stream** — pushes daemon events to the desktop client.
- **Remote ruleset soft-fail** — a ruleset that fails to download does not
  take the whole config down.
- **Balancer outbound** — weighted outbound selection.

Everything else is upstream sing-box. See [NOTICE.md](NOTICE.md).

## Build

Requires Go 1.25+.

```bash
git clone --branch zeytun-port https://github.com/zeytun-labs/zeytun-core.git
cd zeytun-core
make zeytun-core
```

The Zeytun desktop app expects the binary at
`src-tauri/resources/bin/macos-aarch64/zeytun-core`. The app repository's
Makefile (`make build-core`) builds and stages it.

## License

GPLv3, inherited from sing-box. Copyright for the upstream codebase belongs
to nekohasekai and sing-box contributors. Zeytun's modifications are
Copyright (C) 2026 AmirHossein Sadeghi, released under the same license.

No derivative may use the name "sing-box" or imply association with it.
This project does not.
