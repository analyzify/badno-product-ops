# Bad.no Product Operations - Claude Code Context

## Project Overview

Multi-source product enhancement platform for Bad.no. Imports products from Shopify, enhances with data from NOBB and Tiger.nl, exports to multiple destinations.

**Version**: 1.0.0
**Status**: Production Ready
**Language**: Go 1.21+
**Repository**: https://github.com/analyzify/badno-product-ops

## Quick Reference

```bash
# Build
go build -o badops ./cmd/badops

# New workflow (Shopify → Enhance → Export)
export SHOPIFY_API_KEY=your_key
./badops products import --source shopify --vendor Tiger
./badops enhance run --source tiger_nl
./badops export run --dest csv --format matrixify

# Legacy workflow (CSV → Match → Fetch)
./badops products parse testdata/tiger-sample.csv
./badops products match
./badops images fetch --new-only --limit 20
```

## Architecture

```
cmd/badops/cmd/
├── root.go       - CLI setup, ASCII banner
├── config.go     - config init|show|set|get
├── sources.go    - sources list|test|info
├── products.go   - import, parse, list, match, lookup
├── enhance.go    - run, review, apply
├── export.go     - run, list
└── images.go     - compare, fetch, resize

internal/
├── source/                      # Source Connector Framework
│   ├── connector.go             - Connector interface
│   ├── registry.go              - Global registry
│   ├── shopify/connector.go     - Shopify import
│   ├── nobb/connector.go        - NOBB enhancement
│   └── tiger/connector.go       - Tiger.nl images
│
├── output/                      # Output Adapter Framework
│   ├── adapter.go               - Adapter interface
│   ├── registry.go              - Global registry
│   ├── file/csv.go              - CSV (Matrixify/Shopify)
│   ├── file/json.go             - JSON/JSONL
│   ├── shopify/adapter.go       - Shopify API
│   └── clickhouse/adapter.go    - ClickHouse
│
├── state/store.go               - V2 state with migration
├── config/config.go             - YAML config (~/.badops/)
├── orchestrator/orchestrator.go - Pipeline coordinator
│
├── parser/matrixify.go          - CSV parsing
├── matcher/
│   ├── tiger.go                 - Product matching
│   ├── scraper.go               - Tiger.nl scraper
│   └── skumapper.go             - SKU → Tiger ID mapping
└── images/
    ├── fetcher.go               - HTTP downloads
    └── resizer.go               - Center-crop resize

pkg/models/product.go            - EnhancedProduct + legacy Product
```

## Key Interfaces

### Source Connector (`internal/source/connector.go`)
```go
type Connector interface {
    Name() string
    Type() ConnectorType  // "source" or "enhancement"
    Capabilities() []Capability
    Connect(ctx context.Context) error
    FetchProducts(ctx context.Context, opts FetchOptions) (*FetchResult, error)
    EnhanceProduct(ctx context.Context, product *EnhancedProduct) (*EnhancementResult, error)
}
```

### Output Adapter (`internal/output/adapter.go`)
```go
type Adapter interface {
    Name() string
    Connect(ctx context.Context) error
    ExportProducts(ctx context.Context, products []EnhancedProduct, opts ExportOptions) (*ExportResult, error)
    SupportsFormat(format Format) bool
}
```

## Data Models

### EnhancedProduct (`pkg/models/product.go`)
```go
type EnhancedProduct struct {
    // Identity
    ID, SKU, Handle, Barcode, NOBBNumber string

    // Content
    Title, Description, Vendor, ProductType string
    Tags []string

    // Pricing & Physical
    Price *Price
    Dimensions *Dimensions
    Weight *Weight

    // Media
    Images []ProductImage

    // From NOBB
    Properties []Property
    Suppliers []Supplier
    PackageInfo []PackageInfo

    // Tracking
    Enhancements []Enhancement
    Status ProductStatus  // pending, enhanced, approved, exported
}
```

## State Management

### V2 State File (`output/.badops-state.json`)
```json
{
  "version": "2.0",
  "products": { "SKU": { ...EnhancedProduct } },
  "history": [{ "timestamp", "action", "source", "count" }],
  "last_updated": "2024-01-15T..."
}
```

V1 state files (plain arrays) are auto-migrated on first load.

### State Store (`internal/state/store.go`)
```go
store := state.NewStore("")
store.Load()
store.GetAllProducts()
store.GetProductsByVendor("Tiger")
store.ImportProducts(products, "shopify")
store.AddHistory("enhance", "tiger_nl", 10, "details")
store.Save()
```

## Configuration

### Config File (`~/.badops/config.yaml`)
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
  file:
    output_dir: ./output
```

### Config API (`internal/config/config.go`)
```go
cfg, _ := config.Load()
config.Set("sources.shopify.store", "mystore")
config.Init()  // Creates default config
```

## Common Tasks

### Add a new source connector
1. Create `internal/source/myconnector/connector.go`
2. Implement `source.Connector` interface
3. Register in `cmd/badops/cmd/sources.go` → `initSources()`

### Add a new output adapter
1. Create `internal/output/myadapter/adapter.go`
2. Implement `output.Adapter` interface
3. Register in export command or orchestrator

### Add a new CLI command
1. Create `cmd/badops/cmd/mycommand.go`
2. Define cobra.Command with RunE function
3. Register in `cmd/badops/cmd/root.go` → `init()`

### Add a new enhancement source
1. Implement `source.Connector` with `Type() = TypeEnhancement`
2. Implement `EnhanceProduct()` method
3. Add to config and `initSources()`

## API Integrations

### Shopify Admin API
- Version: 2024-01
- Auth: `X-Shopify-Access-Token` header
- Endpoints: `/products.json`, `/products/{id}.json`

### NOBB API
- Base: `https://export.byggtjeneste.no/api`
- Auth: Basic Auth
- Pagination: `X-Forward-Token` header
- Endpoints: `/items`, `/items/{nobb}/properties`, `/items/{nobb}/suppliers`

### Tiger.nl
- Scraping with 150ms rate limit
- Image URL: `https://tiger.nl/pim/528_{UUID}?width=1200&height=1200`
- Cache: 24 hours (`output/.tiger-cache.json`)

## Environment Variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `SHOPIFY_API_KEY` | For import | Shopify Admin API token |
| `NOBB_USERNAME` | For NOBB | NOBB API username |
| `NOBB_PASSWORD` | For NOBB | NOBB API password |

## Test Data

`testdata/tiger-sample.csv` - 10 Tiger products for testing:
- CO-T309012: Boston Toilet Roll Holder
- CO-T309512: Boston Hook Black
- CO-T317312: Urban Toilet Brush

## Dependencies

```
github.com/spf13/cobra v1.10.2      # CLI framework
github.com/fatih/color v1.18.0      # Terminal colors
github.com/schollz/progressbar/v3   # Progress bars
github.com/olekukonko/tablewriter   # ASCII tables
github.com/disintegration/imaging   # Image processing
gopkg.in/yaml.v3                    # YAML config
```

## Command Reference

| Command | Description |
|---------|-------------|
| `config init` | Create config file |
| `config show` | Display configuration |
| `config set <key> <value>` | Set config value |
| `sources list` | List available connectors |
| `sources test [name]` | Test connectivity |
| `products import --source shopify` | Import from Shopify |
| `products parse <csv>` | Parse Matrixify CSV |
| `products list` | List products in state |
| `products match` | Match against Tiger.nl |
| `enhance run --source <names>` | Run enhancements |
| `enhance review` | Review pending |
| `enhance apply` | Apply approved |
| `export run --dest <dest>` | Export products |
| `export list` | List destinations |
| `images compare` | Compare image counts |
| `images fetch` | Download images |
| `images resize` | Resize to square |

## Backward Compatibility

- `products parse` still works (uses new state store)
- `products match` still works (reads from v2 state)
- `images *` commands still work
- V1 state files auto-migrate to V2
- Legacy `Product` struct preserved, converts to/from `EnhancedProduct`
