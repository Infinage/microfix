package store

import (
	"fmt"
	"os"
	"strings"
)

// Store is the unified state manager for Microfix.
// It abstracts strict configurations, persistent aliases, ephemeral script variables,
// and system environment variables behind a single prefixed key interface.
type Store struct {
	cfg        *Config           // Strongly typed and persistent configuration
	vars       map[string]string // Loosely typed and non-persistent runtime variables
	configPath string            // Stored path of the config file for auto-saving changes
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

// Config returns a safe, by-value copy of the underlying typed configuration.
func (c *Store) Config() Config {
	return *c.cfg
}

// LoadConfig dynamically overwrites the current store's config by loading from the specified path.
func (c *Store) LoadConfig(filepath string) error {
	newCfg, err := loadConfig(filepath)
	if err == nil {
		c.cfg = newCfg
		c.configPath = filepath
	}
	return err
}

// Writes the current configuration state to the specified path.
func (c *Store) DumpConfig(filepath string) error {
	return c.cfg.dump(filepath)
}

// Read only copy of path config was loaded from
func (c *Store) ConfigPath() string {
	return c.configPath
}

// Get retrieves a value based on its namespace prefix.
// The key must be in the format `PREFIX.Name` (e.g., "CFG.Port", "ENV.USER").
// It returns the value, a boolean indicating if the key was found, and any potential errors.
func (c *Store) Get(key string) (string, bool, error) {
	prefix, name, err := splitKeyPrefix(key)
	if err != nil {
		return "", false, err
	}

	switch strings.ToUpper(prefix) {
	case "CFG":
		oldVal, err := c.cfg.getField(name)
		return oldVal, err == nil, err

	case "ALIAS":
		val, ok := c.cfg.getAlias(name)
		return val, ok, nil

	case "VARS":
		val, ok := c.vars[name]
		return val, ok, nil

	case "ENV":
		val, ok := os.LookupEnv(name)
		return val, ok, nil

	default:
		return "", false, fmt.Errorf("Unsupported prefix: '%s'", prefix)
	}
}

// Set updates a value in the store and returns the previous value, a boolean
// indicating if it was an update to an existing key, and an error.
func (c *Store) Set(key, value string) (string, bool, error) {
	prefix, name, err := splitKeyPrefix(key)
	if err != nil {
		return "", false, err
	}

	switch strings.ToUpper(prefix) {
	case "CFG":
		oldVal, err := c.cfg.setField(name, value)
		if err != nil {
			return "", false, err
		}
		return oldVal, true, nil

	case "ALIAS":
		val, ok := c.cfg.setAlias(name, value)
		return val, ok, nil

	case "VARS":
		oldVal, ok := c.vars[name]
		c.vars[name] = value
		return oldVal, ok, nil

	case "ENV":
		return "", false, fmt.Errorf("Cannot modify system env variables")

	default:
		return "", false, fmt.Errorf("Unsupported prefix: '%s'", prefix)
	}
}

// Unset removes a key from loosely typed namespaces (ALIAS, VARS).
// It returns the deleted value, a boolean indicating if it existed before deletion, and an error.
func (c *Store) Unset(key string) (string, bool, error) {
	prefix, name, err := splitKeyPrefix(key)
	if err != nil {
		return "", false, err
	}

	switch strings.ToUpper(prefix) {
	case "CFG", "ENV":
		return "", false, fmt.Errorf("Can only delete from ALIAS and VARS namespaces")

	case "ALIAS":
		val, ok := c.cfg.deleteAlias(name)
		return val, ok, nil

	case "VARS":
		oldVal, ok := c.vars[name]
		if ok {
			delete(c.vars, name)
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
