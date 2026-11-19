# Performance

The perf column is 200 points: four workloads, up to 50 each. A workload's p50 is the
median of three timed rounds after the untimed warm-up, and its points are
`round(clamp(25 - 12.5*log2(p50_ms / T_ref_ms), 0, 50))`. A workload that throws or is
cut by its 60 s budget scores 0, whatever rounds it completed.

## The four workloads

- **push**: 200 sequential receive-pack exchanges (advertisement, command list, pack),
  each a new commit on `refs/heads/main` with a 1 KiB blob change. The client's objects
  and payloads are built before timing, so neither hashing nor deflating is timed.
- **clone**: 50 concurrent shallow (depth 1) upload-pack fetches of a 200-commit
  repository; each verifies its pack checksum and does not parse the objects.
- **read**: 2,000 sequential `GET /api/v3/repos/{owner}/{repo}/issues/{number}` over one
  warm keep-alive connection.
- **write**: 200 sequential `POST /api/v3/repos/{owner}/{repo}/issues`.

Setup (creating the repository, pushing the clone history) runs inside each workload's
budget but outside every timed round.

## What survived

- **Immutable clone pack behind an atomic pointer** (`clonepack.go`). The depth-1,
  single-want fast path builds the pack once, under a mutex and a repository read lock,
  frames it as a v2 fetch answer, and stores it in an `atomic.Pointer[[]byte]`; later
  clones return those bytes with no repository lock. zlib readers are pooled and reset per
  object (`packin.go`).
- **Precomputed issue responses** (`issues.go`, `reply.go`, `hotroute.go`). Filing an issue
  frames its 200 body once into `Ticket.Response` and keeps the JSON in `Cached`. A read
  is a byte-level route match, a map lookup and a slice return, with no serialization on
  the hot path.
- **Batched writes** (`issues.go`). A create takes the repository lock once, appends the
  issue and republishes the whole issue table behind one atomic pointer in that same
  acquisition. The created issue's JSON is marshalled once and the 201 is framed fresh per
  call.

## Rebuild

The two committed binaries are what gets graded; rebuild both after any change, from the
repository root:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o forge-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o forge-arm64 .
```

## Grade

From the kit directory:

```sh
node cli/makefun.mjs grade-local --kit . --bot /home/ubuntu/agents/brisk/projects/forge-claude \
  --worktree --driver process
```

`--worktree` grades the files as they are on disk rather than a commit, and
`--driver process` runs the launcher with `node22` on this machine, unsandboxed. The
report prints each workload's p50 and points and the perf total.
