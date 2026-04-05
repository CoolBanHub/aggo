package memory

import (
	"fmt"
	"sync"
)

// ProviderFactory creates a new MemoryProvider instance from the given config.
type ProviderFactory func(config any) (MemoryProvider, error)

// Plugin represents a memory backend plugin that can be registered with the registry.
type Plugin struct {
	// ID is the unique identifier for the plugin (e.g. "builtin", "memu", "mem0").
	ID string
	// Factory creates a new MemoryProvider instance for this plugin.
	Factory ProviderFactory
}

// Registry manages registered memory backend plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]*Plugin),
	}
}

// Register adds a plugin to the registry.
// It returns an error if the plugin ID or factory is empty, or if a plugin
// with the same ID is already registered.
func (r *Registry) Register(plugin *Plugin) error {
	if plugin.ID == "" {
		return fmt.Errorf("memory: plugin ID must not be empty")
	}
	if plugin.Factory == nil {
		return fmt.Errorf("memory: plugin factory must not be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[plugin.ID]; exists {
		return fmt.Errorf("memory: plugin %q is already registered", plugin.ID)
	}

	r.plugins[plugin.ID] = plugin
	return nil
}

// MustRegister registers a plugin and panics if registration fails.
func (r *Registry) MustRegister(plugin *Plugin) {
	if err := r.Register(plugin); err != nil {
		panic(err)
	}
}

// Get returns the plugin with the given ID, or false if not found.
func (r *Registry) Get(id string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[id]
	return p, ok
}

// CreateProvider creates a new MemoryProvider using the plugin identified by pluginID.
func (r *Registry) CreateProvider(pluginID string, config any) (MemoryProvider, error) {
	r.mu.RLock()
	plugin, ok := r.plugins[pluginID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("memory: plugin %q is not registered", pluginID)
	}

	return plugin.Factory(config)
}

// ListPlugins returns the IDs of all registered plugins.
func (r *Registry) ListPlugins() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.plugins))
	for id := range r.plugins {
		ids = append(ids, id)
	}
	return ids
}

// globalRegistry is the default global registry instance.
var globalRegistry = NewRegistry()

// GlobalRegistry returns the global registry instance.
func GlobalRegistry() *Registry {
	return globalRegistry
}

// RegisterPlugin registers a plugin with the global registry.
func RegisterPlugin(plugin *Plugin) error {
	return globalRegistry.Register(plugin)
}

// MustRegisterPlugin registers a plugin with the global registry, panicking on error.
func MustRegisterPlugin(plugin *Plugin) {
	globalRegistry.MustRegister(plugin)
}
