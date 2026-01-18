package output

import (
	"context"
	"fmt"
	"sync"
)

// Registry manages available output adapters
type Registry struct {
	adapters map[string]Adapter
	mu       sync.RWMutex
}

// NewRegistry creates a new adapter registry
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an adapter to the registry
func (r *Registry) Register(adapter Adapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := adapter.Name()
	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("adapter already registered: %s", name)
	}

	r.adapters[name] = adapter
	return nil
}

// Get retrieves an adapter by name
func (r *Registry) Get(name string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapter, exists := r.adapters[name]
	if !exists {
		return nil, fmt.Errorf("adapter not found: %s", name)
	}

	return adapter, nil
}

// List returns all registered adapters
func (r *Registry) List() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapters := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		adapters = append(adapters, a)
	}
	return adapters
}

// ListByFormat returns adapters that support a specific format
func (r *Registry) ListByFormat(format Format) []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var adapters []Adapter
	for _, a := range r.adapters {
		if a.SupportsFormat(format) {
			adapters = append(adapters, a)
		}
	}
	return adapters
}

// ConnectAll connects to all registered adapters
func (r *Registry) ConnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, adapter := range r.adapters {
		if err := adapter.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect %s: %w", name, err)
		}
	}
	return nil
}

// CloseAll closes all registered adapters
func (r *Registry) CloseAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var lastErr error
	for name, adapter := range r.adapters {
		if err := adapter.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close %s: %w", name, err)
		}
	}
	return lastErr
}

// TestAll tests connectivity to all registered adapters
func (r *Registry) TestAll(ctx context.Context) map[string]error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string]error)
	for name, adapter := range r.adapters {
		results[name] = adapter.Test(ctx)
	}
	return results
}

// DefaultRegistry is the global adapter registry
var DefaultRegistry = NewRegistry()

// Register adds an adapter to the default registry
func Register(adapter Adapter) error {
	return DefaultRegistry.Register(adapter)
}

// Get retrieves an adapter from the default registry
func Get(name string) (Adapter, error) {
	return DefaultRegistry.Get(name)
}

// List returns all adapters from the default registry
func List() []Adapter {
	return DefaultRegistry.List()
}
