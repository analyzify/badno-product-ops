# Bad.no Product Operations (badops)

A multi-source product enhancement platform for Bad.no's operations team. Import products from Shopify, enhance them with data from NOBB and Tiger.nl, and export to multiple destinations.

## Features

- **Multi-Source Import**: Pull products directly from Shopify or parse CSV exports
- **Product Enhancement**: Enrich products with images from Tiger.nl and specifications from NOBB
- **Flexible Export**: Output to Matrixify CSV, JSON, Shopify API, or ClickHouse
- **State Management**: Track enhancement history and product status
- **Backward Compatible**: Existing workflows continue to work

## Installation

```bash
# Clone the repository
git clone https://github.com/analyzify/badno-product-ops.git
cd badno-product-ops

# Build
go build -o badops ./cmd/badops

# Or install globally
go install ./cmd/badops
```

## Quick Start

### Option 1: Import from Shopify (New)

```bash
# 1. Configure your API key
export SHOPIFY_API_KEY=your_api_key_here

# 2. Import products from Shopify
./badops products import --source shopify --vendor Tiger

# 3. Enhance with Tiger.nl images
./badops enhance run --source tiger_nl

# 4. Export to Matrixify CSV
./badops export run --dest csv --format matrixify
```

### Option 2: Parse CSV (Legacy)

```bash
# 1. Parse your Matrixify export
./badops products parse exports/tiger-products.csv

# 2. Match and enhance
./badops products match
./badops images compare
./badops images fetch --new-only --limit 20

# 3. Resize images
./badops images resize --size 800
```

## Commands

### Configuration

```bash
# Initialize config file (~/.badops/config.yaml)
./badops config init

# Show current configuration
./badops config show

# Set a config value
./badops config set sources.shopify.store mystore

# Get a config value
./badops config get sources.shopify.store
```

### Sources

```bash
# List available data sources
./badops sources list

# Test connection to a source
./badops sources test shopify
./badops sources test nobb

# Show source details
./badops sources info tiger_nl
```

### Products

```bash
# Import from Shopify (requires SHOPIFY_API_KEY)
./badops products import --source shopify --vendor Tiger --limit 100

# Parse CSV file (legacy)
./badops products parse exports/tiger-products.csv

# List products in state
./badops products list

# Match products against Tiger.nl
./badops products match

# Look up a single SKU
./badops products lookup CO-T309012
```

### Enhance

```bash
# Enhance products with Tiger.nl images
./badops enhance run --source tiger_nl

# Enhance with NOBB data (requires NOBB_USERNAME and NOBB_PASSWORD)
./badops enhance run --source nobb

# Enhance with multiple sources
./badops enhance run --source tiger_nl,nobb

# Dry run (preview without changes)
./badops enhance run --source tiger_nl --dry-run

# Review pending enhancements
./badops enhance review

# Apply approved enhancements
./badops enhance apply
```

### Images

```bash
# Compare image counts (bad.no vs Tiger.nl)
./badops images compare

# Download new images from Tiger.nl
./badops images fetch --new-only --limit 20

# Resize images to square format
./badops images resize --size 800
```

### Export

```bash
# List available export destinations
./badops export list

# Export to Matrixify CSV
./badops export run --dest csv --format matrixify

# Export to JSON
./badops export run --dest json

# Export only enhanced products
./badops export run --dest csv --enhanced-only

# Dry run
./badops export run --dest csv --dry-run

# Custom output path
./badops export run --dest csv -o my-products.csv
```

## Configuration

### Config File

Create with `badops config init` at `~/.badops/config.yaml`:

```yaml
sources:
  shopify:
    store: badno
    api_key_env: SHOPIFY_API_KEY
  nobb:
    username_env: NOBB_USERNAME
    password_env: NOBB_PASSWORD
  tiger_nl:
    rate_limit_ms: 150

outputs:
  shopify:
    store: badno
    api_key_env: SHOPIFY_API_KEY
  clickhouse:
    host: localhost
    port: 9000
    database: products
  file:
    output_dir: ./output
    pretty: true

defaults:
  vendor: Tiger
  enhance_sources:
    - tiger_nl
    - nobb
  export_format: matrixify
```

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `SHOPIFY_API_KEY` | Shopify Admin API access token |
| `NOBB_USERNAME` | NOBB API username |
| `NOBB_PASSWORD` | NOBB API password |
| `CLICKHOUSE_USERNAME` | ClickHouse username (optional) |
| `CLICKHOUSE_PASSWORD` | ClickHouse password (optional) |

## Architecture

```
badno-product-ops/
├── cmd/badops/cmd/
│   ├── root.go         # CLI setup, ASCII banner
│   ├── config.go       # config init|show|set|get
│   ├── sources.go      # sources list|test|info
│   ├── products.go     # products import|parse|list|match|lookup
│   ├── enhance.go      # enhance run|review|apply
│   ├── export.go       # export run|list
│   └── images.go       # images compare|fetch|resize
│
├── internal/
│   ├── source/                    # Source connectors
│   │   ├── connector.go           # Connector interface
│   │   ├── registry.go            # Connector registry
│   │   ├── shopify/connector.go   # Shopify import
│   │   ├── nobb/connector.go      # NOBB enhancement
│   │   └── tiger/connector.go     # Tiger.nl images
│   │
│   ├── output/                    # Output adapters
│   │   ├── adapter.go             # Adapter interface
│   │   ├── registry.go            # Adapter registry
│   │   ├── file/csv.go            # CSV export
│   │   ├── file/json.go           # JSON export
│   │   ├── shopify/adapter.go     # Shopify API
│   │   └── clickhouse/adapter.go  # ClickHouse
│   │
│   ├── state/store.go             # State management
│   ├── config/config.go           # Configuration
│   ├── orchestrator/orchestrator.go # Pipeline coordinator
│   │
│   ├── parser/matrixify.go        # CSV parsing
│   ├── matcher/                   # Tiger.nl matching
│   │   ├── tiger.go
│   │   ├── scraper.go
│   │   └── skumapper.go
│   └── images/                    # Image processing
│       ├── fetcher.go
│       └── resizer.go
│
├── pkg/models/product.go          # Data models
└── testdata/                      # Sample data
```

## Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    CLI Commands (Cobra)                          │
│  config | sources | products | enhance | images | export         │
└─────────────────────────────────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────┐
│                   State Store (v2)                               │
│  Products map + History + Auto-migration from v1                 │
└─────────────────────────────────────────────────────────────────┘
         │                                           │
┌────────▼────────────┐                 ┌───────────▼───────────┐
│  Source Connectors  │                 │   Output Adapters     │
│  • Shopify (import) │                 │   • CSV (Matrixify)   │
│  • NOBB (enhance)   │                 │   • JSON/JSONL        │
│  • Tiger.nl (images)│                 │   • Shopify API       │
└─────────────────────┘                 │   • ClickHouse        │
                                        └───────────────────────┘
```

## State File

Products are stored in `output/.badops-state.json` (v2 format):

```json
{
  "version": "2.0",
  "products": {
    "CO-T309012": {
      "sku": "CO-T309012",
      "title": "Tiger Boston Toalettrullholder",
      "vendor": "Tiger",
      "images": [...],
      "enhancements": [...],
      "status": "enhanced"
    }
  },
  "history": [
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "action": "import",
      "source": "shopify",
      "count": 179
    }
  ],
  "last_updated": "2024-01-15T10:30:00Z"
}
```

Legacy v1 state files are automatically migrated on first load.

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/fatih/color` | Colored terminal output |
| `github.com/schollz/progressbar/v3` | Progress bars |
| `github.com/olekukonko/tablewriter` | ASCII tables |
| `github.com/disintegration/imaging` | Image processing |
| `gopkg.in/yaml.v3` | YAML config parsing |

## Workflow Examples

### Full Enhancement Pipeline

```bash
# 1. Set up credentials
export SHOPIFY_API_KEY=shpat_xxxxx
export NOBB_USERNAME=myuser
export NOBB_PASSWORD=mypass

# 2. Import Tiger products from Shopify
./badops products import --source shopify --vendor Tiger
# Output: "Imported 179 products from Shopify"

# 3. Enhance with Tiger.nl images
./badops enhance run --source tiger_nl
# Output: "Enhanced 156 products, added 892 images"

# 4. Enhance with NOBB specifications
./badops enhance run --source nobb
# Output: "Enhanced 142 products, updated 568 fields"

# 5. Review enhancements
./badops enhance review
# Shows table of enhanced products

# 6. Export to Matrixify format
./badops export run --dest csv --format matrixify -o tiger-enhanced.csv
# Output: "Exported 179 products to tiger-enhanced.csv"

# 7. Upload tiger-enhanced.csv to Shopify via Matrixify
```

### Quick Image Update (Legacy)

```bash
# Parse → Match → Compare → Fetch → Resize
./badops products parse ~/Downloads/tiger-export.csv
./badops products match
./badops images compare
./badops images fetch --new-only --limit 50
./badops images resize --size 800
```

## API Integrations

### Shopify Admin API
- Uses REST Admin API (2024-01)
- Requires access token with `read_products` and `write_products` scopes
- Supports pagination for large catalogs

### NOBB API
- Endpoint: `https://export.byggtjeneste.no/api`
- Basic Auth authentication
- Fetches: items, properties, suppliers, packages

### Tiger.nl
- Web scraping with rate limiting (150ms default)
- Image URLs: `https://tiger.nl/pim/528_{UUID}?width=1200&height=1200`
- Results cached for 24 hours

## License

Internal tool for Bad.no AS / Analyzify. Not for public distribution.
