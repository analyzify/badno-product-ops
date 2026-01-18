# NOBB Integration Documentation

This document details the technical integration with NOBB (Norsk Byggevare Database) API for product data enhancement.

## Overview

NOBB is the Norwegian Building Products Database, containing comprehensive data for construction materials and products sold in Norway. The badops NOBB connector enriches products with:

- **Product Details**: Descriptions, product types, classifications
- **Physical Specifications**: Dimensions, weight, volume
- **Images**: Product images, documentation, assembly instructions
- **ETIM Properties**: Technical specifications (material, color, dimensions, etc.)
- **Environmental Properties**: BREEAM-NOR compliance, emissions data
- **Supplier Information**: Multiple suppliers with article numbers
- **Package Details**: F-PAK, D-PAK with GTIN, delivery/stock status

## API Information

### Base URL
```
https://export.byggtjeneste.no/api/v1
```

### Authentication
- **Method**: HTTP Basic Authentication
- **Credentials**: Username and password provided by Byggtjeneste AS
- **Environment Variables**:
  - `NOBB_USERNAME` - API username
  - `NOBB_PASSWORD` - API password

### Rate Limiting
- **Limit**: 200 requests per time window
- **Headers**:
  - `X-RateLimit-Limit: 200`
  - `X-RateLimit-Reset: <timestamp>`

## API Endpoints

### Items Endpoint

**Search by NOBB Number:**
```
GET /items?nobbnos={nobbNumber}
```

**Search by GTIN/EAN:**
```
GET /items?gtins={gtin}
```

**Search by Text:**
```
GET /items?search={query}&pageSize={limit}
```

**Pagination:**
- Use `pageSize` parameter (default varies)
- Forward token in `X-Forward-Token` header for next page

### Properties Endpoint (Separate)
```
GET /items/{nobbNumber}/properties
```

Returns flat list of all properties for an item.

### Suppliers Endpoint (Separate)
```
GET /items/{nobbNumber}/suppliers
```

Returns supplier details including packages and media.

## Response Structure

### Item Response

```json
{
  "nobbNumber": 10002681,
  "primaryText": "SKOGSØKS 0,9 KG M/26\" SKAFT",
  "secondaryText": "227H",
  "description": "Denne skogsøksa er en god arbeidskamerat...",
  "digitalChannelText": "SKOGSØKS 0,9 KG M/26\" SKAFT",
  "productGroupNumber": "1850260",
  "productGroupName": "Økser",
  "countryOfOrigin": "SI",
  "customsNoCode": "82014000",
  "customsEuCode": null,
  "etimClass": "EC002112",
  "manufacturerItemNumber": "07050",
  "nrfInfo": null,
  "suppliers": [...],
  "properties": {...}
}
```

### Supplier Structure

```json
{
  "participantNumber": "50377",
  "name": "Øyo AS",
  "isMainSupplier": true,
  "supplierItemNumber": "07050",
  "expiryDate": null,
  "packages": [...],
  "media": [...]
}
```

### Package Structure

```json
{
  "class": "F-PAK",
  "gtin": "7051750070501",
  "gtinType": "GTIN13",
  "weight": 1.5000,
  "length": 170,
  "width": 30,
  "height": 640,
  "volume": 0.003264,
  "unit": "STK",
  "isPCU": true,
  "minOrderQuantity": null,
  "deliverable": true,
  "stocked": true,
  "calculatedCount": 1.000000,
  "consistsOfCount": 1.000000,
  "consistsOfUnit": "STK"
}
```

### Media Structure

```json
{
  "guid": "3da55f5c-12d1-4414-b4e3-7a34cfc9f806",
  "mediaType": "PB",
  "url": "https://cdn.byggtjeneste.no/nobb/3da55f5c-12d1-4414-b4e3-7a34cfc9f806/square",
  "isPrimary": true
}
```

### Properties Structure

Properties are grouped by category:

```json
{
  "properties": {
    "etim": [
      {
        "propertyGuid": "...",
        "propertyName": "Material",
        "propertyDescription": "...",
        "value": "Steel",
        "unit": null
      }
    ],
    "environment": [
      {
        "propertyGuid": "0E474427-ED7A-4A13-89D3-4E4FB25810A1",
        "propertyName": "Produktet omfattes av BREEAM-NOR A20 2016",
        "value": "True",
        "unit": "Ja/Nei"
      }
    ],
    "marketing": [
      {
        "propertyGuid": "...",
        "propertyName": "Produktfordel 1",
        "value": "øksehodet under en kile"
      }
    ],
    "epd": [],
    "other": []
  }
}
```

## Media Types

| Code | Description | Usage |
|------|-------------|-------|
| `PB` | Produktbilde (Product Image) | Main product photo |
| `FDV` | Forvaltning, Drift, Vedlikehold | Documentation files |
| `MTG` | Monteringsveiledning | Assembly/mounting instructions |
| `MB` | Miljøbilde | Environment/lifestyle image |
| `TEG` | Teknisk tegning | Technical drawing |

## Package Classes

| Class | Description |
|-------|-------------|
| `F-PAK` | Forbrukerpakke (Consumer Package) - Single unit |
| `D-PAK` | Detaljistpakke (Retail Package) - Inner carton |
| `T-PAK` | Transportpakke (Transport Package) - Outer carton |
| `PAL` | Pall (Pallet) |

## Data Extraction

### Fields Extracted from NOBB

| Category | Field | Model Location |
|----------|-------|----------------|
| **Identity** | NOBB Number | `product.NOBBNumber` |
| **Content** | Description | `product.Description` |
| **Content** | Product Type | `product.ProductType` |
| **Physical** | Dimensions (L×W×H) | `product.Dimensions` |
| **Physical** | Weight | `product.Weight` |
| **Identity** | Barcode (GTIN) | `product.Barcode` |
| **Specifications** | Country of Origin | `product.Specifications["country_of_origin"]` |
| **Specifications** | Customs Code | `product.Specifications["customs_code"]` |
| **Specifications** | ETIM Class | `product.Specifications["etim_class"]` |
| **Specifications** | Manufacturer Item Number | `product.Specifications["manufacturer_item_number"]` |
| **Specifications** | NRF Info | `product.Specifications["nrf_*"]` |
| **Media** | Product Images | `product.Images[]` (source: "nobb") |
| **Properties** | ETIM Properties | `product.Properties[]` (source: "nobb_etim") |
| **Properties** | Environmental | `product.Properties[]` (source: "nobb_env") |
| **Properties** | Marketing | `product.Properties[]` (source: "nobb_marketing") |
| **Properties** | EPD | `product.Properties[]` (source: "nobb_epd") |
| **Supply Chain** | Suppliers | `product.Suppliers[]` |
| **Packaging** | Package Info | `product.PackageInfo[]` |

### PackageInfo Model

```go
type PackageInfo struct {
    Type            string  // Package class (F-PAK, D-PAK, etc.)
    Quantity        int     // Number of items
    GTIN            string  // Barcode
    Weight          float64 // Weight in kg
    WeightUnit      string  // Always "kg" from NOBB
    Length          float64 // Length in mm
    Width           float64 // Width in mm
    Height          float64 // Height in mm
    DimUnit         string  // Always "mm" from NOBB
    Volume          float64 // Volume in liters
    IsPCU           bool    // Is Price Calculation Unit
    MinOrderQty     int     // Minimum order quantity
    Deliverable     bool    // Can be delivered
    Stocked         bool    // Is in stock
    CalculatedCount float64 // Calculated count
    ConsistsOfCount float64 // Number of contained items
    ConsistsOfUnit  string  // Unit (STK, etc.)
    DangerousGoods  bool    // Contains dangerous goods
    DGUNNumber      string  // UN number if dangerous
}
```

## Usage in badops

### Configuration

```yaml
# ~/.badops/config.yaml
sources:
  nobb:
    username_env: NOBB_USERNAME
    password_env: NOBB_PASSWORD
```

### Environment Variables

```bash
export NOBB_USERNAME="your_username"
export NOBB_PASSWORD="your_password"
```

### Testing Connection

```bash
badops sources test nobb
```

### Enhancing Products

```bash
# Enhance all products with NOBB numbers
badops enhance run --source nobb

# Dry run to preview
badops enhance run --source nobb --dry-run
```

### Search Priority

The connector searches for products in this order:

1. **NOBB Number** - If `product.NOBBNumber` is set, direct lookup
2. **Barcode/EAN** - If `product.Barcode` is set, GTIN search
3. **SKU** - If SKU looks like a NOBB number (8 digits), try lookup

## Example Output

Enhancing product NOBB#10002681 (SKOGSØKS - Forest Axe):

```
=== ENHANCEMENT RESULT ===
Fields updated: [description product_type dimensions weight barcode properties suppliers images package_info]

=== PRODUCT DATA ===
NOBB Number: 10002681
Description: Denne skogsøksa er en god arbeidskamerat...
Product Type: Økser
Barcode: 7051750070501
Dimensions: 170x30x640 mm
Weight: 1.50 kg

=== SPECIFICATIONS ===
  country_of_origin: SI
  customs_code: 82014000
  etim_class: EC002112
  manufacturer_item_number: 07050
  nobb_product_group_code: 1850260
  nobb_product_group_name: Økser

=== IMAGES (2) ===
  [nobb] Product Image: https://cdn.byggtjeneste.no/nobb/3da55f5c.../square
  [nobb] Product Image: https://cdn.byggtjeneste.no/nobb/9397f64b.../square

=== PROPERTIES (2) ===
  [nobb_marketing] Produktfordel 1: øksehodet under en kile
  [nobb_marketing] Produktfordel 2: kvisting, trimming

=== SUPPLIERS (4) ===
  Øyo AS (ID: 50377, Article: 07050) [PRIMARY]
  Ole Moe AS (ID: 207393, Article: 10002681)
  Fjelland Handel AS (ID: 209151, Article: 2440090)
  Schou Norge AS (ID: 211864, Article: 6190007050)

=== PACKAGE INFO (8) ===
  Type: F-PAK | GTIN: 7051750070501
    Weight: 1.50 kg | Volume: 0.003 L
    Size: 170x30x640 mm
    PCU: true | Deliverable: true | Stocked: true
    ConsistsOf: 1 STK
```

## Technical Implementation

### Connector Location
```
internal/source/nobb/connector.go
```

### Key Functions

| Function | Purpose |
|----------|---------|
| `NewConnector(cfg)` | Create connector with config |
| `Connect(ctx)` | Authenticate and validate connection |
| `Test(ctx)` | Test API connectivity |
| `EnhanceProduct(ctx, product)` | Enrich product with NOBB data |
| `fetchItemByNOBBNumber(ctx, nobb)` | Fetch item by NOBB number |
| `searchItemByEAN(ctx, ean)` | Search by GTIN/EAN |
| `fetchPropertiesSeparate(ctx, nobb)` | Fetch properties from separate endpoint |
| `applyNobbData(product, item)` | Apply NOBB data to product |

### Error Handling

The connector provides detailed error messages:

```
NOBB search by number failed: JSON decode error: ...
product not found in NOBB database (searched by: nobb_number, nobb=12345678)
```

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| `NOBB credentials not configured` | Missing env vars | Set NOBB_USERNAME and NOBB_PASSWORD |
| `NOBB authentication failed` | Invalid credentials | Verify credentials with Byggtjeneste |
| `product not found in NOBB database` | Product not in NOBB | Verify NOBB number or barcode |
| `JSON decode error` | API response format changed | Report issue, may need struct updates |

### Debug Mode

For debugging API issues, the connector logs:
- Search method used (nobb_number, barcode, sku)
- Fields searched with their values
- Number of items found

## Best Practices

1. **Always verify NOBB numbers** - 8-digit format required
2. **Handle multiple suppliers** - Same product may have different suppliers
3. **Check package types** - F-PAK is usually the consumer unit
4. **Use isPCU flag** - Identifies the pricing unit
5. **Check deliverable/stocked** - May vary by supplier
6. **Image URLs** - Use CDN URLs directly, they support resizing

## References

- [NOBB Database](https://www.byggtjeneste.no/nobb)
- [Byggtjeneste API Documentation](https://export.byggtjeneste.no/api)
- [ETIM Classification](https://www.etim-international.com)
- [BREEAM-NOR](https://www.ngbc.no/breeam-nor)
