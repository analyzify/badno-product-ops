package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/badno/badops/internal/database"
	"github.com/badno/badops/internal/database/postgres"
	"github.com/google/uuid"
)

// SyncResult contains the results of a sync operation
type SyncResult struct {
	RecordsSynced int
	StartTime     time.Time
	EndTime       time.Time
	Errors        []string
}

// Syncer handles data synchronization from PostgreSQL to ClickHouse
type Syncer struct {
	pgClient *postgres.Client
	chClient *Client
}

// NewSyncer creates a new syncer
func NewSyncer(pgClient *postgres.Client, chClient *Client) *Syncer {
	return &Syncer{
		pgClient: pgClient,
		chClient: chClient,
	}
}

// SyncPriceObservations syncs price observations from PostgreSQL to ClickHouse
func (s *Syncer) SyncPriceObservations(ctx context.Context, since time.Time) (*SyncResult, error) {
	result := &SyncResult{
		StartTime: time.Now(),
	}

	// Get price observations from PostgreSQL
	query := `
		SELECT
			po.product_id,
			p.sku,
			p.barcode,
			c.name as competitor_name,
			po.price,
			po.currency,
			po.in_stock,
			po.stock_quantity,
			po.observed_at,
			po.source
		FROM price_observations po
		JOIN products p ON po.product_id = p.id
		JOIN competitors c ON po.competitor_id = c.id
		WHERE po.observed_at >= $1
		ORDER BY po.observed_at
	`

	rows, err := s.pgClient.Pool().Query(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query PostgreSQL: %w", err)
	}
	defer rows.Close()

	var records []PriceHistoryRecord
	for rows.Next() {
		var r PriceHistoryRecord
		var productID uuid.UUID
		var inStock bool

		err := rows.Scan(
			&productID,
			&r.ProductSKU,
			&r.ProductBarcode,
			&r.CompetitorName,
			&r.Price,
			&r.Currency,
			&inStock,
			&r.StockQuantity,
			&r.ObservedAt,
			&r.Source,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("scan error: %v", err))
			continue
		}

		r.InStock = inStock
		records = append(records, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	if len(records) == 0 {
		result.EndTime = time.Now()
		return result, nil
	}

	// Insert into ClickHouse in batches
	batchSize := 10000
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}

		batch := records[i:end]
		if err := s.chClient.InsertPriceHistory(ctx, batch); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("batch insert error: %v", err))
			continue
		}

		result.RecordsSynced += len(batch)
	}

	result.EndTime = time.Now()
	return result, nil
}

// SyncAll syncs all historical data from PostgreSQL to ClickHouse
func (s *Syncer) SyncAll(ctx context.Context) (*SyncResult, error) {
	// Sync from the beginning of time
	return s.SyncPriceObservations(ctx, time.Time{})
}

// SyncRecent syncs recent data (last N days)
func (s *Syncer) SyncRecent(ctx context.Context, days int) (*SyncResult, error) {
	since := time.Now().AddDate(0, 0, -days)
	return s.SyncPriceObservations(ctx, since)
}

// GetLastSyncTime returns the timestamp of the most recent synced record
func (s *Syncer) GetLastSyncTime(ctx context.Context) (time.Time, error) {
	var lastTime time.Time
	query := "SELECT max(observed_at) FROM price_history"
	if err := s.chClient.conn.QueryRow(ctx, query).Scan(&lastTime); err != nil {
		// If table is empty or doesn't exist, return zero time
		return time.Time{}, nil
	}
	return lastTime, nil
}

// SyncIncremental syncs only new data since the last sync
func (s *Syncer) SyncIncremental(ctx context.Context) (*SyncResult, error) {
	lastSync, err := s.GetLastSyncTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get last sync time: %w", err)
	}

	// Add a small buffer to avoid missing records
	if !lastSync.IsZero() {
		lastSync = lastSync.Add(-1 * time.Minute)
	}

	return s.SyncPriceObservations(ctx, lastSync)
}

// GetSyncStats returns statistics about synced data
type SyncStats struct {
	TotalPGRecords int64
	TotalCHRecords uint64
	OldestPGRecord time.Time
	NewestPGRecord time.Time
	OldestCHRecord time.Time
	NewestCHRecord time.Time
}

// GetSyncStats returns sync statistics
func (s *Syncer) GetSyncStats(ctx context.Context) (*SyncStats, error) {
	stats := &SyncStats{}

	// PostgreSQL stats
	pgRepo := postgres.NewPriceObservationRepo(s.pgClient)
	pgCount, err := pgRepo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count PG records: %w", err)
	}
	stats.TotalPGRecords = pgCount

	// Get PG date range
	var oldest, newest *time.Time
	s.pgClient.Pool().QueryRow(ctx, "SELECT MIN(observed_at), MAX(observed_at) FROM price_observations").Scan(&oldest, &newest)
	if oldest != nil {
		stats.OldestPGRecord = *oldest
	}
	if newest != nil {
		stats.NewestPGRecord = *newest
	}

	// ClickHouse stats
	chCount, err := s.chClient.GetObservationCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count CH records: %w", err)
	}
	stats.TotalCHRecords = chCount

	// Get CH date range
	s.chClient.conn.QueryRow(ctx, "SELECT min(observed_at), max(observed_at) FROM price_history").Scan(&stats.OldestCHRecord, &stats.NewestCHRecord)

	return stats, nil
}

// ConvertPGObservationsToRecords converts PostgreSQL observations to ClickHouse records
func ConvertPGObservationsToRecords(
	observations []*database.PriceObservation,
	productMap map[uuid.UUID]struct{ SKU, Barcode string },
	competitorMap map[int]string,
) []PriceHistoryRecord {
	records := make([]PriceHistoryRecord, 0, len(observations))

	for _, obs := range observations {
		product, ok := productMap[obs.ProductID]
		if !ok {
			continue
		}

		competitor, ok := competitorMap[obs.CompetitorID]
		if !ok {
			continue
		}

		r := PriceHistoryRecord{
			ProductSKU:     product.SKU,
			CompetitorName: competitor,
			Price:          obs.Price,
			Currency:       obs.Currency,
			InStock:        obs.InStock,
			ObservedAt:     obs.ObservedAt,
			Source:         obs.Source,
		}

		if product.Barcode != "" {
			r.ProductBarcode = &product.Barcode
		}

		if obs.StockQuantity != nil {
			qty := int32(*obs.StockQuantity)
			r.StockQuantity = &qty
		}

		records = append(records, r)
	}

	return records
}
