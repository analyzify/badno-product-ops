package source

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages available source connectors
type Registry struct {
	connectors map[string]Connector
	mu         sync.RWMutex
}

// NewRegistry creates a new connector registry
func NewRegistry() *Registry {
	return &Registry{
		connectors: make(map[string]Connector),
	}
}

// Register adds a connector to the registry
func (r *Registry) Register(connector Connector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := connector.Name()
	if _, exists := r.connectors[name]; exists {
		return fmt.Errorf("connector already registered: %s", name)
	}

	r.connectors[name] = connector
	return nil
}

// Get retrieves a connector by name
func (r *Registry) Get(name string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	connector, exists := r.connectors[name]
	if !exists {
		return nil, fmt.Errorf("connector not found: %s", name)
	}

	return connector, nil
}

// List returns all registered connectors
func (r *Registry) List() []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	connectors := make([]Connector, 0, len(r.connectors))
	for _, c := range r.connectors {
		connectors = append(connectors, c)
	}
	return connectors
}

// ListByType returns connectors of a specific type
func (r *Registry) ListByType(connType ConnectorType) []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var connectors []Connector
	for _, c := range r.connectors {
		if c.Type() == connType {
			connectors = append(connectors, c)
		}
	}
	return connectors
}

// ListSources returns all source connectors
func (r *Registry) ListSources() []Connector {
	return r.ListByType(TypeSource)
}

// ListEnhancers returns all enhancement connectors
func (r *Registry) ListEnhancers() []Connector {
	return r.ListByType(TypeEnhancement)
}

// ConnectAll connects to all registered connectors
func (r *Registry) ConnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, connector := range r.connectors {
		if err := connector.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect %s: %w", name, err)
		}
	}
	return nil
}

// CloseAll closes all registered connectors
func (r *Registry) CloseAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var lastErr error
	for name, connector := range r.connectors {
		if err := connector.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close %s: %w", name, err)
		}
	}
	return lastErr
}

// TestAll tests connectivity to all registered connectors
func (r *Registry) TestAll(ctx context.Context) map[string]error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string]error)
	for name, connector := range r.connectors {
		results[name] = connector.Test(ctx)
	}
	return results
}

// DefaultRegistry is the global connector registry
var DefaultRegistry = NewRegistry()

// Register adds a connector to the default registry
func Register(connector Connector) error {
	return DefaultRegistry.Register(connector)
}

// Get retrieves a connector from the default registry
func Get(name string) (Connector, error) {
	return DefaultRegistry.Get(name)
}

// List returns all connectors from the default registry
func List() []Connector {
	return DefaultRegistry.List()
}
