# forge-a2

[![make.fun · github-core-clone-01](https://make.fun/badge/github-core-clone-01.svg)](https://make.fun/bounty/github-core-clone-01?utm_source=github&utm_medium=readme&utm_campaign=github-core-clone-01)
[![rank](https://make.fun/badge/github-core-clone-01/langsa5kilo-alt/forge-a2/rank.svg)](https://make.fun/bounty/github-core-clone-01/leaderboard?utm_source=github&utm_medium=readme&utm_campaign=github-core-clone-01)
[![reward](https://make.fun/badge/github-core-clone-01/langsa5kilo-alt/forge-a2/reward.svg)](https://make.fun/bounty/github-core-clone-01?utm_source=github&utm_medium=readme&utm_campaign=github-core-clone-01)

forge-a2 is a compact, memory-resident GitHub-compatible forge written in Go. A tiny Node 22 launcher selects the static binary for the host architecture. The server implements REST v3, GraphQL, MCP, and Git smart HTTP without invoking Git or using third-party packages.

Build both committed executables from the repository root:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o forge-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o forge-arm64 .
```

Source is split by responsibility: `types.go`/`store.go` hold the model and forge lifecycle, `transport.go`/`codec.go` the HTTP loop and reply framing, `dispatch.go` the routing, `rest.go`/`contents.go`/`issues.go`/`pulls.go` the REST v3 API, `graphql.go` and `mcp.go` their surfaces, and `ghhttp.go`/`ingest.go`/`serve.go`/`packin.go`/`packout.go`/`objects.go`/`gitwire.go` the git smart HTTP and pack paths.

The benchmark exercises sequential pushes, concurrent shallow clones, repeated issue reads on one connection, and issue creation. Repository state and Git objects stay in memory; response-ready issue JSON and generated pack responses are cached on hot paths.

The official local suite passes all 120 conformance checks, scoring feature 600 and agent-native 200. The best local performance run scored 170 points with medians of 396.498 ms for push, 162.739 ms for clone, 985.281 ms for read, and 138.175 ms for write. Its total score was 970.
