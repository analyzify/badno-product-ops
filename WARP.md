# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project overview

Bad.no Operations Terminal (`badops`) is a Go 1.21+ CLI tool for Bad.no's product operations team. It enhances Tiger bathroom accessory products by:
- Parsing Shopify Matrixify CSV exports for Tiger products
- Matching products against Tiger.nl's catalog
- Comparing image counts between bad.no and Tiger.nl
- Downloading missing/new images from Tiger.nl
- Resizing images into square formats suitable for Shopify

## Key commands

All commands are run from the repository root.

### Build and install

```bash
# Build local binary
go build -o badops ./cmd/badops

# Install into GOPATH/bin
go install ./cmd/badops
```

The built `badops` binary is the entrypoint used in the examples below (you can also run `./badops` directly from the repo root).

### Core CLI workflow

```bash
# 1. Parse a Matrixify CSV export from Shopify
./badops products parse path/to/tiger-export.csv

# 2. Match parsed products against Tiger.nl
./badops products match

# 3. Compare image counts (bad.no vs Tiger.nl)
./badops images compare

# 4. Download only NEW images not already on bad.no
./badops images fetch --new-only --limit 20

# 5. Resize images for Shopify
./badops images resize --size 800
```

These commands operate on shared state persisted under the `output/` directory (see "Architecture and data flow").

### Tests and linting

There is no project-specific test or lint configuration checked into this repo.

Go-standard commands that apply here:

```bash
# Run all Go tests (if/when tests are added)
go test ./...

# Run tests in a specific package
go test ./internal/parser

# Run a single test by name pattern
go test ./internal/parser -run TestName

# Basic static analysis using the Go toolchain
go vet ./...
```

Use these commands as needed when adding new code or tests.

## Architecture and data flow

### High-level layout

- `cmd/badops/main.go`
  - Program entrypoint. Initializes the Cobra root command and wires subcommands.
- `cmd/badops/cmd/`
  - `root.go`: Cobra root command, global flags, ASCII banner, and wiring of subcommands.
  - `products.go`: Implements `products` subcommands (`parse`, `match`) and orchestrates parsing and matching workflows.
  - `images.go`: Implements `images` subcommands (`compare`, `fetch`, `resize`) and orchestrates image comparison, download, and resize workflows.
- `pkg/models/product.go`
  - Core domain types for products and related metadata (e.g., SKU, title, vendor, image counts, match status).
- `internal/parser/matrixify.go`
  - Responsible for parsing Shopify Matrixify CSV exports.
  - Produces in-memory product models and writes persistent state.
- `internal/matcher/tiger.go`
  - Encapsulates product matching logic between bad.no products and Tiger.nl catalog entries.
  - Uses series- and SKU-based heuristics and confidence scoring to classify matches (`matched`, `review`, `no match`).
- `internal/matcher/scraper.go`
  - Handles Tiger.nl scraping and catalog fetching.
  - Encodes knowledge of Tiger.nl URL patterns and PIM image endpoints.
  - Maintains mappings like `seriesMap` used for product series handling.
- `internal/images/fetcher.go`
  - Handles HTTP downloads of images from Tiger.nl using URLs constructed from the matcher/scraper layer.
  - Writes original images into `output/originals/` following the naming scheme described below.
- `internal/images/resizer.go`
  - Implements center-crop square resizing using `github.com/disintegration/imaging`.
  - Reads from `output/originals/` and writes to `output/resized/{size}/`.

### Persistent state and outputs

All commands share state and outputs under `output/`:

- `output/.badops-state.json`
  - Internal state file written by `products parse`.
  - Holds parsed products and intermediate matching data used by subsequent commands.
- `output/report.json`
  - Match results and statistics produced by `products match`.
- `output/originals/`
  - Original and newly downloaded images from Tiger.nl.
  - Naming convention:
    - Original: `{SKU}.jpg`
    - New images: `{SKU}_new_{N}.jpg`
- `output/resized/{size}/`
  - Resized square images for a given pixel size (e.g., `800`, `2000`).

The typical data flow is:
1. `products parse` parses the Matrixify CSV into product models and writes `.badops-state.json`.
2. `products match` enriches those products using Tiger.nl via the matcher/scraper packages and writes `report.json`.
3. `images compare` reads the state and report to compute image deltas between bad.no and Tiger.nl.
4. `images fetch` uses the comparison results plus Tiger.nl image URLs to download missing images into `output/originals/`.
5. `images resize` converts downloaded originals into square thumbnails in `output/resized/{size}/`.

## How Warp should extend or modify this project

### Adding or changing CLI commands

- New top-level or subcommands should be implemented under `cmd/badops/cmd/` using Cobra.
- Pattern for adding a new command:
  1. Implement the command handler function in the appropriate file (`images.go` or `products.go`, or a new file if it is logically distinct).
  2. Define a `*cobra.Command` with flags and help text.
  3. Register the command in an `init()` function so it is attached to the root or a parent command.
- Keep commands thin: delegate parsing, matching, scraping, and image processing to the `internal/` and `pkg/` packages rather than doing heavy logic in the CLI layer.

### Working with product matching and Tiger.nl

- Product series and Tiger.nl catalog specifics live primarily in:
  - `internal/matcher/scraper.go` → series/category mappings and scraping logic.
  - `internal/matcher/tiger.go` → in-memory catalog representation and matching heuristics.
- When adding support for a new product series or adjusting matching rules:
  - Update `seriesMap` (or equivalent) in `scraper.go` so the scraper can discover the relevant Tiger.nl pages.
  - Update catalog/matching logic in `tiger.go` so confidence scores and classifications remain meaningful.
- Tiger.nl image URL patterns are documented in `docs/TIGER-NL.md` and mirrored in the scraper/matcher code; refer to both when modifying URL construction.

### Working with images

- Download behavior and HTTP concerns (timeouts, rate limiting, retries) belong in `internal/images/fetcher.go`.
- Image transformations and output formats belong in `internal/images/resizer.go`:
  - To change the resize behavior (e.g., padding vs. center-crop), update the relevant functions here.
  - To support new output sizes or formats, extend this package and keep naming consistent with existing conventions.

### Respecting known limitations and roadmap

From existing documentation (README, CLAUDE rules):

- Current limitations include:
  - No built-in rate limiting for Tiger.nl requests.
  - Some scraped PIM URLs return 404s.
  - Category-level scraping can pull images from related products, not just the target SKU.
  - Image comparisons are count-based only (no visual deduplication).
- Planned or suggested improvements include:
  - `--dry-run` flag for all commands.
  - Rate limiting (e.g., 100–200ms between requests) and retry logic with backoff.
  - More precise per-product detail page scraping.
  - Visual deduplication (perceptual hashing) and support for additional suppliers.

When implementing features in these areas, prefer to:
- Add network-oriented behavior (rate limiting, retries) to the scraper/fetcher layers.
- Keep CLI flags thin shims over reusable internal package behavior.

## Relevant documentation files

Warp should consult these files for deeper context before making non-trivial changes:

- `README.md`
  - User-facing overview, core workflows, and example commands.
- `CLAUDE.md`
  - Concise architecture summary, common tasks (e.g., how to add commands, update series), known issues, and next features.
- `CHANGELOG.md`
  - Version history and high-level roadmap (when present).
- `docs/PROJECT.md`
  - Business context from the client.
- `docs/BRIEF.md`
  - Technical implementation details and constraints.
- `docs/TIGER-NL.md`
  - Details of Tiger.nl integration, URL patterns, scraping methodology, and known limitations.
- `testdata/tiger-sample.csv`
  - Realistic sample data for local experiments with the CLI.
