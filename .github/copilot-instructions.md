# GitHub Copilot Instructions for markdownviewer

## Project Overview

`markdownviewer` is a terminal-based Markdown viewer and editor written in Go. The project focuses on a responsive terminal UI, high-quality Markdown rendering, and keyboard-driven editing/navigation for local documentation workflows.

## Tech Stack

- Language: Go
- UI: Bubble Tea, Bubbles, Lip Gloss
- Markdown rendering: Glamour
- File watching: fsnotify
- Testing: Go standard testing package
- CI: GitHub Actions

## Working Rules

- Prefer small, reviewable changes with tests when behavior changes.
- Preserve user-visible keyboard workflows unless the task explicitly calls for UX changes.
- Keep public repository files clean and intentional; store ephemeral planning material in `history/`.

## Code Quality

- Run `gofmt`, `go test ./...`, and `go vet ./...` before finalizing changes.
- Run `golangci-lint run` when editing Go code.
- Avoid introducing new dependencies without a clear need.
- Prefer clear package boundaries over continuing to grow a single large file.

## Documentation Expectations

- Update `README.md` when installation, CLI behavior, or key features change.
- Update `CHANGELOG.md` for user-visible changes.
- Keep release-related files aligned with the actual build and install path.
