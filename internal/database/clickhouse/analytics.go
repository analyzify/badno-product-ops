package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// PriceTrend represents price trend data for a product
type PriceTrend struct {
	ProductSKU     string
	CompetitorName string
	Date           time.Time
	MinPrice       float64
	MaxPrice       float64
	AvgPrice       float64
	Count          int64
}

// MarketPosition represents a product's market position
type MarketPosition struct {
	ProductSKU      string
	Date            time.Time
	MarketMin       float64
	MarketMax       float64
	MarketAvg       float64
	CompetitorCount int
}

// PriceAlert represents a price alert
type PriceAlert struct {
	ProductSKU     string
	CompetitorName string
	CurrentPrice   float64
	MarketAvg      float64
	Difference     float64
	DiffPercent    float64
}

// GetPriceTrends returns price trends for a product over time
func (c *Client) GetPriceTrends(ctx context.Context, productSKU string, days int) ([]PriceTrend, error) {
	since := time.Now().AddDate(0, 0, -days)

	query := `
		SELECT
			product_sku,
			competitor_name,
			toDate(observed_at) as date,
			min(price) as min_price,
			max(price) as max_price,
			avg(price) as avg_price,
			count() as count
		FROM price_history
		WHERE product_sku = ?
		  AND observed_at >= ?
		GROUP BY product_sku, competitor_name, date
		ORDER BY date, competitor_name
	`

	rows, err := c.conn.Query(ctx, query, productSKU, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query trends: %w", err)
	}
	defer rows.Close()

	var trends []PriceTrend
	for rows.Next() {
		var t PriceTrend
		if err := rows.Scan(&t.ProductSKU, &t.CompetitorName, &t.Date, &t.MinPrice, &t.MaxPrice, &t.AvgPrice, &t.Count); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		trends = append(trends, t)
	}

	return trends, rows.Err()
}

// GetVendorTrends returns price trends for all products from a vendor
func (c *Client) GetVendorTrends(ctx context.Context, vendor string, days int) ([]PriceTrend, error) {
	since := time.Now().AddDate(0, 0, -days)

	// This query joins with PostgreSQL data through ClickHouse's external tables
	// For now, we'll query based on SKU prefix if vendor products follow naming conventions
	query := `
		SELECT
			product_sku,
			competitor_name,
			toDate(observed_at) as date,
			min(price) as min_price,
			max(price) as max_price,
			avg(price) as avg_price,
			count() as count
		FROM price_history
		WHERE observed_at >= ?
		GROUP BY product_sku, competitor_name, date
		ORDER BY product_sku, date, competitor_name
		LIMIT 10000
	`

	rows, err := c.conn.Query(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query vendor trends: %w", err)
	}
	defer rows.Close()

	var trends []PriceTrend
	for rows.Next() {
		var t PriceTrend
		if err := rows.Scan(&t.ProductSKU, &t.CompetitorName, &t.Date, &t.MinPrice, &t.MaxPrice, &t.AvgPrice, &t.Count); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		trends = append(trends, t)
	}

	return trends, rows.Err()
}

// GetMarketPositions returns market positions for products
func (c *Client) GetMarketPositions(ctx context.Context, skus []string, days int) ([]MarketPosition, error) {
	since := time.Now().AddDate(0, 0, -days)

	query := `
		SELECT
			product_sku,
			toDate(observed_at) as date,
			min(price) as market_min,
			max(price) as market_max,
			avg(price) as market_avg,
			count(DISTINCT competitor_name) as competitor_count
		FROM price_history
		WHERE observed_at >= ?
		GROUP BY product_sku, date
		ORDER BY product_sku, date
	`

	if len(skus) > 0 {
		query = `
			SELECT
				product_sku,
				toDate(observed_at) as date,
				min(price) as market_min,
				max(price) as market_max,
				avg(price) as market_avg,
				count(DISTINCT competitor_name) as competitor_count
			FROM price_history
			WHERE product_sku IN (?)
			  AND observed_at >= ?
			GROUP BY product_sku, date
			ORDER BY product_sku, date
		`
	}

	var rows driver.Rows
	var err error

	if len(skus) > 0 {
		rows, err = c.conn.Query(ctx, query, skus, since)
	} else {
		rows, err = c.conn.Query(ctx, query, since)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query positions: %w", err)
	}
	defer rows.Close()

	var positions []MarketPosition
	for rows.Next() {
		var p MarketPosition
		if err := rows.Scan(&p.ProductSKU, &p.Date, &p.MarketMin, &p.MarketMax, &p.MarketAvg, &p.CompetitorCount); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		positions = append(positions, p)
	}

	return positions, rows.Err()
}

// GetPriceAlerts returns products where our price differs significantly from market
func (c *Client) GetPriceAlerts(ctx context.Context, thresholdPercent float64, ownPrices map[string]float64) ([]PriceAlert, error) {
	// Get latest market averages
	query := `
		SELECT
			product_sku,
			competitor_name,
			argMax(price, observed_at) as current_price
		FROM price_history
		WHERE observed_at >= now() - INTERVAL 7 DAY
		GROUP BY product_sku, competitor_name
	`

	rows, err := c.conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query prices: %w", err)
	}
	defer rows.Close()

	// Collect prices per product
	productPrices := make(map[string][]float64)
	for rows.Next() {
		var sku, competitor string
		var price float64
		if err := rows.Scan(&sku, &competitor, &price); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		productPrices[sku] = append(productPrices[sku], price)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Calculate alerts
	var alerts []PriceAlert
	for sku, prices := range productPrices {
		ownPrice, hasOwnPrice := ownPrices[sku]
		if !hasOwnPrice {
			continue
		}

		// Calculate market average
		var sum float64
		for _, p := range prices {
			sum += p
		}
		marketAvg := sum / float64(len(prices))

		diff := ownPrice - marketAvg
		diffPercent := (diff / marketAvg) * 100

		// Check if above threshold
		if diffPercent > thresholdPercent || diffPercent < -thresholdPercent {
			alerts = append(alerts, PriceAlert{
				ProductSKU:  sku,
				CurrentPrice: ownPrice,
				MarketAvg:   marketAvg,
				Difference:  diff,
				DiffPercent: diffPercent,
			})
		}
	}

	return alerts, nil
}

// GetPriceDistribution returns the distribution of prices across competitors
func (c *Client) GetPriceDistribution(ctx context.Context, productSKU string) (map[string]float64, error) {
	query := `
		SELECT
			competitor_name,
			argMax(price, observed_at) as latest_price
		FROM price_history
		WHERE product_sku = ?
		  AND observed_at >= now() - INTERVAL 7 DAY
		GROUP BY competitor_name
		ORDER BY latest_price
	`

	rows, err := c.conn.Query(ctx, query, productSKU)
	if err != nil {
		return nil, fmt.Errorf("failed to query distribution: %w", err)
	}
	defer rows.Close()

	distribution := make(map[string]float64)
	for rows.Next() {
		var competitor string
		var price float64
		if err := rows.Scan(&competitor, &price); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		distribution[competitor] = price
	}

	return distribution, rows.Err()
}

// PriceHistoryRecord represents a single price history record
type PriceHistoryRecord struct {
	ProductSKU     string
	ProductBarcode *string
	CompetitorName string
	Price          float64
	Currency       string
	InStock        bool
	StockQuantity  *int32
	ObservedAt     time.Time
	Source         string
}

// InsertPriceHistory inserts price records into ClickHouse
func (c *Client) InsertPriceHistory(ctx context.Context, records []PriceHistoryRecord) error {
	if len(records) == 0 {
		return nil
	}

	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO price_history (
			product_sku, product_barcode, competitor_name,
			price, currency, in_stock, stock_quantity,
			observed_at, observed_date, source
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, r := range records {
		var inStock uint8
		if r.InStock {
			inStock = 1
		}

		observedDate := r.ObservedAt.Truncate(24 * time.Hour)

		err := batch.Append(
			r.ProductSKU,
			r.ProductBarcode,
			r.CompetitorName,
			r.Price,
			r.Currency,
			inStock,
			r.StockQuantity,
			r.ObservedAt,
			observedDate,
			r.Source,
		)
		if err != nil {
			return fmt.Errorf("failed to append to batch: %w", err)
		}
	}

	return batch.Send()
}

// GetObservationCount returns the total number of price observations
func (c *Client) GetObservationCount(ctx context.Context) (uint64, error) {
	var count uint64
	if err := c.conn.QueryRow(ctx, "SELECT count() FROM price_history").Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count observations: %w", err)
	}
	return count, nil
}
