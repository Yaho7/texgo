# texgo

[简体中文](./README.zh.md)

`texgo` is a small Go CLI for building LaTeX projects from the terminal. It reduces repeated LaTeX compilation overhead by caching converted figure PDFs, detects the main `.tex` file, and delegates compilation to `latexmk`.

## Features

- Builds a LaTeX project with a single `texgo` command.
- Shortens repeated compilation by converting supported figures once and reusing cached PDFs.
- Detects common main files such as `main.tex`, `paper.tex`, `manuscript.tex`, and `thesis.tex`.
- Parses local `\includegraphics{...}` references to discover figure directories.
- Converts supported images to cached PDFs through GraphicsMagick, running conversions concurrently.
- Stores project preferences in `.texgo.conf`.
- Produces a single Go binary for distribution.

## Requirements

Runtime dependencies:

- `latexmk`
- A LaTeX engine supported by `latexmk`: `xelatex`, `pdflatex`, or `lualatex`
- `gm` from GraphicsMagick, a required dependency for the accelerated image conversion workflow

Source build dependency:

- Go, required only when building from source

## Installation

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/Yaho7/texgo/main/install.sh | bash -s -- --yes
```

## Quick Start

Configure a project once:

```bash
texgo setup
```

Build the project:

```bash
texgo
```

`texgo` without arguments is equivalent to `texgo build` when run inside a LaTeX project.

## Commands

```bash
texgo                    # Build the current project
texgo setup              # Create or update .texgo.conf interactively
texgo build              # Build the current project
texgo build main.tex     # Build a specific TeX file
texgo images             # Convert and cache image files only
texgo images --workers 8 # Convert images with up to eight concurrent jobs
texgo clean              # Remove the build directory
texgo clean --figures    # Remove build output and cached figure PDFs
texgo doctor             # Check required external commands
texgo init my-paper      # Create a standalone article starter project
```

`texgo init` creates `template/manuscript.tex`, `template/bibliography/references.bib`, and an embedded `template/figures/logo.png` source image. The template includes the project URL in its footer and replaceable contact lines for `i@yaho7.cn` and `yaho7.cn`; running `texgo build` converts the bundled logo through GraphicsMagick before compiling the manuscript.

Common options:

```text
--project-dir DIR
--figures-dir DIR
--pdf-dir DIR
--build-dir DIR
--tex-file FILE
--engine xelatex|pdflatex|lualatex
--no-images
--workers N
```

## Configuration

`texgo setup` writes `.texgo.conf` in the project root.

Example:

```conf
tex_file=paper.tex
engine=xelatex
build_dir=build
figures_dir=
convert_images=1
```

Precedence:

```text
command-line options > .texgo.conf > auto-detected defaults
```

## Image Conversion

When image conversion is enabled, `texgo` converts supported files in figure directories to PDFs and stores them under a `pdf/` subdirectory. It runs up to four `gm convert` jobs concurrently by default; pass `--workers N` to `texgo build` or `texgo images` to tune that limit. This avoids forcing LaTeX to repeatedly process large raster/vector assets during later builds, which can substantially reduce compilation time for figure-heavy papers, theses, and reports.

Supported source extensions:

```text
png, jpg, jpeg, gif, tif, tiff, bmp, svg
```

Cached PDFs are refreshed when the source image is newer. Stale cached PDFs are removed when their source image no longer exists.

## Development

Run tests:

```bash
go test ./...
bash tests/install_release_test.sh
```

Build a local binary:

```bash
./scripts/build-binary.sh --output /tmp/texgo
```

Project layout:

```text
cmd/texgo/              Go CLI source and tests
cmd/texgo/template-assets/ Embedded assets copied by texgo init
.github/workflows/      Multi-platform release builds
scripts/                Build helper scripts
install.sh              Release installer and source build helper
```
