# Changelog

All notable changes to the Bad.no Operations Terminal (badops) project.

## [1.2.0] - 2026-01-18

### Added

#### Enhanced NOBB Connector - Complete Data Extraction

Major update to the NOBB connector (`internal/source/nobb/connector.go`) to extract ALL available data from the NOBB API.

**New Data Extracted:**

- **Images/Media** - Product images, documentation, assembly instructions from NOBB CDN
  - Media types: PB (Product Image), FDV (Documentation), MTG (Assembly), MB (Environment), TEG (Technical Drawing)
  - URLs: `https://cdn.byggtjeneste.no/nobb/{guid}/square`

- **ETIM Properties** - Full technical specifications
  - Material, color, form, dimensions
  - Mounting method, surface treatment
  - All ETIM classification properties

- **Environmental Properties** - Compliance and sustainability data
  - BREEAM-NOR A20 2016 compliance
  - BREEAM-NOR Hea 02 2016 (VOC emissions)
  - EPD (Environmental Product Declaration) data

- **Marketing Properties** - Product benefits and features
  - Produktfordel 1, 2, 3... (Product benefits)
  - Marketing descriptions

- **Extended Package Information** - Complete logistics data
  - Volume (liters)
  - Minimum order quantity
  - Deliverable status (per supplier)
  - Stocked status (per supplier)
  - Calculated count
  - Consists of count/unit (items per package)
  - Dangerous goods flag and UN number

- **Additional Item Fields**
  - Customs code (Norwegian and EU)
  - ETIM class code
  - Manufacturer item number
  - NRF (Norwegian Retail Federation) info
  - Digital channel text
  - Country of origin

**Model Updates:**

- `PackageInfo` struct extended with 11 new fields:
  - `Volume`, `IsPCU`, `MinOrderQty`, `Deliverable`, `Stocked`
  - `CalculatedCount`, `ConsistsOfCount`, `ConsistsOfUnit`
  - `DangerousGoods`, `DGUNNumber`

- `nobbItem` struct updated for complete API response parsing:
  - Properties now grouped by category (ETIM, Environment, Marketing, EPD, Other)
  - Added NRF info struct
  - Added customs codes (NO and EU)

**Technical Improvements:**

- Fixed JSON parsing for float fields (consistsOfCount, calculatedCount)
- Added property category extraction (nobb_etim, nobb_env, nobb_marketing, nobb_epd, nobb_other)
- Improved error messages with search method details
- Added helper methods for properties group (IsEmpty, TotalCount)

#### New Documentation

- **`docs/NOBB.md`** - Comprehensive NOBB API integration documentation
  - Complete API endpoint reference
  - Response structure examples
  - Media types and package classes
  - Troubleshooting guide
  - Best practices

### Changed

- CLAUDE.md updated with detailed NOBB connector information
- Version bumped to 1.2.0

---

## [1.0.0] - 2026-01-18

### Added

#### Multi-Source Enhancement Platform
Complete rewrite as a multi-source product enhancement platform for Bad.no's operations team.

#### Source Connector Framework (`internal/source/`)
- **Connector Interface** - Unified interface for data sources
  - `connector.go` - Core interface definition with FetchProducts and EnhanceProduct
  - `registry.go` - Global connector registry for plugin-style architecture
- **Shopify Connector** (`shopify/connector.go`)
  - Import products directly from Shopify Admin API 2024-01
  - Filter by vendor, limit, or specific handles
  - Full pagination support for large catalogs
- **NOBB Connector** (`nobb/connector.go`)
  - Enhance products with Norwegian building products data
  - Fetches: items, properties, suppliers, packages
  - Basic Auth authentication with pagination via X-Forward-Token
- **Tiger.nl Connector** (`tiger/connector.go`)
  - Wraps existing scraper for image enhancement
  - Rate limiting support (configurable delay)
  - Extracts high-resolution PIM images

#### Output Adapter Framework (`internal/output/`)
- **Adapter Interface** - Unified interface for export destinations
  - `adapter.go` - Core interface with ExportProducts and format support
  - `registry.go` - Global adapter registry
- **File Adapters** (`file/`)
  - `csv.go` - Matrixify-compatible and Shopify CSV formats
  - `json.go` - JSON and JSONL export formats
- **Shopify Adapter** (`shopify/adapter.go`)
  - Direct product updates via Admin API
  - Update images, metafields, and product data
- **ClickHouse Adapter** (`clickhouse/adapter.go`)
  - Data warehouse export for analytics
  - Auto-creates products table if not exists

#### New CLI Commands

- **Configuration** (`cmd/badops/cmd/config.go`)
  - `badops config init` - Initialize config file at ~/.badops/config.yaml
  - `badops config show` - Display current configuration
  - `badops config set <key> <value>` - Set config values
  - `badops config get <key>` - Get specific config values

- **Sources** (`cmd/badops/cmd/sources.go`)
  - `badops sources list` - List available data sources
  - `badops sources test <name>` - Test connection to a source
  - `badops sources info <name>` - Show source details and capabilities

- **Enhance** (`cmd/badops/cmd/enhance.go`)
  - `badops enhance run --source <name>` - Enhance products from a source
  - `badops enhance review` - Review pending enhancements
  - `badops enhance apply` - Apply approved enhancements
  - Supports `--dry-run` flag for preview

- **Export** (`cmd/badops/cmd/export.go`)
  - `badops export run --dest <dest>` - Export to destination
  - `badops export list` - List available export destinations
  - Supports formats: matrixify, shopify, json, jsonl
  - Supports `--enhanced-only` and `--dry-run` flags

- **Products** (enhanced `cmd/badops/cmd/products.go`)
  - `badops products import --source shopify` - Import from Shopify
  - `badops products list` - List products in state
  - Existing parse, match, lookup commands preserved

#### Enhanced Data Model (`pkg/models/product.go`)
- **EnhancedProduct** struct with rich product data:
  - Identity: ID, SKU, Handle, Barcode, NOBBNumber
  - Content: Title, Description, Vendor, ProductType, Tags
  - Pricing: Price struct with Amount and Currency
  - Physical: Dimensions (L/W/H), Weight (Value/Unit)
  - Media: ProductImage with source tracking
  - Specifications: Key-value map for product specs
  - Properties: Structured properties from NOBB
  - Supply Chain: Suppliers and PackageInfo from NOBB
  - Tracking: Enhancement history and status
- **Conversion Methods**: ToEnhancedProduct() and ToLegacyProduct() for backward compatibility

#### State Management (`internal/state/store.go`)
- **V2 State Format** with automatic migration from v1
- Product map indexed by SKU for fast lookups
- Enhancement history tracking with timestamps
- Thread-safe operations

#### Configuration System (`internal/config/config.go`)
- YAML-based configuration at `~/.badops/config.yaml`
- Environment variable references for secrets
- Configurable sources, outputs, and defaults

#### Orchestrator (`internal/orchestrator/orchestrator.go`)
- Pipeline coordinator for import → enhance → export flows
- Multi-source enhancement in single pass
- Unified error handling and progress reporting

### Changed
- State file upgraded to v2 format (auto-migrates from v1)
- Products command now uses new state store
- All image commands work with EnhancedProduct model

### Dependencies Added
- `gopkg.in/yaml.v3` - YAML configuration parsing

### Backward Compatibility
- All existing commands (`parse`, `match`, `images compare/fetch/resize`) continue to work
- Legacy v1 state files automatically migrated on first load
- Existing workflows fully supported

---

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

### Completed in 1.2.0
- [x] Rate limiting for Tiger.nl requests (configurable via config)
- [x] `--dry-run` flag for enhance and export commands
- [x] Config file support (~/.badops/config.yaml)
- [x] Direct Shopify API integration (import and export)
- [x] Support for additional suppliers (NOBB integration)
- [x] Complete NOBB data extraction (images, properties, packages)
- [x] NOBB API documentation

### [1.3.0] - Planned
- [ ] Retry logic with exponential backoff for failed downloads
- [ ] Visual image deduplication (perceptual hashing)
- [ ] Manual review queue for low-confidence matches
- [ ] Better error handling and logging
- [ ] Pipeline command for chained operations

### [1.4.0] - Planned
- [ ] Batch processing with job queues
- [ ] Progress persistence for resumable operations
- [ ] NOBB property mapping to Shopify metafields

### [2.0.0] - Future
- [ ] Web UI for manual review
- [ ] Automated scheduling/cron support
- [ ] Webhook notifications for completed jobs
- [ ] Multi-tenant support
