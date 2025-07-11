package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// MemoryStateStorage implements StateStorage using in-memory storage.
// This is primarily used for testing purposes.
type MemoryStateStorage struct {
	data  map[string]any
	mutex sync.RWMutex
}

// NewMemoryStateStorage creates a new memory-based state storage.
func NewMemoryStateStorage() *MemoryStateStorage {
	return &MemoryStateStorage{
		data:  make(map[string]any),
		mutex: sync.RWMutex{},
	}
}

// Save saves a value to the memory storage.
func (m *MemoryStateStorage) Save(key string, value any) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.data[key] = value

	return nil
}

// Load loads a value from the memory storage.
func (m *MemoryStateStorage) Load(key string, dest any) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	value, exists := m.data[key]
	if !exists {
		return os.ErrNotExist
	}

	// Convert to JSON and back to properly handle type conversion
	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	if err := json.Unmarshal(jsonData, dest); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// Delete removes a key from the memory storage.
func (m *MemoryStateStorage) Delete(key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.data, key)

	return nil
}

// Exists checks if a key exists in the memory storage.
func (m *MemoryStateStorage) Exists(key string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.data[key]

	return exists
}

// Clear removes all data from the memory storage.
func (m *MemoryStateStorage) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.data = make(map[string]any)
}

// Size returns the number of items in the memory storage.
func (m *MemoryStateStorage) Size() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.data)
}

// Keys returns all keys in the memory storage.
func (m *MemoryStateStorage) Keys() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	keys := make([]string, 0, len(m.data))
	for key := range m.data {
		keys = append(keys, key)
	}

	return keys
}
