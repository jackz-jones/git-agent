package storage

import (
	"os"
	"path/filepath"
)

// Storage handles local file storage for Git agent.
type Storage struct {
	basePath string
}

// New creates a new Storage.
func New(basePath string) (*Storage, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}

	// Create base path if it doesn't exist
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, err
	}

	return &Storage{
		basePath: absPath,
	}, nil
}

// Get retrieves a file from storage.
func (s *Storage) Get(path string) ([]byte, error) {
	fullPath := filepath.Join(s.basePath, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Put stores a file to storage.
func (s *Storage) Put(path string, data []byte) error {
	fullPath := filepath.Join(s.basePath, path)
	err := os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err != nil {
		return err
	}
	return os.WriteFile(fullPath, data, 0644)
}

// Delete removes a file from storage.
func (s *Storage) Delete(path string) error {
	fullPath := filepath.Join(s.basePath, path)
	return os.Remove(fullPath)
}

// Exists checks if a file exists.
func (s *Storage) Exists(path string) (bool, error) {
	fullPath := filepath.Join(s.basePath, path)
	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
