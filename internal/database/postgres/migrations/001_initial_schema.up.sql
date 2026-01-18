-- Bad.no Operations Database Schema
-- Migration 001: Initial Schema

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Products table (core product data)
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sku VARCHAR(100) NOT NULL UNIQUE,
    handle VARCHAR(255),
    barcode VARCHAR(100),
    nobb_number VARCHAR(50),

    -- Content
    title VARCHAR(500) NOT NULL,
    description TEXT,
    vendor VARCHAR(200),
    product_type VARCHAR(200),
    tags TEXT[],

    -- Pricing
    price DECIMAL(12, 2),
    cost DECIMAL(12, 2),
    compare_at_price DECIMAL(12, 2),
    currency VARCHAR(3) DEFAULT 'NOK',
    profit_margin DECIMAL(5, 2),

    -- Physical attributes
    weight_value DECIMAL(10, 3),
    weight_unit VARCHAR(10),
    length_mm DECIMAL(10, 2),
    width_mm DECIMAL(10, 2),
    height_mm DECIMAL(10, 2),

    -- Status and metadata
    status VARCHAR(50) DEFAULT 'pending',
    specifications JSONB DEFAULT '{}',

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_price_check TIMESTAMP WITH TIME ZONE,

    -- Legacy fields for migration compatibility
    legacy_matched_url TEXT,
    legacy_match_score DECIMAL(5, 4)
);

-- Indexes for products
CREATE INDEX idx_products_sku ON products(sku);
CREATE INDEX idx_products_barcode ON products(barcode) WHERE barcode IS NOT NULL;
CREATE INDEX idx_products_nobb ON products(nobb_number) WHERE nobb_number IS NOT NULL;
CREATE INDEX idx_products_vendor ON products(vendor);
CREATE INDEX idx_products_status ON products(status);
CREATE INDEX idx_products_updated ON products(updated_at);

-- Competitors table
CREATE TABLE competitors (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL UNIQUE,
    normalized_name VARCHAR(200) NOT NULL,
    website VARCHAR(500),
    scrape_enabled BOOLEAN DEFAULT false,
    scrape_config JSONB DEFAULT '{}',
    product_count INTEGER DEFAULT 0,
    last_scraped TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_competitors_name ON competitors(normalized_name);

-- Competitor products (links products to competitor listings)
CREATE TABLE competitor_products (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    competitor_id INTEGER NOT NULL REFERENCES competitors(id) ON DELETE CASCADE,
    url TEXT,
    competitor_sku VARCHAR(200),
    competitor_title VARCHAR(500),
    is_active BOOLEAN DEFAULT true,
    match_method VARCHAR(50),
    match_confidence DECIMAL(5, 4) DEFAULT 1.0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (product_id, competitor_id)
);

CREATE INDEX idx_competitor_products_competitor ON competitor_products(competitor_id);
CREATE INDEX idx_competitor_products_active ON competitor_products(is_active);

-- Price observations (partitioned by date for efficient retention)
CREATE TABLE price_observations (
    id BIGSERIAL,
    product_id UUID NOT NULL,
    competitor_id INTEGER NOT NULL,
    price DECIMAL(12, 2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'NOK',
    in_stock BOOLEAN DEFAULT true,
    stock_quantity INTEGER,
    observed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    observed_date DATE NOT NULL DEFAULT CURRENT_DATE,
    source VARCHAR(50) DEFAULT 'reprice_csv',
    PRIMARY KEY (id, observed_date)
) PARTITION BY RANGE (observed_date);

-- Create partitions for 3 months (can add more as needed)
CREATE TABLE price_observations_default PARTITION OF price_observations DEFAULT;

-- Indexes for price observations
CREATE INDEX idx_price_obs_product ON price_observations(product_id);
CREATE INDEX idx_price_obs_competitor ON price_observations(competitor_id);
CREATE INDEX idx_price_obs_date ON price_observations(observed_date);
CREATE INDEX idx_price_obs_lookup ON price_observations(product_id, competitor_id, observed_date);

-- Product images
CREATE TABLE product_images (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    source_url TEXT NOT NULL,
    source VARCHAR(50) NOT NULL,
    local_path TEXT,
    width INTEGER,
    height INTEGER,
    position INTEGER DEFAULT 1,
    alt_text VARCHAR(500),
    status VARCHAR(50) DEFAULT 'pending',
    resized_paths JSONB DEFAULT '{}',
    downloaded_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_product_images_product ON product_images(product_id);
CREATE INDEX idx_product_images_status ON product_images(status);

-- Product properties (from NOBB, Tiger.nl, etc.)
CREATE TABLE product_properties (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(300) NOT NULL,
    value TEXT NOT NULL,
    unit VARCHAR(50),
    source VARCHAR(50) NOT NULL,
    PRIMARY KEY (product_id, code, source)
);

CREATE INDEX idx_product_properties_product ON product_properties(product_id);

-- Suppliers
CREATE TABLE suppliers (
    id VARCHAR(100) PRIMARY KEY,
    name VARCHAR(300) NOT NULL,
    gln VARCHAR(20),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Product suppliers (many-to-many relationship)
CREATE TABLE product_suppliers (
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    supplier_id VARCHAR(100) NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    article_number VARCHAR(100),
    is_primary BOOLEAN DEFAULT false,
    PRIMARY KEY (product_id, supplier_id)
);

CREATE INDEX idx_product_suppliers_supplier ON product_suppliers(supplier_id);

-- Package info
CREATE TABLE package_info (
    id SERIAL PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    package_type VARCHAR(50) NOT NULL,
    quantity INTEGER DEFAULT 1,
    gtin VARCHAR(20),
    weight DECIMAL(10, 3),
    weight_unit VARCHAR(10),
    length DECIMAL(10, 2),
    width DECIMAL(10, 2),
    height DECIMAL(10, 2),
    dim_unit VARCHAR(10)
);

CREATE INDEX idx_package_info_product ON package_info(product_id);

-- Enhancement log (audit trail for enhancements)
CREATE TABLE enhancement_log (
    id BIGSERIAL PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    source VARCHAR(50) NOT NULL,
    action VARCHAR(100) NOT NULL,
    fields_added TEXT[],
    success BOOLEAN DEFAULT true,
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_enhancement_log_product ON enhancement_log(product_id);
CREATE INDEX idx_enhancement_log_source ON enhancement_log(source);

-- Operation history (global operation log)
CREATE TABLE operation_history (
    id BIGSERIAL PRIMARY KEY,
    action VARCHAR(100) NOT NULL,
    source VARCHAR(100),
    count INTEGER DEFAULT 0,
    details TEXT,
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_operation_history_action ON operation_history(action);
CREATE INDEX idx_operation_history_started ON operation_history(started_at);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply updated_at trigger to relevant tables
CREATE TRIGGER update_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_competitors_updated_at
    BEFORE UPDATE ON competitors
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_competitor_products_updated_at
    BEFORE UPDATE ON competitor_products
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
