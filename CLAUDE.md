# Bad.no Product Operations - Claude Code Context

## Project Overview

Multi-source product enhancement platform for Bad.no. Imports products from Shopify, enhances with data from NOBB and Tiger.nl, exports to multiple destinations.

**Version**: 1.2.0
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

# Database workflow (PostgreSQL + ClickHouse)
export POSTGRES_USER=badops POSTGRES_PASSWORD=secret
./badops db init                              # Create PostgreSQL schema
./badops db migrate --from-state              # Import existing JSON state
./badops prices import reprice-export.csv     # Import competitor prices
./badops competitors stats                    # View competitor coverage
./badops analytics init                       # Create ClickHouse schema
./badops analytics sync --all                 # Sync to ClickHouse
./badops analytics alerts --threshold 10     # Find pricing issues

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
├── images.go     - compare, fetch, resize
├── db.go         - db init|status|migrate
├── prices.go     - prices import|check|summary
├── competitors.go - competitors list|add|stats|remove
└── analytics.go  - analytics init|sync|trends|position|alerts

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
│   └── clickhouse/adapter.go    - ClickHouse export
│
├── database/                    # Database Layer
│   ├── repository.go            - Repository interfaces
│   ├── postgres/
│   │   ├── client.go            - Connection pool + migrations
│   │   ├── products.go          - Product CRUD
│   │   ├── competitors.go       - Competitors + price observations
│   │   ├── history.go           - History, images, properties
│   │   └── migrations/          - SQL migration files
│   └── clickhouse/
│       ├── client.go            - ClickHouse connection
│       ├── analytics.go         - Analytics queries
│       └── sync.go              - PG → CH sync
│
├── prices/                      # Price Tracking
│   └── parser.go                - Reprice CSV parser
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
    Dimensions *Dimensions  // Length, Width, Height in mm
    Weight *Weight          // Value in kg

    // Media
    Images []ProductImage   // From Shopify, NOBB, Tiger.nl

    // From NOBB
    Properties []Property   // ETIM, environmental, marketing properties
    Suppliers []Supplier    // Multiple suppliers with article numbers
    PackageInfo []PackageInfo // F-PAK, D-PAK with full details

    // Specifications (key-value from various sources)
    Specifications map[string]string

    // Tracking
    Enhancements []Enhancement
    Status ProductStatus  // pending, enhanced, approved, exported
}

// PackageInfo - Complete packaging data from NOBB
type PackageInfo struct {
    Type            string  // F-PAK, D-PAK, T-PAK, PAL
    GTIN            string  // Barcode (GTIN-13)
    Weight          float64 // Weight in kg
    Length, Width, Height float64 // Dimensions in mm
    Volume          float64 // Volume in liters
    IsPCU           bool    // Is Price Calculation Unit
    MinOrderQty     int     // Minimum order quantity
    Deliverable     bool    // Can be delivered
    Stocked         bool    // Is in stock
    ConsistsOfCount float64 // Items per package
    ConsistsOfUnit  string  // Unit (STK, etc.)
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

## Database Architecture

### Overview

PostgreSQL serves as the primary database for master data, while ClickHouse handles time-series analytics for price history.

```
┌─────────────────────────────────────────────────────────────┐
│                      BADOPS CLI                              │
│  products | prices | competitors | analytics | db            │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
┌───────────────┐    ┌───────────────┐    ┌───────────────┐
│  PostgreSQL   │    │  ClickHouse   │    │  File System  │
│  (Primary DB) │───▶│  (Analytics)  │    │  (Images)     │
├───────────────┤    ├───────────────┤    ├───────────────┤
│ • products    │    │ • price_hist  │    │ • originals/  │
│ • competitors │    │ • daily_mv    │    │ • resized/    │
│ • prices_90d  │    │ • position_mv │    │ • exports/    │
│ • images      │    │               │    │               │
│ • properties  │    │               │    │               │
└───────────────┘    └───────────────┘    └───────────────┘
```

### PostgreSQL Schema

```sql
-- Core tables
products            -- 46K+ products with full EnhancedProduct mapping
competitors         -- 29 tracked competitors
competitor_products -- Product-competitor links (many-to-many)
price_observations  -- Recent prices (partitioned, 90-day retention)
product_images      -- Image metadata and paths
product_properties  -- NOBB/Tiger properties
suppliers           -- Supplier master data
product_suppliers   -- Product-supplier links

-- Audit tables
enhancement_log     -- Per-product enhancement history
operation_history   -- Global operation log
```

### ClickHouse Schema

```sql
-- Time-series table with 2-year TTL
price_history (product_sku, competitor_name, price, observed_at)

-- Materialized views for fast analytics
price_daily_mv      -- Daily min/max/avg per product/competitor
price_position_mv   -- Daily market position per product
```

### Repository Pattern (`internal/database/repository.go`)

```go
type ProductRepository interface {
    Create(ctx, product) error
    GetBySKU(ctx, sku) (*EnhancedProduct, error)
    BulkUpsert(ctx, products) (int, error)
    GetAll(ctx, opts QueryOptions) ([]*EnhancedProduct, error)
    CountByVendor(ctx) (map[string]int64, error)
}

type PriceObservationRepository interface {
    BulkCreate(ctx, observations) (int, error)
    GetLatestByProduct(ctx, productID) ([]*PriceObservation, error)
    GetPriceHistory(ctx, productID, days) ([]*PriceObservation, error)
}
```

### Migration from JSON State

```bash
# 1. Initialize database
badops db init

# 2. Import existing state
badops db migrate --from-state ./output/.badops-state.json

# 3. Enable database backend
badops config set database.use_db true

# 4. Verify migration
badops db status
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

database:
  use_db: false  # Enable to use PostgreSQL instead of JSON state
  postgres:
    host: localhost
    port: 5432
    database: badops
    username_env: POSTGRES_USER
    password_env: POSTGRES_PASSWORD
    ssl_mode: prefer
  clickhouse:
    host: localhost
    port: 9000
    database: badops
    username_env: CLICKHOUSE_USERNAME
    password_env: CLICKHOUSE_PASSWORD
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

### Set up database backend
1. Install PostgreSQL and create `badops` database
2. Set environment variables: `POSTGRES_USER`, `POSTGRES_PASSWORD`
3. Run `badops db init` to create schema
4. Run `badops db migrate --from-state` to import existing data
5. Run `badops config set database.use_db true`

### Import competitor prices
1. Export CSV from Reprice with competitor columns
2. Run `badops prices import <csv-file>`
3. View results: `badops competitors stats`
4. Check specific product: `badops prices check --sku CO-T309012`

### Set up analytics
1. Install ClickHouse and create `badops` database
2. Set environment variables: `CLICKHOUSE_USERNAME`, `CLICKHOUSE_PASSWORD`
3. Run `badops analytics init` to create schema
4. Run `badops analytics sync --all` to sync historical data
5. Query trends: `badops analytics trends --sku CO-T309012 --period 30d`

## API Integrations

### Shopify Admin API
- Version: 2024-01
- Auth: `X-Shopify-Access-Token` header
- Endpoints: `/products.json`, `/products/{id}.json`

### NOBB API (Norwegian Building Products Database)
- Base: `https://export.byggtjeneste.no/api/v1`
- Auth: Basic Auth (username/password)
- Rate Limit: 200 requests per window
- Endpoints:
  - `/items?nobbnos={nobb}` - Search by NOBB number
  - `/items?gtins={gtin}` - Search by GTIN/EAN
  - `/items?search={query}` - Full-text search
  - `/items/{nobb}/properties` - Get ETIM/environment properties
  - `/items/{nobb}/suppliers` - Get supplier details

**Data Extracted:**
- Product details (description, type, ETIM class)
- Physical specs (dimensions L×W×H in mm, weight in kg)
- Images from CDN (`cdn.byggtjeneste.no/nobb/{guid}/square`)
- Properties (ETIM technical, environmental, marketing, EPD)
- Suppliers (with article numbers, multiple per product)
- Packages (F-PAK, D-PAK with GTIN, volume, delivery status)
- Classifications (customs code, NRF info, country of origin)

See `docs/NOBB.md` for detailed integration documentation.

### Tiger.nl
- Scraping with 150ms rate limit
- Image URL: `https://tiger.nl/pim/528_{UUID}?width=1200&height=1200`
- Cache: 24 hours (`output/.tiger-cache.json`)
- See `docs/TIGER-NL.md` for detailed documentation

## Environment Variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `SHOPIFY_API_KEY` | For import | Shopify Admin API token |
| `NOBB_USERNAME` | For NOBB | NOBB API username |
| `NOBB_PASSWORD` | For NOBB | NOBB API password |
| `POSTGRES_USER` | For database | PostgreSQL username |
| `POSTGRES_PASSWORD` | For database | PostgreSQL password |
| `CLICKHOUSE_USERNAME` | For analytics | ClickHouse username |
| `CLICKHOUSE_PASSWORD` | For analytics | ClickHouse password |

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
github.com/jackc/pgx/v5             # PostgreSQL driver
github.com/golang-migrate/migrate   # SQL migrations
github.com/ClickHouse/clickhouse-go # ClickHouse driver
github.com/google/uuid              # UUID generation
```

## Command Reference

### Configuration & Sources
| Command | Description |
|---------|-------------|
| `config init` | Create config file |
| `config show` | Display configuration |
| `config set <key> <value>` | Set config value |
| `sources list` | List available connectors |
| `sources test [name]` | Test connectivity |

### Products & Enhancement
| Command | Description |
|---------|-------------|
| `products import --source shopify` | Import from Shopify |
| `products parse <csv>` | Parse Matrixify CSV |
| `products list` | List products in state |
| `products match` | Match against Tiger.nl |
| `enhance run --source <names>` | Run enhancements |
| `enhance review` | Review pending |
| `enhance apply` | Apply approved |
| `export run --dest <dest>` | Export products |
| `export list` | List destinations |

### Images
| Command | Description |
|---------|-------------|
| `images compare` | Compare image counts |
| `images fetch` | Download images |
| `images resize` | Resize to square |

### Database Management
| Command | Description |
|---------|-------------|
| `db init` | Create PostgreSQL schema |
| `db status` | Show database health and table stats |
| `db migrate --from-state [path]` | Migrate JSON state to database |

### Price Tracking
| Command | Description |
|---------|-------------|
| `prices import <csv>` | Import Reprice CSV export |
| `prices check --sku <sku>` | Check competitor prices for product |
| `prices summary` | Show price data overview |

### Competitor Management
| Command | Description |
|---------|-------------|
| `competitors list` | List all tracked competitors |
| `competitors add <name>` | Add new competitor |
| `competitors stats` | Show coverage statistics |
| `competitors remove <name>` | Remove competitor and data |

### Analytics
| Command | Description |
|---------|-------------|
| `analytics init` | Initialize ClickHouse schema |
| `analytics sync [--all\|--days N]` | Sync PostgreSQL to ClickHouse |
| `analytics trends --sku <sku>` | Show price trends over time |
| `analytics position --sku <sku>` | Analyze market position |
| `analytics alerts --threshold N` | Find products above/below market |

## Backward Compatibility

- `products parse` still works (uses new state store)
- `products match` still works (reads from v2 state)
- `images *` commands still work
- V1 state files auto-migrate to V2
- Legacy `Product` struct preserved, converts to/from `EnhancedProduct`
