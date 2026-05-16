# claude-usage

A small macOS CLI that shows your Claude Code subscription usage and extra-usage budget.

It reads the OAuth access token that the `claude` CLI stores in the macOS Keychain and calls the Anthropic usage endpoint, then prints a compact summary of each rate-limit window (5-hour, 7-day, per-model) and your extra-usage spend.

## Requirements

- macOS (uses the Keychain via `security find-generic-password`)
- Go 1.22+
- A working install of [Claude Code](https://claude.com/claude-code) — you must have run `claude login` so credentials exist in the Keychain

## Install

```sh
git clone https://github.com/rlshepherd/claude-usage.git
cd claude-usage
go build -o claude-usage .
```

Move the resulting `claude-usage` binary somewhere on your `PATH` (e.g. `/usr/local/bin`).

## Usage

```sh
claude-usage           # human-readable summary
claude-usage -json     # parsed JSON
claude-usage -raw      # raw server response
```

Example output:

```
5-hour:                 12.3% used  (87.7% left, resets in 2h15m)
7-day:                  41.0% used  (59.0% left, resets in 4d)
7-day (opus):           58.2% used  (41.8% left, resets in 4d)
7-day (sonnet):         22.1% used  (77.9% left, resets in 4d)

extra usage: $3.40 / $40.00 USD used  ($36.60 remaining, 8.5% of monthly limit)
```

## How it works

1. Looks up the credential blob in the Keychain under service `Claude Code-credentials`.
2. Extracts the OAuth `accessToken`.
3. `GET https://api.anthropic.com/api/oauth/usage` with that bearer token.
4. Pretty-prints the result.

No credentials are written, transmitted anywhere other than Anthropic's API, or cached on disk.

## Troubleshooting

- **`no Claude Code credentials in Keychain`** — run `claude login` first.
- **`token rejected`** — your token has expired; re-run `claude login`.
- **non-macOS** — this tool currently only supports macOS because it depends on `security`. PRs welcome.
