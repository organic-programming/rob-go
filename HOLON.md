---
# Holon Identity v1
uuid: "d4e5f6a7-8b9c-0d1e-2f3a-4b5c6d7e8f9a"
given_name: Rob
family_name: Go
motto: "Build anything, test everything."
composer: "B. ALTER"
clade: "deterministic/toolchain"
status: draft
born: "2026-03-02"

# Lineage
parents: []
reproduction: "assisted"

# Metadata
generated_by: "codex"
proto_status: defined
---

# Rob Go

> *"Build anything, test everything."*

## Description

The foundational toolchain holon of Organic Programming. Rob wraps the
`go` command AND Go's standard analysis packages, exposing the full
power of the Go ecosystem as gRPC RPCs.

Two execution modes:
- **Exec**: subprocess calls for orchestration (build, test, run, mod, ...)
- **Library**: in-process calls for analysis (parse, type-check, format, vet, ...)

Every OP holon that works with Go code goes through Rob.

## Contract

- Proto: `protos/go/v1/go.proto`
- Service: `go.v1.GoService`
- Transport: `stdio://` (default), `tcp://`, `unix://`

## Technical Notes

- Exec mode: `os/exec.Command("go", ...)` — requires Go on `$PATH`.
- Library mode: `go/parser`, `go/types`, `go/format`,
  `golang.org/x/tools/go/packages`, `golang.org/x/tools/go/analysis`.
- All RPCs accept `workdir` (defaults to `.`).
- gRPC reflection enabled — compatible with `op grpc://` dispatch.
