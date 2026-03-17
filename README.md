# carry-on

> Born to route. Forged in localhost.

A reverse proxy dev server that forwards your requests with the fury of a thousand metal warriors. Kills CORS errors dead. Also supports WebSockets.

> **For local development only.** carry-on is a dev tool — it injects permissive CORS headers on everything and is not hardened for production use.

## Install

```sh
go install github.com/amir/carry-on@latest
```

Or grab a binary from [releases](https://github.com/amir/carry-on/releases).

## Usage

```sh
carry-on "/api@5000" 5713
```

Listens on `:1987`. Routes `/api/*` to `localhost:5000` (stripping the prefix). Everything else goes to `localhost:5713`.

```
carry-on  ▶  :1987
  /api           →  localhost:5000  (strip)
  *              →  localhost:5713  (fallback)
```

Override the listening port with `PORT`:

```sh
PORT=8080 carry-on "/api@5000" 5713
```

Multiple routes:

```sh
carry-on "/api@5000" "/ws@5001" 5713
```

Keep the prefix instead of stripping it:

```sh
carry-on --no-strip "/api@5000" 5713
```

## Config file

```sh
carry-on init        # generates carry-on.toml in the current directory
carry-on             # auto-loads carry-on.toml if present
carry-on -c proxy.toml
```

```toml
# carry-on.toml
# port = 1987

fallback = "localhost:3000"

[[route]]
path = "/api"
target = "localhost:8080"
# strip = true

[[route]]
path = "/ws"
target = "localhost:8081"
strip = false
```

CLI args take precedence over the config file. `PORT` env always wins.

## CORS

carry-on injects CORS headers on every response by default — that's the whole point. No configuration needed. To disable:

```sh
carry-on --no-cors "/api@5000" 5713
```

Or in `carry-on.toml`:

```toml
cors = false
```

## WebSockets

They just work. carry-on tunnels WebSocket upgrades transparently to the target, stripping prefixes as configured.

## Flags

| Flag | Description |
|------|-------------|
| `--no-cors` | Disable CORS header injection |
| `--no-strip` | Don't strip path prefix when forwarding (CLI routes only) |
| `-c`, `--config` | Path to config file |

## Route syntax

```
/path@target
```

`target` can be a port number (`5000`), a host:port (`localhost:5000`), or a full URL (`http://localhost:5000`).
