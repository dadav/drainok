# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`drainok` is a read-only Go CLI that reports which Kubernetes nodes could be drained right now. Each node is evaluated independently ("could THIS node drain, all others staying up?") against a set of checks; a node is drainable only if every check returns zero blockers. Exit codes are part of the contract: 0 all drainable, 1 at least one blocked, 2 analysis error.

## Commands

```sh
just build              # go build -o drainok .
just test               # go test ./...
just lint               # go vet + golangci-lint if installed
just fmt                # gofmt -w .
just run -- worker-1    # go run against the current kube context
just snapshot           # local goreleaser snapshot build
just kind-up/kind-down  # 3-node kind test cluster from kind-config.yaml

go test ./internal/checks -run TestPDBBlocksAtZeroDisruptions   # single test
```

Local goreleaser runs need at least one git commit and a configured remote; CI (`.github/workflows/`) is the authority for the full release matrix.

## Architecture

The pipeline is strictly linear: `cmd/root.go` (cobra + viper) -> `internal/kube` (client + snapshot) -> `internal/analyzer` -> `internal/output`.

The load-bearing design decision: `kube.FetchSnapshot` does ALL API reads up front into a `ClusterSnapshot` (nodes, pods indexed by node, PDBs, PVCs/PVs). Everything downstream is a pure function over that struct. This is why checks are deterministic and unit-testable with hand-built snapshots (see `internal/checks/helpers_test.go`) — preserve this property; never make a check call the API.

Checks implement the two-method `checks.Check` interface and are registered in `checks.All()` (`internal/checks/checks.go`). One file per check. To add a check: new file with the type, append to `All()`, add a focused `_test.go`, mention it in the README table. `Blocker.Check` is the user-facing blocker kind and usually equals `Name()`, except `RescheduleCheck` ("reschedule") which emits two kinds: `fit` (no capacity) and `constraints` (no node matches). `--ignore-checks` matches `Name()`, not blocker kinds.

Pod eligibility lives in one place: `snapshot.EvictablePods()` excludes DaemonSet, mirror, and terminal pods — mirroring what `kubectl drain` would actually evict. Checks must go through it rather than reading `PodsByNode` directly (only `RescheduleCheck.buildTargets` reads raw pods, deliberately, to count resource usage on target nodes).

Scheduler fidelity comes from `k8s.io/component-helpers` (same code the kube-scheduler uses) for node affinity matching, taint toleration, and pod request math — do not hand-roll these. Known, documented limitations (README "Known limitations"): anti-affinity is hostname-level only, only CPU/memory/pod-count are simulated, `NamespaceSelector` in anti-affinity terms conservatively matches all namespaces. When trading off accuracy, err toward reporting "not drainable" (false blockers beat false green lights for a pre-drain check).

## Conventions

- Version info is injected by goreleaser into `main.version/commit/date` ldflags and passed through `cmd.SetVersionInfo`; `.goreleaser.yaml` and `main.go` must stay in sync on those names.
- New flags: define in `cmd/root.go` init(), add to the viper `BindPFlag` loop (env `DRAINOK_*` works via the `-` to `_` replacer), read via `viper.Get*` in `run()`, document in the README flags table.
- `cmd.Execute()` maps errors to exit codes; "not drainable" is the sentinel `cmd.ErrNotDrainable`, not an os.Exit call deep in the stack.
- Tests build fixtures with the `testNode`/`testPod`/`testSnapshot` helpers in `internal/checks/helpers_test.go`; the fake clientset is only used for `FetchSnapshot` itself.
