package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileStateStorage implements StateStorage using file system.
type FileStateStorage struct {
	filePath string
	mutex    sync.RWMutex
}

// NewFileStateStorage creates a new file-based state storage.
func NewFileStateStorage(filePath string) *FileStateStorage {
	return &FileStateStorage{
		filePath: filePath,
		mutex:    sync.RWMutex{},
	}
}

// Save saves a value to the file storage.
func (f *FileStateStorage) Save(key string, value any) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(f.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Load existing data
	data := make(map[string]any)
	if f.fileExists() {
		if err := f.loadFromFile(&data); err != nil {
			return err
		}
	}

	// Update with new value
	data[key] = value

	// Save back to file
	return f.saveToFile(data)
}

// Load loads a value from the file storage.
func (f *FileStateStorage) Load(key string, dest any) error {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	if !f.fileExists() {
		return os.ErrNotExist
	}

	data := make(map[string]any)
	if err := f.loadFromFile(&data); err != nil {
		return err
	}

	value, exists := data[key]
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

// Delete removes a key from the file storage.
func (f *FileStateStorage) Delete(key string) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.fileExists() {
		return nil
	}

	data := make(map[string]any)
	if err := f.loadFromFile(&data); err != nil {
		return err
	}

	delete(data, key)

	if len(data) == 0 {
		if err := os.Remove(f.filePath); err != nil {
			return fmt.Errorf("failed to remove file: %w", err)
		}

		return nil
	}

	return f.saveToFile(data)
}

// Exists checks if a key exists in the file storage.
func (f *FileStateStorage) Exists(key string) bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	if !f.fileExists() {
		return false
	}

	data := make(map[string]any)
	if err := f.loadFromFile(&data); err != nil {
		return false
	}

	_, exists := data[key]

	return exists
}

// fileExists checks if the storage file exists.
func (f *FileStateStorage) fileExists() bool {
	_, err := os.Stat(f.filePath)

	return !os.IsNotExist(err)
}

// loadFromFile loads data from the file.
func (f *FileStateStorage) loadFromFile(data *map[string]any) error {
	file, err := os.Open(f.filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(data); err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	return nil
}

// saveToFile saves data to the file.
func (f *FileStateStorage) saveToFile(data map[string]any) error {
	file, err := os.Create(f.filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}
