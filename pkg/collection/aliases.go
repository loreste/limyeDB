package collection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AliasManager manages collection aliases
type AliasManager struct {
	aliases map[string]string // alias -> collection name
	mu      sync.RWMutex
	dataDir string
}

// NewAliasManager creates a new alias manager
func NewAliasManager(dataDir string) (*AliasManager, error) {
	am := &AliasManager{
		aliases: make(map[string]string),
		dataDir: dataDir,
	}

	// Load existing aliases
	if err := am.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return am, nil
}

// Create creates a new alias for a collection
func (am *AliasManager) Create(alias, collectionName string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if alias already exists
	if existing, ok := am.aliases[alias]; ok && existing != collectionName {
		return ErrAliasExists
	}

	am.aliases[alias] = collectionName
	return am.save()
}

// Delete removes an alias
func (am *AliasManager) Delete(alias string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if _, ok := am.aliases[alias]; !ok {
		return ErrAliasNotFound
	}

	delete(am.aliases, alias)
	return am.save()
}

// Resolve returns the collection name for an alias
// If the input is not an alias, it returns the input unchanged
func (am *AliasManager) Resolve(nameOrAlias string) string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if collName, ok := am.aliases[nameOrAlias]; ok {
		return collName
	}
	return nameOrAlias
}

// Get returns the collection name for an alias
func (am *AliasManager) Get(alias string) (string, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	collName, ok := am.aliases[alias]
	return collName, ok
}

// List returns all aliases
func (am *AliasManager) List() map[string]string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make(map[string]string, len(am.aliases))
	for k, v := range am.aliases {
		result[k] = v
	}
	return result
}

// ListForCollection returns all aliases pointing to a specific collection
func (am *AliasManager) ListForCollection(collectionName string) []string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var aliases []string
	for alias, name := range am.aliases {
		if name == collectionName {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

// Rename updates all aliases pointing to oldName to point to newName
func (am *AliasManager) Rename(oldName, newName string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	changed := false
	for alias, name := range am.aliases {
		if name == oldName {
			am.aliases[alias] = newName
			changed = true
		}
	}

	if changed {
		return am.save()
	}
	return nil
}

// DeleteForCollection removes all aliases for a collection
func (am *AliasManager) DeleteForCollection(collectionName string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	changed := false
	for alias, name := range am.aliases {
		if name == collectionName {
			delete(am.aliases, alias)
			changed = true
		}
	}

	if changed {
		return am.save()
	}
	return nil
}

// Switch atomically switches an alias from one collection to another
// This is useful for blue-green deployments
func (am *AliasManager) Switch(alias, newCollectionName string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.aliases[alias] = newCollectionName
	return am.save()
}

// load loads aliases from disk
func (am *AliasManager) load() error {
	path := filepath.Join(am.dataDir, "aliases.json")
	// #nosec G304 - path is from internal dataDir, not user input
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &am.aliases)
}

// save persists aliases to disk
func (am *AliasManager) save() error {
	if err := os.MkdirAll(am.dataDir, 0750); err != nil {
		return err
	}

	path := filepath.Join(am.dataDir, "aliases.json")
	data, err := json.MarshalIndent(am.aliases, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// Alias errors
const (
	ErrAliasExists   CollectionError = "alias already exists"
	ErrAliasNotFound CollectionError = "alias not found"
)
