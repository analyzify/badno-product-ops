-- Rollback migration 001: Initial Schema

-- Drop triggers first
DROP TRIGGER IF EXISTS update_products_updated_at ON products;
DROP TRIGGER IF EXISTS update_competitors_updated_at ON competitors;
DROP TRIGGER IF EXISTS update_competitor_products_updated_at ON competitor_products;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS operation_history;
DROP TABLE IF EXISTS enhancement_log;
DROP TABLE IF EXISTS package_info;
DROP TABLE IF EXISTS product_suppliers;
DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS product_properties;
DROP TABLE IF EXISTS product_images;
DROP TABLE IF EXISTS price_observations_default;
DROP TABLE IF EXISTS price_observations;
DROP TABLE IF EXISTS competitor_products;
DROP TABLE IF EXISTS competitors;
DROP TABLE IF EXISTS products;

-- Drop extension (optional, may be used by other databases)
-- DROP EXTENSION IF EXISTS "uuid-ossp";
