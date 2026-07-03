package store

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/infinage/microfix/pkg/message"
)

// Store is the unified state manager for Microfix.
// It abstracts strict configurations, persistent aliases, ephemeral script variables,
// and system environment variables behind a single prefixed key interface.
type Store struct {
	cfg        *Config           // Strongly typed and persistent configuration
	vars       map[string]string // Loosely typed and non-persistent runtime variables
	buffer     message.Message   // Scratch pad to store utmost one message of interest
	configPath string            // Stored path of the config file for auto-saving changes
	mu         sync.RWMutex      // Concurrent access across GUI
}

// NewStoreFromPath initializes a Store by loading the configuration from a specific file path.
func NewStoreFromPath(filepath string) (*Store, error) {
	cfg, err := loadConfig(filepath)
	if err != nil {
		return nil, fmt.Errorf("Failed to load config: %w", err)
	}

	return &Store{
		cfg:        cfg,
		configPath: filepath,
		vars:       make(map[string]string),
	}, nil
}

// InitStore attempts to load the configuration from default locations (CWD or Home).
// If none are found, it creates a default configuration in the CWD.
func InitStore() Store {
	cfg, configPath := initConfig()

	return Store{
		cfg:        &cfg,
		configPath: configPath,
		vars:       make(map[string]string),
	}
}

func (s *Store) Buffer() message.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buffer
}

func (s *Store) SetBuffer(msg message.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer = msg
}

func (s *Store) getTagFromBuffer(key string) (string, bool, error) {
	tag, err := strconv.ParseUint(key, 10, 16)
	if err != nil {
		return "", false, fmt.Errorf("failed to parse '%s' as a tag: %w", key, err)
	}

	val, ok := s.buffer.Get(uint16(tag))
	return val, ok, nil
}

// Config returns a safe, by-value copy of the underlying typed configuration.
func (s *Store) Config() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.cfg
}

// LoadConfig dynamically overwrites the current store's config by loading from the specified path.
func (s *Store) LoadConfig(filepath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	newCfg, err := loadConfig(filepath)
	if err == nil {
		s.cfg = newCfg
		s.configPath = filepath
	}
	return err
}

// Writes the current configuration state to the specified path.
func (s *Store) DumpConfig(filepath string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.dump(filepath)
}

// Read only copy of path config was loaded from
func (s *Store) ConfigPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configPath
}

// Get retrieves a value based on its namespace prefix.
// The key must be in the format `PREFIX.Name` (e.g., "CFG.Port", "ENV.USER").
// It returns the value, a boolean indicating if the key was found, and any potential errors.
func (s *Store) Get(key string) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix, name, err := splitKeyPrefix(key)
	if err != nil {
		return "", false, err
	}

	switch strings.ToUpper(prefix) {
	case "CFG":
		oldVal, err := s.cfg.getField(name)
		return oldVal, err == nil, err

	case "ALIAS":
		val, ok := s.cfg.getAlias(name)
		return val, ok, nil

	case "VARS":
		val, ok := s.vars[name]
		return val, ok, nil

	case "ENV":
		val, ok := os.LookupEnv(name)
		return val, ok, nil

	case "BUF":
		return s.getTagFromBuffer(name)

	default:
		return "", false, fmt.Errorf("Unsupported prefix: '%s'", prefix)
	}
}

// Set updates a value in the store and returns the previous value, a boolean
// indicating if it was an update to an existing key, and an error.
func (s *Store) Set(key, value string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix, name, err := splitKeyPrefix(key)
	if err != nil {
		return "", false, err
	}

	switch strings.ToUpper(prefix) {
	case "CFG":
		oldVal, err := s.cfg.setField(name, value)
		if err != nil {
			return "", false, err
		}
		return oldVal, true, nil

	case "ALIAS":
		val, ok := s.cfg.setAlias(name, value)
		return val, ok, nil

	case "VARS":
		oldVal, ok := s.vars[name]
		s.vars[name] = value
		return oldVal, ok, nil

	case "ENV":
		return "", false, fmt.Errorf("Cannot modify system env variables")

	case "BUF":
		return "", false, fmt.Errorf("Cannot modify message buffer")

	default:
		return "", false, fmt.Errorf("Unsupported prefix: '%s'", prefix)
	}
}

// Unset removes a key from loosely typed namespaces (ALIAS, VARS).
// It returns the deleted value, a boolean indicating if it existed before deletion, and an error.
func (s *Store) Unset(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix, name, err := splitKeyPrefix(key)
	if err != nil {
		return "", false, err
	}

	switch strings.ToUpper(prefix) {
	case "CFG", "ENV":
		return "", false, fmt.Errorf("Can only delete from ALIAS and VARS namespaces")

	case "ALIAS":
		val, ok := s.cfg.deleteAlias(name)
		return val, ok, nil

	case "VARS":
		oldVal, ok := s.vars[name]
		if ok {
			delete(s.vars, name)
		}
		return oldVal, ok, nil

	default:
		return "", false, fmt.Errorf("unsupported prefix: '%s'", prefix)
	}
}

// splitKeyPrefix splits a key formatted as `PREFIX.Name` into its two components.
func splitKeyPrefix(key string) (string, string, error) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("Invalid key format, must be PREFIX.Name (e.g. CFG.Port)")
	}
	return parts[0], parts[1], nil
}
