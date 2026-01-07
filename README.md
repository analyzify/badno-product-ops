# Bad.no Operations Terminal (badops)

CLI tool for Bad.no's product operations team to enhance product images by fetching high-quality photos from supplier catalogs.

## Overview

Bad.no sells Tiger bathroom accessories. This tool automates the process of:
1. Parsing product exports from Shopify (Matrixify format)
2. Matching products against Tiger.nl's official catalog
3. Comparing image counts (bad.no vs Tiger.nl)
4. Downloading missing/new images from Tiger.nl
5. Resizing images to square format for Shopify

## Installation

```bash
# Clone and build
cd /Users/ermankuplu/Building/bad-no-ops
go build -o badops ./cmd/badops

# Or install globally
go install ./cmd/badops
```

## Quick Start

```bash
# 1. Parse your Matrixify export
./badops products parse testdata/tiger-sample.csv

# 2. Match products against Tiger.nl
./badops products match

# 3. Compare image counts (bad.no vs Tiger.nl)
./badops images compare

# 4. Download only NEW images (not already on bad.no)
./badops images fetch --new-only --limit 20

# 5. Resize to square format
./badops images resize --size 800
```

## Commands

### Products

#### `badops products parse <csv-file>`
Parse a Matrixify CSV export from Shopify.

```bash
./badops products parse exports/tiger-products.csv
```

**Input CSV format:**
```csv
Handle,Title,Vendor,Image Src
CO-T309012,Tiger Boston Toalettrullholder RVS,Tiger,https://cdn.shopify.com/...
```

**Output:**
- Displays product table with SKU, name, and image count
- Shows products missing images (highlighted in red)
- Saves state to `output/.badops-state.json` for next commands

#### `badops products match`
Match parsed products against Tiger.nl catalog.

```bash
./badops products match
```

**Output:**
- Match confidence scores (0-100%)
- Status: `matched` (>70%), `review` (50-70%), `no match` (<50%)
- Saves report to `output/report.json`

### Images

#### `badops images compare`
Compare image counts between bad.no and Tiger.nl.

```bash
./badops images compare
```

**Output:**
```
SKU        | Product                    | Bad.no | Tiger.nl | New
-----------+----------------------------+--------+----------+-----
CO-T309012 | Tiger Boston Toalettrul... |      1 |       19 | +18
CO-T309512 | Tiger Boston krok matt...  |      0 |       19 | +19
```

#### `badops images fetch`
Download images from Tiger.nl.

```bash
# Download demo images (hardcoded URLs)
./badops images fetch --limit 5

# Download only NEW images not on bad.no
./badops images fetch --new-only --limit 20
```

**Flags:**
- `--limit, -l` - Limit number of images to download (0 = all)
- `--new-only` - Only download images not already on bad.no

**Output files:**
- `output/originals/{SKU}.jpg` - Original product image
- `output/originals/{SKU}_new_{N}.jpg` - New images from Tiger.nl

#### `badops images resize`
Resize images to square format with center-crop.

```bash
./badops images resize --size 800
./badops images resize --size 2000
```

**Flags:**
- `--size, -s` - Target size in pixels (default: 800)

**Output:**
- `output/resized/{size}/{filename}.jpg`

## Output Structure

```
output/
├── originals/              # Downloaded full-size images
│   ├── CO-T309012.jpg      # Original product image
│   ├── CO-T309012_new_1.jpg # New image #1 from Tiger.nl
│   ├── CO-T309012_new_2.jpg # New image #2 from Tiger.nl
│   └── ...
├── resized/
│   └── 800/                # Resized to 800x800
│       ├── CO-T309012.jpg
│       └── ...
├── report.json             # Match results and statistics
└── .badops-state.json      # Internal state between commands
```

## Architecture

```
bad-no-ops/
├── cmd/badops/
│   ├── main.go             # Entry point
│   └── cmd/
│       ├── root.go         # Root command with ASCII banner
│       ├── products.go     # products parse/match commands
│       └── images.go       # images compare/fetch/resize commands
├── internal/
│   ├── parser/
│   │   └── matrixify.go    # CSV parser for Shopify exports
│   ├── matcher/
│   │   ├── tiger.go        # Product matching logic
│   │   └── scraper.go      # Tiger.nl web scraper
│   └── images/
│       ├── fetcher.go      # Image downloader
│       └── resizer.go      # Center-crop resize logic
├── pkg/models/
│   └── product.go          # Data structures
├── testdata/
│   └── tiger-sample.csv    # Sample test data
└── output/                 # Generated files
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/fatih/color` | Colored terminal output |
| `github.com/schollz/progressbar/v3` | Progress bars |
| `github.com/olekukonko/tablewriter` | ASCII tables |
| `github.com/disintegration/imaging` | Image processing |

## Configuration

Currently all configuration is via command-line flags. Future versions may support:
- Config file (`~/.badops.yaml`)
- Environment variables
- Rate limiting settings

## Tiger.nl Integration

See [docs/TIGER-NL.md](docs/TIGER-NL.md) for detailed documentation on:
- URL structure and patterns
- Image API endpoints
- Scraping methodology
- Known limitations

## Workflow Example

### Full workflow for updating Tiger product images:

```bash
# 1. Export Tiger products from Shopify using Matrixify
#    Filter: Vendor = "Tiger"
#    Download as CSV

# 2. Parse the export
./badops products parse ~/Downloads/tiger-export.csv
# Output: "Parsed 179 products, 45 missing images"

# 3. Match against Tiger.nl
./badops products match
# Output: "Matched 156/179 products (87%)"

# 4. Compare image counts
./badops images compare
# Output: "Found 892 new images across 179 products"

# 5. Download new images (in batches)
./badops images fetch --new-only --limit 50
# Output: "Downloaded 47 NEW images to output/originals/"

# 6. Resize for Shopify
./badops images resize --size 800
# Output: "Resized 47 images to output/resized/800/"

# 7. Upload to Shopify via Matrixify or manual upload
```

## Known Limitations

1. **Rate Limiting**: No built-in rate limiting for Tiger.nl requests
2. **Image Deduplication**: Compares by count, not by visual similarity
3. **Series Matching**: Some product series (2-Store, Carv) have lower match rates
4. **Manual Review**: Products with <70% match confidence need manual verification

## Future Improvements

- [ ] Add `--dry-run` flag to preview without downloading
- [ ] Implement visual image deduplication (perceptual hashing)
- [ ] Add rate limiting and retry logic
- [ ] Support for other suppliers (not just Tiger.nl)
- [ ] Direct Shopify upload via API
- [ ] Web UI for manual review queue

## License

Internal tool for Bad.no AS. Not for public distribution.
