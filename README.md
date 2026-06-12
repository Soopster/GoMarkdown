# GoMarkdown

`markdownviewer` is a single-binary Go TUI for reading, navigating, and editing Markdown. The app is built around Bubble Tea's MVU loop, with preview, raw-edit, outline, search, file browsing, and command-palette behavior all driven from one shared model.

## Architecture

- `main.go` holds the core model, message handling, rendering, and editing logic.
- `entrypoint.go` wires CLI parsing into the Bubble Tea program.
- `runtime.go`, `render.go`, `layout_runtime.go`, and `update_*.go` split the runtime, render, layout, and input handling paths.
- `markdown_features.go` and `mermaid.go` preprocess Markdown features before rendering.
- `internal/mermaidascii/` contains the bundled Mermaid-to-ASCII renderer.

The main interaction loop is:

1. Parse CLI flags and choose a working directory or file.
2. Load the current document and session state.
3. Run the Bubble Tea program and route key, mouse, watcher, and file events through the model.
4. Render either the preview pane, the raw editor, or a split view from the same document state.

## Install

From a checkout, build or install the current module with:

```bash
go install .
go build -o markdownviewer .
```

## Usage

```bash
markdownviewer [file-or-directory]
```

Expected CLI surface for release builds:

```bash
markdownviewer --help
markdownviewer --version
markdownviewer --no-session
markdownviewer --session-file /path/to/session.json
```

If no path is provided, `markdownviewer` opens the current working directory.

## Working Model

- Preview mode renders Markdown with theme-aware styling and custom handling for callouts, footnotes, images, math, and Mermaid blocks.
- Raw edit mode uses a text area model and keeps cursor, selection, and scroll state in sync with the preview path.
- The left pane can show files or outline entries depending on the current navigation state.
- Session state is local to the current checkout and is written to `.markdownviewer.session.json` unless disabled.
- Supported Markdown extensions are defined in `main.go` and are intentionally small and explicit.

## Developer Flow

The codebase is designed for fast iteration:

- `make build` builds the binary.
- `make test` runs the full test suite.
- `make lint` runs `golangci-lint`.
- `go test ./...` and `go vet ./...` are the main validation commands used in CI.

If you're changing behavior, start by tracing the relevant model path in `main.go`, then follow the render or input helper that owns that feature. The project favors direct control flow over deep abstraction, so most changes stay local to a small set of files.

## User Controls

- `?`: help overlay
- `e`: raw edit mode
- `p`: preview mode
- `/`, `ctrl+f`: search in the current document
- `ctrl+shift+f`: workspace search
- `ctrl+s`: save in edit mode
- `o`: toggle files and outline
- `tab`: switch pane focus
- `f`: full screen
- `T`: theme picker
- `q`: quit

The full shortcut list is available inside the application.

## Development

- Go, using the version declared in [`go.mod`](./go.mod)
- `golangci-lint` for local lint runs

```bash
make build
make test
make lint
```

## Rendering Features

- GitHub callouts render as styled panels
- footnotes render as numbered references plus a preview section
- inline and block math render in TUI-friendly form
- markdown images render as styled metadata blocks
- fenced `mermaid` blocks render directly in the TUI
- supported diagram families currently follow the bundled `mermaid-ascii` renderer
- unsupported Mermaid syntax falls back to a styled textual block with the parse/render error

## Release Notes

This repository includes:

- GitHub Actions CI under `.github/workflows/`
- a GoReleaser configuration for tagged builds
- a permissive MIT license
- a changelog for project history

Publishing binaries or tags is intentionally left as a separate step.

## Project Notes

- Session state is local-only and should not be committed.
- AI-generated planning/history documents should live under `history/` rather than the repository root.
