# Changelog

All notable changes to the Bad.no Operations Terminal (badops) project.

## [0.2.0] - 2025-01-07

### Added
- **Image Comparison Feature** (`badops images compare`)
  - Compares image counts between bad.no and Tiger.nl
  - Shows table with: SKU, Product Name, Bad.no count, Tiger.nl count, New count
  - Identifies products missing images on bad.no

- **New-Only Fetch** (`badops images fetch --new-only`)
  - Downloads only images not already on bad.no
  - Scrapes Tiger.nl product pages in real-time
  - Names new images as `{SKU}_new_{N}.jpg` for easy identification
  - Supports `--limit` flag to control batch size

- **Tiger.nl Web Scraper** (`internal/matcher/scraper.go`)
  - Scrapes Tiger.nl product category pages
  - Extracts all PIM image URLs from product listings
  - Maps bad.no product names to Tiger.nl categories
  - Supports series filtering (Boston, Urban, 2-Store, Carv, etc.)

### Changed
- Updated test data with real bad.no product SKUs (CO-T309012, etc.)
- Fetcher now uses real Tiger.nl PIM image URLs instead of Unsplash placeholders

### Technical Details
- Tiger.nl PIM image URL format: `https://tiger.nl/pim/528_{UUID}?width=1200&height=1200&format=jpg&quality=90`
- Scraper extracts ~19 images per product page (product shots, lifestyle, packaging)

---

## [0.1.0] - 2025-01-07

### Added
- **Initial CLI Structure**
  - Cobra-based CLI with ASCII banner
  - Colored output using fatih/color
  - Progress bars using schollz/progressbar

- **Products Commands**
  - `badops products parse <csv>` - Parse Matrixify CSV exports
  - `badops products match` - Match products against Tiger.nl (simulated)
  - State persistence between commands (`output/.badops-state.json`)
  - JSON report generation (`output/report.json`)

- **Images Commands**
  - `badops images fetch` - Download images from Tiger.nl
  - `badops images resize` - Center-crop resize to square format
  - Support for `--limit` and `--size` flags

- **Core Packages**
  - `internal/parser/matrixify.go` - CSV parsing for Shopify exports
  - `internal/matcher/tiger.go` - Product matching with confidence scores
  - `internal/images/fetcher.go` - HTTP image downloader
  - `internal/images/resizer.go` - Center-crop resize using imaging library

- **Data Models**
  - Product struct with SKU, Name, Brand, ExistingImages, MatchedURL, MatchScore
  - Image struct with SourceURL, OriginalPath, ResizedPaths, Status
  - Report struct for JSON export

### Dependencies
- github.com/spf13/cobra v1.10.2
- github.com/fatih/color v1.18.0
- github.com/schollz/progressbar/v3 v3.19.0
- github.com/olekukonko/tablewriter v0.0.5
- github.com/disintegration/imaging v1.6.2

---

## Roadmap

### [0.3.0] - Planned
- [ ] Rate limiting for Tiger.nl requests
- [ ] Retry logic for failed downloads
- [ ] `--dry-run` flag for all commands
- [ ] Better error handling and logging

### [0.4.0] - Planned
- [ ] Visual image deduplication (perceptual hashing)
- [ ] Manual review queue for low-confidence matches
- [ ] Config file support (~/.badops.yaml)

### [1.0.0] - Future
- [ ] Direct Shopify API integration
- [ ] Support for additional suppliers
- [ ] Web UI for manual review
- [ ] Automated scheduling/cron support
