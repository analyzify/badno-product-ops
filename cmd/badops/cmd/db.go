package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/badno/badops/internal/config"
	"github.com/badno/badops/internal/database"
	"github.com/badno/badops/internal/database/postgres"
	"github.com/badno/badops/internal/state"
	"github.com/badno/badops/pkg/models"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management commands",
	Long:  "Commands for managing the PostgreSQL database backend",
}

var dbInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the database schema",
	Long:  "Creates all required tables and indexes in the PostgreSQL database",
	RunE:  runDBInit,
}

var dbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show database status",
	Long:  "Shows connection status, table counts, and database health information",
	RunE:  runDBStatus,
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate data from JSON state to database",
	Long:  "Imports products and history from the JSON state file into the database",
	RunE:  runDBMigrate,
}

var (
	migrateFromState string
	migrateForce     bool
)

func init() {
	dbCmd.AddCommand(dbInitCmd)
	dbCmd.AddCommand(dbStatusCmd)
	dbCmd.AddCommand(dbMigrateCmd)

	dbMigrateCmd.Flags().StringVar(&migrateFromState, "from-state", "", "Path to JSON state file (default: output/.badops-state.json)")
	dbMigrateCmd.Flags().BoolVar(&migrateForce, "force", false, "Force migration even if products already exist in database")
}

// getDBClient creates a PostgreSQL client from configuration
func getDBClient() (*postgres.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	pgConfig := &postgres.Config{
		Host:     cfg.Database.Postgres.Host,
		Port:     cfg.Database.Postgres.Port,
		Database: cfg.Database.Postgres.Database,
		Username: os.Getenv(cfg.Database.Postgres.UsernameEnv),
		Password: os.Getenv(cfg.Database.Postgres.PasswordEnv),
		SSLMode:  cfg.Database.Postgres.SSLMode,
	}

	if pgConfig.Username == "" {
		return nil, fmt.Errorf("PostgreSQL username not set. Set the %s environment variable", cfg.Database.Postgres.UsernameEnv)
	}

	return postgres.NewClient(pgConfig), nil
}

func runDBInit(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := getDBClient()
	if err != nil {
		return err
	}

	fmt.Println("Connecting to PostgreSQL...")
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	color.Green("✓ Connected to database")

	fmt.Println("Running migrations...")
	if err := client.RunMigrations(); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	color.Green("✓ Database schema initialized")

	// Show migration version
	version, dirty, err := client.MigrationVersion()
	if err != nil {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	fmt.Printf("\nMigration version: %d", version)
	if dirty {
		color.Yellow(" (dirty)")
	}
	fmt.Println()

	// Show table summary
	stats, err := client.GetTableStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get table stats: %w", err)
	}

	fmt.Println("\nCreated tables:")
	for _, s := range stats {
		fmt.Printf("  • %s\n", s.TableName)
	}

	color.Green("\n✓ Database initialization complete")
	return nil
}

func runDBStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := getDBClient()
	if err != nil {
		return err
	}

	fmt.Println("Checking database connection...")
	if err := client.Connect(ctx); err != nil {
		color.Red("✗ Connection failed: %v", err)
		return nil
	}
	defer client.Close()

	color.Green("✓ Connected")

	// Get database info
	info, err := client.GetDatabaseInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get database info: %w", err)
	}

	fmt.Println("\n" + color.CyanString("Database Information"))
	fmt.Printf("  Database:    %s\n", info.DatabaseName)
	fmt.Printf("  Size:        %s\n", info.DatabaseSize)
	fmt.Printf("  Connections: %d/%d\n", info.ConnectionsNow, info.ConnectionsMax)

	// Get migration version
	version, dirty, err := client.MigrationVersion()
	if err != nil {
		fmt.Printf("  Migration:   %s\n", color.YellowString("not initialized"))
	} else {
		status := fmt.Sprintf("v%d", version)
		if dirty {
			status += color.YellowString(" (dirty)")
		}
		fmt.Printf("  Migration:   %s\n", status)
	}

	// Get table statistics
	stats, err := client.GetTableStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get table stats: %w", err)
	}

	if len(stats) > 0 {
		fmt.Println("\n" + color.CyanString("Table Statistics"))

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Table", "Rows", "Size"})
		table.SetBorder(false)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)

		for _, s := range stats {
			table.Append([]string{s.TableName, fmt.Sprintf("%d", s.RowCount), s.Size})
		}
		table.Render()
	}

	// Show pool stats
	poolStats := client.Stats()
	if poolStats != nil {
		fmt.Println("\n" + color.CyanString("Connection Pool"))
		fmt.Printf("  Total conns:      %d\n", poolStats.TotalConns())
		fmt.Printf("  Idle conns:       %d\n", poolStats.IdleConns())
		fmt.Printf("  Acquired conns:   %d\n", poolStats.AcquiredConns())
	}

	return nil
}

func runDBMigrate(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Load state file
	statePath := migrateFromState
	if statePath == "" {
		statePath = state.DefaultStateFile
	}

	fmt.Printf("Loading state from: %s\n", statePath)
	store := state.NewStore(statePath)
	if err := store.Load(); err != nil {
		return fmt.Errorf("failed to load state file: %w", err)
	}

	products := store.GetAllProducts()
	if len(products) == 0 {
		color.Yellow("No products found in state file")
		return nil
	}

	fmt.Printf("Found %d products to migrate\n", len(products))

	// Connect to database
	client, err := getDBClient()
	if err != nil {
		return err
	}

	fmt.Println("Connecting to PostgreSQL...")
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	color.Green("✓ Connected")

	// Check if products already exist
	productRepo := postgres.NewProductRepo(client)
	existingCount, err := productRepo.Count(ctx)
	if err != nil {
		return fmt.Errorf("failed to count existing products: %w", err)
	}

	if existingCount > 0 && !migrateForce {
		color.Yellow("Database already contains %d products", existingCount)
		fmt.Println("Use --force to migrate anyway (will merge with existing data)")
		return nil
	}

	// Migrate products
	fmt.Println("\nMigrating products...")
	count, err := productRepo.BulkUpsert(ctx, products)
	if err != nil {
		return fmt.Errorf("failed to migrate products: %w", err)
	}

	color.Green("✓ Migrated %d products", count)

	// Migrate images
	imageRepo := postgres.NewImageRepo(client)
	var totalImages int
	for _, p := range products {
		if len(p.Images) > 0 {
			dbImages := make([]*database.ProductImage, 0, len(p.Images))
			for _, img := range p.Images {
				dbImg := convertToDBImage(p.ID, &img)
				dbImages = append(dbImages, dbImg)
			}
			imgCount, err := imageRepo.BulkUpsert(ctx, dbImages)
			if err != nil {
				color.Yellow("Warning: failed to migrate images for %s: %v", p.SKU, err)
				continue
			}
			totalImages += imgCount
		}
	}
	if totalImages > 0 {
		color.Green("✓ Migrated %d images", totalImages)
	}

	// Migrate properties
	propertyRepo := postgres.NewPropertyRepo(client)
	var totalProps int
	for _, p := range products {
		if len(p.Properties) > 0 {
			dbProps := make([]*database.ProductProperty, 0, len(p.Properties))
			for _, prop := range p.Properties {
				dbProp := convertToDBProperty(p.ID, &prop)
				dbProps = append(dbProps, dbProp)
			}
			propCount, err := propertyRepo.BulkUpsert(ctx, dbProps)
			if err != nil {
				color.Yellow("Warning: failed to migrate properties for %s: %v", p.SKU, err)
				continue
			}
			totalProps += propCount
		}
	}
	if totalProps > 0 {
		color.Green("✓ Migrated %d properties", totalProps)
	}

	// Migrate history
	historyRepo := postgres.NewHistoryRepo(client)
	history := store.GetHistory()
	for _, h := range history {
		entry := &database.OperationHistory{
			Action:    h.Action,
			Source:    h.Source,
			Count:     h.Count,
			Details:   h.Details,
			StartedAt: h.Timestamp,
		}
		completed := h.Timestamp
		entry.CompletedAt = &completed
		if err := historyRepo.Add(ctx, entry); err != nil {
			color.Yellow("Warning: failed to migrate history entry: %v", err)
		}
	}
	if len(history) > 0 {
		color.Green("✓ Migrated %d history entries", len(history))
	}

	// Summary
	fmt.Println("\n" + color.CyanString("Migration Summary"))
	fmt.Printf("  Products:   %d\n", count)
	fmt.Printf("  Images:     %d\n", totalImages)
	fmt.Printf("  Properties: %d\n", totalProps)
	fmt.Printf("  History:    %d\n", len(history))

	color.Green("\n✓ Migration complete")
	fmt.Println("\nTo enable database backend, run:")
	fmt.Println("  badops config set database.use_db true")

	return nil
}

// Helper functions for converting models
func convertToDBImage(productID string, img *models.ProductImage) *database.ProductImage {
	dbImg := &database.ProductImage{
		SourceURL:    img.SourceURL,
		Source:       img.Source,
		LocalPath:    img.LocalPath,
		Width:        img.Width,
		Height:       img.Height,
		Position:     img.Position,
		AltText:      img.Alt,
		Status:       img.Status,
		ResizedPaths: img.ResizedPaths,
	}

	if productID != "" {
		dbImg.ProductID, _ = uuid.Parse(productID)
	}
	if img.ID != "" {
		dbImg.ID, _ = uuid.Parse(img.ID)
	}
	if !img.DownloadedAt.IsZero() {
		dbImg.DownloadedAt = &img.DownloadedAt
	}

	return dbImg
}

func convertToDBProperty(productID string, prop *models.Property) *database.ProductProperty {
	dbProp := &database.ProductProperty{
		Code:   prop.Code,
		Name:   prop.Name,
		Value:  prop.Value,
		Unit:   prop.Unit,
		Source: prop.Source,
	}

	if productID != "" {
		dbProp.ProductID, _ = uuid.Parse(productID)
	}

	return dbProp
}
