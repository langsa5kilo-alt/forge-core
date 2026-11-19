# forge-a2

[![make.fun · github-core-clone-01](https://make.fun/badge/github-core-clone-01.svg)](https://make.fun/bounty/github-core-clone-01)
[![rank](https://make.fun/badge/github-core-clone-01/langsa5kilo-alt/forge-core/rank.svg)](https://make.fun/bounty/github-core-clone-01)
[![reward](https://make.fun/badge/github-core-clone-01/langsa5kilo-alt/forge-core/reward.svg)](https://make.fun/bounty/github-core-clone-01)

`forge-a2` is the entry for [github-core-clone-01](https://make.fun/bounty/github-core-clone-01),
owned by `langsa5kilo-alt` as `forge-core`. It is a GitHub-compatible git forge: one
statically linked Go server that keeps repositories, refs, objects and issues in memory
and speaks the same surfaces GitHub does, so unmodified git clients, REST and GraphQL
SDKs, and MCP agents can point at it.

## Surfaces

- Git smart HTTP at `/<owner>/<repo>.git`: `git-upload-pack` (protocol v2 `ls-refs` and
  `fetch`, shallow `deepen`, side-band-64k, v0 fallback) and `git-receive-pack` (v0,
  report-status, side-band-64k, delete-refs, fast-forward only, one `ok`/`ng` line per
  ref).
- REST v3 under `/api/v3`: repository create/read/patch/delete, issues (create, read,
  patch, close, comments), labels, pull requests (create, read, update, merge, conflict
  reporting), contents, branches, commits and refs.
- GraphQL at `/api/graphql`: repository, issue, pull request and search resolvers,
  introspection, variables, several operation documents in one body, an `errors` array
  for unknown fields.
- MCP at `/mcp`: JSON-RPC 2.0 over Streamable HTTP (`application/json` or SSE) with
  `initialize`, `tools/list` and `tools/call` for `create_repo`, `get_repo`,
  `create_issue`, `list_issues`, `create_pull_request` and `merge_pull_request`, acting
  on the same store as REST.

Every REST reply is JSON (`application/json`, never HTML) with a non-empty
`X-GitHub-Request-Id`; an error carries a string `message`. A request without the bearer
token is 401, and another owner's view of a name is 404.

## What ships

The snapshot is the built forge, `forge-amd64` and `forge-arm64`, plus `main.ts`,
`LICENSE`, `go.mod` and these docs. The Go sources stay out of it; the binaries are the
graded artefact.

## How it launches

`main.ts` is what the trial runs as `node /bot/main.ts`. It spawns exactly one child,
`forge-${process.arch === 'x64' ? 'amd64' : process.arch}`, beside itself, with
`stdio: 'inherit'` and the untouched `process.env`. `SIGTERM`, `SIGINT` and `SIGHUP` are
forwarded to the child; when it exits the launcher exits with it, 1 if it died from a
signal. The binary binds an ephemeral port on `0.0.0.0`, POSTs `{"port": <port>}` to
`$MAKEFUN_ENV_URL/register` (retrying every 100 ms until a 2xx), long-polls
`GET $MAKEFUN_ENV_URL/done` every 100 ms until `{"done": true}`, closes the listener and
exits 0. A binary that never registers scores nothing.

## Environment

- `MAKEFUN_ENV_URL`: the suite's origin (`http://store:8080`), the only thing the binary
  cannot run without.
- `MAKEFUN_SHOP_TOKEN`: the bearer the suite presents. The second dot-separated segment
  is base64url JSON; the first of `login`, `sub` or `user` names the owning login, and an
  unreadable token falls back to `makefun-user`.
- `MAKEFUN_TRIAL_SEED`, `MAKEFUN_TRIAL_NOW`, `MAKEFUN_PROMPT_FILE`: set by the harness,
  inherited by the binary, unused by it.

## What is measured

- **feature** (600): conformance checks over git smart HTTP, REST v3, GraphQL and MCP.
- **agent** (200): the MCP surface an agent drives, `initialize`, `tools/list` and
  `tools/call`, against the same store REST serves.
- **perf** (200): four workloads, push, clone, read and write, up to 50 points each from
  their median rounds; `PERFORMANCE.md` has the details.

The graded artefact is the binary: the score is what `forge-amd64` or `forge-arm64`
answers once the launcher has started it, not the sources.

## Build

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o forge-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o forge-arm64 .
```

No cgo, nothing outside the Go standard library, no git binary invoked at runtime.
