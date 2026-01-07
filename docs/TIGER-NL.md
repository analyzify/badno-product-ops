# Tiger.nl Integration Documentation

This document details the technical findings from integrating with Tiger.nl's website for product image extraction.

## Website Structure

### Base URL
```
https://tiger.nl
```

### Main Sections
| Path | Description |
|------|-------------|
| `/producten/badkameraccessoires/` | Bathroom accessories catalog |
| `/producten/badkamermeubels/` | Bathroom furniture |
| `/producten/handdouches-douchesets/` | Shower sets |
| `/producten/comfort-accessoires/` | Comfort accessories |
| `/producten/badkamerverlichting/` | Bathroom lighting |
| `/producten/toiletbrillen/` | Toilet seats |
| `/series/` | Product series landing pages |

## Product Series

Tiger organizes products into series. Key series for bad.no:

| Series | URL Path | Description |
|--------|----------|-------------|
| Boston | `/series/boston/` | Classic chrome/black bathroom accessories |
| Urban | `/series/urban/` | Modern minimalist design |
| 2-Store | `/series/2-store/` | Shower caddies and storage |
| Carv | `/series/carv/` | Adhesive-mounted accessories |
| Tune | `/series/tune/` | Budget-friendly line |
| Impuls | `/series/impuls/` | Stainless steel collection |
| Nomad | `/series/nomad/` | Bamboo accents |
| Items | `/series/items/` | Individual accessories |

## URL Patterns

### Category Filtering
```
/producten/badkameraccessoires/?productserie={series-slug}&color={color-slug}
```

Example:
```
/producten/badkameraccessoires/?productserie=productserie-boston&color=zwart
```

### Product Detail Pages
```
/producten/badkameraccessoires/{product-type}/{product-id}-{product-slug}/
```

Example:
```
/producten/badkameraccessoires/toiletrolhouder/1316530146-toiletrolhouder-zonder-klep/
```

### Series Slug Format
- `productserie-boston`
- `productserie-urban`
- `productserie-2-store`
- `productserie-carv`

## Image System (PIM)

Tiger.nl uses a Product Information Management (PIM) system for images.

### PIM Image URL Format
```
https://tiger.nl/pim/528_{UUID}?{parameters}
```

### Parameters
| Parameter | Values | Description |
|-----------|--------|-------------|
| `width` | 240, 528, 800, 1200, 2000 | Image width in pixels |
| `height` | 240, 528, 800, 1200, 2000 | Image height in pixels |
| `format` | jpg, webp, png | Output format |
| `quality` | 1-100 | JPEG quality (default: 100) |
| `rmode` | crop, max, pad | Resize mode |
| `ranchor` | center, topleft, etc. | Crop anchor point |

### Example URLs

**Thumbnail (240x240):**
```
https://tiger.nl/pim/528_2be183e7-9c32-419d-975e-f2b4aad4145a?rmode=crop&ranchor=center&width=240&height=240&format=webp&quality=100
```

**High Resolution (1200x1200):**
```
https://tiger.nl/pim/528_2be183e7-9c32-419d-975e-f2b4aad4145a?width=1200&height=1200&format=jpg&quality=90
```

**Maximum Size (no crop):**
```
https://tiger.nl/pim/528_2be183e7-9c32-419d-975e-f2b4aad4145a?rmode=max&width=2000&format=jpg&quality=95
```

### Known PIM Image IDs

Collected during development:

| Product | Color | PIM UUID |
|---------|-------|----------|
| Boston Toilet Roll Holder | Polished | `2be183e7-9c32-419d-975e-f2b4aad4145a` |
| Boston Toilet Roll Holder | Brushed | `e0e4d5eb-5754-4940-85c0-a0295c925e31` |
| Boston Toilet Roll Holder | Black | `2098839f-839b-433f-a2d0-b42a23fdfc55` |
| Boston Towel Rail 2-arm | Polished | `b72deb2e-c380-48ec-9cb6-8a1e9b8d0acb` |
| Boston Towel Rail 2-arm | Brushed | `f2d7516e-345c-49f1-9496-ab3c2612f280` |
| Boston Towel Rail 2-arm | Black | `8039cec5-687a-49cf-a2b3-715b93b0bfeb` |
| Urban Toilet Brush | White | `ae05a28d-0b9f-4f69-9809-53ec87d80ed9` |
| Urban Toilet Brush | Black | `33c1baa2-1951-482e-bfc3-021c1d85d77f` |
| 2-Store Shower Caddy | White | `3ecd131e-3371-40cb-a97e-cf0475e3a4cd` |
| 2-Store Shower Caddy | Black | `cc6d89b6-357c-4de9-9251-0145336f0f80` |

## Media Images

Non-PIM images (lifestyle, mood shots) use a different path:

```
https://tiger.nl/media/{hash}/{filename}.{ext}
```

### Examples
```
/media/hoobpnot/na_tiger_2store_imd_black_set2-crop.jpeg
/media/qkvipyvn/hr-8720553726524_tiger_2store_imd_hangingshowercaddy_white.jpg
/media/141dqwei/40089113154__-sfeertoiletrolhouder-hang-koper.jpg
```

### Filename Patterns
- `hr-` prefix = High resolution
- `na_` prefix = Product shot (Netherlands?)
- `_imd_` = Product imagery
- `_pack_` = Packaging shot
- `sfeer` = Mood/lifestyle shot

## Images Per Product

A typical Tiger.nl product page contains **~6 images**:

1. **Main product shot** - Clean white background
2. **Alternate angle** - Different perspective
3. **Detail view** - Close-up of features
4. **Technical drawing** - Line art with dimensions
5. **Packaging image** - Product in box
6. **Installation image** - TigerFix mounting system

When scraping category pages, you may get **15-20 images** because the scraper picks up images from multiple products in the listing.

## Scraping Implementation

### Current Approach (v0.2.0)

1. **Build search URL** based on product name keywords
2. **Fetch category page** HTML
3. **Extract PIM URLs** using regex: `/pim/528_[a-f0-9-]+`
4. **Extract media URLs** using regex: `/media/[a-z0-9]+/[^"'\s]+\.(jpg|jpeg|png|webp)`
5. **Filter duplicates** using seen map
6. **Request high-res versions** by modifying URL parameters

### Regex Patterns

```go
// PIM images
pimRe := regexp.MustCompile(`/pim/528_[a-f0-9-]+`)

// Media images (excluding icons/logos)
mediaRe := regexp.MustCompile(`/media/[a-z0-9]+/[^"'\s]+\.(jpg|jpeg|png|webp)`)
```

### Product Type Mapping

Norwegian (bad.no) to Dutch (Tiger.nl):

| Norwegian | Dutch | Tiger.nl Category |
|-----------|-------|-------------------|
| toalettrullholder | toiletrolhouder | toiletrolhouder |
| toalettbørste | toiletborstel | toiletborstel-met-houder |
| håndklestang | handdoekhouder | handdoekhouder |
| krok | haak | haak |
| dusjkurv | douchekorf | douchekorf |
| speil | spiegel | spiegel |

## Rate Limiting

### Observations
- No explicit rate limiting detected during testing
- Typical response time: 500-1500ms per page
- Recommended: Add 100-200ms delay between requests

### Headers Sent
```
User-Agent: Go-http-client/1.1
```

Consider adding:
```
User-Agent: BadOps/0.2.0 (Bad.no Product Tool)
Accept: text/html,application/xhtml+xml
```

## Known Issues

### 1. Category Page vs Product Page
The scraper currently fetches category pages, which return images for **multiple products**. For more accurate results, should fetch individual product detail pages.

### 2. Image Deduplication
Currently compares by count only. Two different images from Tiger.nl might duplicate existing bad.no images. Future: implement perceptual hashing.

### 3. Some URLs Return 404
Not all extracted PIM URLs are valid. Some may be:
- Removed products
- Regional restrictions
- Cached HTML with stale references

### 4. Series Matching
Products in certain series (2-Store, Carv) have lower match rates because their names don't follow the standard "{Series} {Type}" pattern.

## API Endpoints (Not Used)

Tiger.nl appears to have some API endpoints (observed in network traffic):

```
/umbraco/api/productfilter/getproducts
/umbraco/api/search/search
```

These are not currently used but could provide more structured data.

## Recommendations

### For Better Matching
1. Maintain a mapping table of bad.no SKU → Tiger.nl product ID
2. Use EAN/barcode matching when available
3. Implement fuzzy string matching with higher thresholds

### For Production Use
1. Add rate limiting (max 1 req/sec)
2. Implement retry with exponential backoff
3. Cache Tiger.nl responses (15-minute TTL)
4. Log all requests for debugging
5. Add User-Agent header identifying the tool

### For Image Quality
1. Always request `width=1200` or higher
2. Use `format=jpg` for compatibility
3. Use `quality=90` for balance of size/quality
4. Consider downloading original size and resizing locally
