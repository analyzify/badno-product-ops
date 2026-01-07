# Bad.no Operations Terminal - MVP

## Project Summary

**Client**: Bad.no AS (CEO: Erland)
**Scope**: CLI-based product operations toolkit for their ecommerce team
**Phase**: MVP Prototype
**Purpose**: Validate the concept of empowering product teams with AI-assisted CLI workflows

**Local Path**: `/Users/ermankuplu/Building/bad-no-ops`

## Business Context

Bad.no's product team handles ~9-10 sub-tasks when creating new products:
1. Group variants from PriceList
2. Assign variant attributes and options
3. Set product type + collections/breadcrumb
4. Create SEO-friendly product names + title/description
5. Assign brand, supplier cost price(s), RRP
6. Collect data (PDFs: FDV, Assembly instructions, EPD, etc)
7. Collect images - resize images
8. Write product descriptions, dimensions

## MVP Scope

For this prototype, focus on **one workflow**: Tiger product image enhancement.

### Input
- Matrixify export from Bad.no's Shopify store (Tiger brand products)
- CSV/Excel format with existing product data and image URLs

### Processing
1. Parse the product list into structured records
2. For each product, match against Tiger.nl official website
3. Identify images not already in Bad.no's product data
4. Download original-size images from Tiger.nl
5. Resize images to square format (800x800 to 2000x2000) with product-centered cropping

### Output
- Original images saved to designated folder (preserve full quality)
- Resized images saved separately (square, product-centered)
- Report showing: matched products, new images found, processing status

## Technical Requirements

### CLI Commands

```
# Parse and validate product list
badops products parse <matrixify-export.csv>

# Match products against Tiger.nl
badops products match --source tiger-nl

# Fetch missing images
badops images fetch --dry-run
badops images fetch --download

# Resize images (center-crop to square)
badops images resize --size 800x800
badops images resize --size 2000x2000
```

### Image Processing Rules

- **Original preservation**: Always save original before any processing
- **Center-crop logic**: Detect product bounds, center in frame before resize
- **Formats**: Support JPEG, PNG, WebP
- **Naming**: Maintain product SKU reference in filename

### Product Matching Strategy

For Tiger.nl matching:
1. Use product name/SKU from Matrixify export
2. Search Tiger.nl product catalog
3. Fuzzy match on product name + attributes
4. Confidence threshold for auto-match vs manual review queue

## Data Structures

### Product Record
```
Product:
  - id: string (internal)
  - sku: string
  - name: string
  - brand: string (Tiger)
  - source_images: []string
  - matched_url: string (Tiger.nl URL)
  - match_confidence: float
  - new_images: []Image
```

### Image Record
```
Image:
  - url: string (source)
  - local_path: string
  - original_path: string
  - resized_paths: map[size]string
  - status: pending|downloaded|processed|failed
```

## Out of Scope (Future Phases)

- Automated Shopify upload
- SEO name/alt generation (they have a Shopify app for this)
- PDF collection workflows
- Price management
- Multi-brand support (MVP is Tiger only)

## Success Criteria

- [ ] CLI parses Matrixify export without errors
- [ ] Product matching finds 80%+ of Tiger products on Tiger.nl
- [ ] Image download preserves original quality
- [ ] Resize maintains product centering (no awkward crops)
- [ ] Full workflow runs in <5 min for 100 products
