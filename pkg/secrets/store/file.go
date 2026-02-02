package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/encryption"
)

// FileStore stores secrets as encrypted files on disk.
// Each secret is stored as a JSON file encrypted with age.
type FileStore struct {
	basePath  string
	encryptor encryption.Encryptor
	watchers  map[string][]chan *secrets.Secret
	mu        sync.RWMutex
	log       *logger.Logger
	closed    bool
	closeCh   chan struct{}
	pollInterval time.Duration
}

// FileStoreConfig configures the file store.
type FileStoreConfig struct {
	// BasePath is the directory where secrets are stored.
	BasePath string

	// Encryptor encrypts/decrypts secret data.
	Encryptor encryption.Encryptor

	// Logger is the logger to use.
	Logger *logger.Logger

	// PollInterval is how often to check for file changes (for watching).
	PollInterval time.Duration
}

// NewFileStore creates a new file-based secret store.
func NewFileStore(config FileStoreConfig) (*FileStore, error) {
	if config.BasePath == "" {
		return nil, errors.New("base path is required")
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(config.BasePath, 0700); err != nil {
		return nil, fmt.Errorf("create base directory: %w", err)
	}

	log := config.Logger
	if log == nil {
		log = logger.New(io.Discard)
	}

	pollInterval := config.PollInterval
	if pollInterval == 0 {
		pollInterval = 5 * time.Second
	}

	fs := &FileStore{
		basePath:     config.BasePath,
		encryptor:    config.Encryptor,
		watchers:     make(map[string][]chan *secrets.Secret),
		log:          log.With().Str("store", "file").Logger(),
		closeCh:      make(chan struct{}),
		pollInterval: pollInterval,
	}

	// Start file watcher
	go fs.watchFiles()

	return fs, nil
}

// pathToFile converts a secret path to a file path.
func (s *FileStore) pathToFile(path string) string {
	// Replace path separators with double underscores to avoid directory issues
	safePath := strings.ReplaceAll(path, "/", "__")
	return filepath.Join(s.basePath, safePath+".json")
}

// fileToPath converts a file path back to a secret path.
func (s *FileStore) fileToPath(filePath string) string {
	name := filepath.Base(filePath)
	name = strings.TrimSuffix(name, ".json")
	return strings.ReplaceAll(name, "__", "/")
}

// Get retrieves a secret by path.
func (s *FileStore) Get(ctx context.Context, path string) (*secrets.Secret, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, secrets.ErrStoreClosed
	}
	s.mu.RUnlock()

	filePath := s.pathToFile(path)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, secrets.ErrSecretNotFound
		}
		return nil, fmt.Errorf("read secret file: %w", err)
	}

	// Decrypt if encryptor is configured
	if s.encryptor != nil {
		data, err = s.encryptor.Decrypt(data)
		if err != nil {
			return nil, fmt.Errorf("decrypt secret: %w", err)
		}
	}

	var secret secrets.Secret
	if err := json.Unmarshal(data, &secret); err != nil {
		return nil, fmt.Errorf("unmarshal secret: %w", err)
	}

	return &secret, nil
}

// Set stores a secret at the given path.
func (s *FileStore) Set(ctx context.Context, path string, secret *secrets.Secret) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return secrets.ErrStoreClosed
	}
	s.mu.RUnlock()

	filePath := s.pathToFile(path)

	data, err := json.MarshalIndent(secret, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal secret: %w", err)
	}

	// Encrypt if encryptor is configured
	if s.encryptor != nil {
		data, err = s.encryptor.Encrypt(data)
		if err != nil {
			return fmt.Errorf("encrypt secret: %w", err)
		}
	}

	// Write to temp file first, then rename for atomicity
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	// Notify watchers
	s.notifyWatchers(path, secret)

	s.log.Debug().Str("path", path).Str("file", filePath).Msg("secret stored to file")
	return nil
}

// Delete removes a secret by path.
func (s *FileStore) Delete(ctx context.Context, path string) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return secrets.ErrStoreClosed
	}
	s.mu.RUnlock()

	filePath := s.pathToFile(path)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return secrets.ErrSecretNotFound
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("remove secret file: %w", err)
	}

	// Notify watchers with nil (indicates deletion)
	s.notifyWatchers(path, nil)

	s.log.Debug().Str("path", path).Str("file", filePath).Msg("secret deleted from file")
	return nil
}

// List returns all secret paths matching the prefix.
func (s *FileStore) List(ctx context.Context, prefix string) ([]string, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, secrets.ErrStoreClosed
	}
	s.mu.RUnlock()

	entries, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := s.fileToPath(entry.Name())
		if strings.HasPrefix(path, prefix) {
			paths = append(paths, path)
		}
	}

	return paths, nil
}

// Watch watches for changes to a secret.
func (s *FileStore) Watch(ctx context.Context, path string) (<-chan *secrets.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, secrets.ErrStoreClosed
	}

	ch := make(chan *secrets.Secret, 10)
	s.watchers[path] = append(s.watchers[path], ch)

	// Clean up when context is done
	go func() {
		select {
		case <-ctx.Done():
			s.removeWatcher(path, ch)
		case <-s.closeCh:
			// Store closed
		}
	}()

	return ch, nil
}

// notifyWatchers sends a secret update to all watchers.
func (s *FileStore) notifyWatchers(path string, secret *secrets.Secret) {
	s.mu.RLock()
	watchers := s.watchers[path]
	s.mu.RUnlock()

	for _, ch := range watchers {
		select {
		case ch <- secret:
		default:
			// Channel full, skip
		}
	}
}

// removeWatcher removes a watcher channel from the list.
func (s *FileStore) removeWatcher(path string, ch chan *secrets.Secret) {
	s.mu.Lock()
	defer s.mu.Unlock()

	watchers := s.watchers[path]
	for i, w := range watchers {
		if w == ch {
			s.watchers[path] = append(watchers[:i], watchers[i+1:]...)
			close(ch)
			break
		}
	}
}

// watchFiles polls for file changes.
func (s *FileStore) watchFiles() {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	modTimes := make(map[string]time.Time)

	for {
		select {
		case <-ticker.C:
			s.checkForChanges(modTimes)
		case <-s.closeCh:
			return
		}
	}
}

// checkForChanges checks for file modifications.
func (s *FileStore) checkForChanges(modTimes map[string]time.Time) {
	s.mu.RLock()
	watchedPaths := make([]string, 0, len(s.watchers))
	for path := range s.watchers {
		watchedPaths = append(watchedPaths, path)
	}
	s.mu.RUnlock()

	for _, path := range watchedPaths {
		filePath := s.pathToFile(path)
		info, err := os.Stat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				// File was deleted
				if _, ok := modTimes[path]; ok {
					delete(modTimes, path)
					s.notifyWatchers(path, nil)
				}
			}
			continue
		}

		lastMod := info.ModTime()
		if prevMod, ok := modTimes[path]; !ok || lastMod.After(prevMod) {
			modTimes[path] = lastMod
			if ok { // Only notify if we've seen this file before
				secret, err := s.Get(context.Background(), path)
				if err == nil {
					s.notifyWatchers(path, secret)
				}
			}
		}
	}
}

// Close closes the store and releases resources.
func (s *FileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.closeCh)

	// Close all watcher channels
	for _, watchers := range s.watchers {
		for _, ch := range watchers {
			close(ch)
		}
	}

	s.log.Info().Msg("file store closed")
	return nil
}

// SecretFile represents the structure of an encrypted secret file.
// This is used for SOPS-compatible file format.
type SecretFile struct {
	// Data is the encrypted secret data.
	Data map[string]interface{} `json:"data"`

	// SOPS contains SOPS-compatible encryption metadata.
	SOPS *SOPSMetadata `json:"sops,omitempty"`

	// Metadata contains additional file metadata.
	Metadata *SecretFileMetadata `json:"metadata,omitempty"`
}

// SOPSMetadata contains SOPS-compatible encryption metadata.
type SOPSMetadata struct {
	// Age contains age recipient information.
	Age []AgeRecipient `json:"age,omitempty"`

	// LastModified is when the file was last modified.
	LastModified string `json:"lastmodified,omitempty"`

	// MAC is the message authentication code.
	MAC string `json:"mac,omitempty"`

	// Version is the SOPS version.
	Version string `json:"version,omitempty"`
}

// AgeRecipient contains age encryption recipient information.
type AgeRecipient struct {
	// Recipient is the age public key.
	Recipient string `json:"recipient"`

	// EncryptedKey is the encrypted data encryption key.
	EncryptedKey string `json:"enc"`
}

// SecretFileMetadata contains additional metadata for secret files.
type SecretFileMetadata struct {
	// Path is the secret path.
	Path string `json:"path"`

	// Type is the secret type.
	Type secrets.SecretType `json:"type"`

	// Version is the secret version.
	Version int64 `json:"version"`

	// CreatedAt is when the secret was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the secret was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}
