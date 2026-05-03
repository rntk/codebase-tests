# Architecture Decisions

## Backend Language: Go

Rationale: mature LSP client libraries, easy to ship as a single binary per project, simple plugin model.

## Frontend: React + Vite + TypeScript

Rationale: fast dev server, modern toolchain, strong typing for API contract alignment.

## Plugin Transport: In-Process Go Interface

Rationale: v1 prioritizes simplicity. The interface is designed so it can be swapped to gRPC later for out-of-process plugins without changing consumer code.

## Storage: On-Disk JSON

Per-project configuration lives in `.review/config.json`. No database in v1.
