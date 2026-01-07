# Bad.no Operations Terminal - Claude Code Context

## Project Overview

CLI tool for Bad.no's product operations team to enhance Tiger bathroom accessories with images from Tiger.nl.

**Version**: 0.2.0
**Status**: MVP Complete
**Language**: Go 1.21+

## Quick Reference

```bash
# Build
go build -o badops ./cmd/badops

# Full workflow
./badops products parse testdata/tiger-sample.csv
./badops products match
./badops images compare
./badops images fetch --new-only --limit 20
./badops images resize --size 800
```

## Documentation

| File | Purpose |
|------|---------|
| `README.md` | User-facing documentation |
| `CHANGELOG.md` | Version history and roadmap |
| `docs/PROJECT.md` | Business context from client |
| `docs/BRIEF.md` | Technical implementation details |
| `docs/TIGER-NL.md` | Tiger.nl scraping documentation |

## Architecture

```
cmd/badops/cmd/
├── root.go      - CLI setup, ASCII banner
├── products.go  - parse, match commands
└── images.go    - compare, fetch, resize commands

internal/
├── parser/matrixify.go   - CSV parsing
├── matcher/tiger.go      - Product matching
├── matcher/scraper.go    - Tiger.nl web scraper
└── images/
    ├── fetcher.go        - HTTP downloads
    └── resizer.go        - Center-crop resize

pkg/models/product.go     - Data structures
```

## Key Technical Details

### Tiger.nl Image URLs
```
https://tiger.nl/pim/528_{UUID}?width=1200&height=1200&format=jpg&quality=90
```

### State File
Products are persisted between commands in `output/.badops-state.json`

### Image Naming
- Original: `{SKU}.jpg`
- New images: `{SKU}_new_{N}.jpg`

## Common Tasks

### Add a new command
1. Create function in `cmd/badops/cmd/images.go` or `products.go`
2. Add cobra.Command variable
3. Register in `init()` function

### Add a new product series
1. Update `internal/matcher/scraper.go` → `seriesMap`
2. Update `internal/matcher/tiger.go` → `catalog`

### Change image output format
Edit `internal/images/resizer.go` → `ResizeSquare()`

## Known Issues

1. **Rate Limiting**: No delays between Tiger.nl requests
2. **Some 404s**: Not all scraped PIM URLs are valid
3. **Category Scraping**: Gets images from all products in category, not just the target product

## Next Features to Implement

1. `--dry-run` flag for all commands
2. Rate limiting (100-200ms between requests)
3. Retry logic with exponential backoff
4. Per-product detail page scraping (instead of category)

## Test Data

`testdata/tiger-sample.csv` contains 10 real bad.no Tiger products:
- CO-T309012: Boston Toilet Roll Holder
- CO-T309512: Boston Hook Black
- CO-T317312: Urban Toilet Brush
- etc.

## Dependencies

```
github.com/spf13/cobra v1.10.2
github.com/fatih/color v1.18.0
github.com/schollz/progressbar/v3 v3.19.0
github.com/olekukonko/tablewriter v0.0.5
github.com/disintegration/imaging v1.6.2
```
