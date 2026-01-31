// Package config provides a comprehensive configuration management system
// for the Unheaded Kingdom. It supports multiple sources (files, environment
// variables, command-line flags, remote endpoints), hot reload, validation,
// and template expansion.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/config/merge"
	"unheaded/pkg/config/sources"
	"unheaded/pkg/config/template"
	"unheaded/pkg/config/watch"
)

// Config represents a loaded configuration with support for multiple sources,
// hot reload, and change notifications.
type Config struct {
	mu         sync.RWMutex
	data       map[string]any
	sources    []Source
	defaults   map[string]any
	watcher    *watch.Watcher
	callbacks  []func(*Config)
	validators []merge.Validator
	envPrefix  string
	filePath   string
}

// Source defines the interface for configuration sources.
type Source interface {
	Load() (map[string]any, error)
	Name() string
	Priority() int
}

// Option is a functional option for configuring the Config loader.
type Option func(*Config) error

// Load creates a new Config with the provided options.
func Load(opts ...Option) (*Config, error) {
	cfg := &Config{
		data:       make(map[string]any),
		defaults:   make(map[string]any),
		callbacks:  make([]func(*Config), 0),
		validators: make([]merge.Validator, 0),
	}

	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, fmt.Errorf("config option error: %w", err)
		}
	}

	if err := cfg.reload(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// WithFile adds a file source to the configuration.
func WithFile(path string) Option {
	return func(c *Config) error {
		c.filePath = path
		src, err := sources.NewFileSource(path)
		if err != nil {
			return fmt.Errorf("file source %s: %w", path, err)
		}
		c.sources = append(c.sources, src)
		return nil
	}
}

// WithEnvPrefix adds environment variable source with the given prefix.
func WithEnvPrefix(prefix string) Option {
	return func(c *Config) error {
		c.envPrefix = prefix
		src := sources.NewEnvSource(prefix)
		c.sources = append(c.sources, src)
		return nil
	}
}

// WithFlags adds command-line flags as a configuration source.
func WithFlags(args []string) Option {
	return func(c *Config) error {
		src := sources.NewFlagSource(args)
		c.sources = append(c.sources, src)
		return nil
	}
}

// WithDefaults sets default values for configuration keys.
func WithDefaults(defaults map[string]any) Option {
	return func(c *Config) error {
		c.defaults = defaults
		return nil
	}
}

// WithRemote adds a remote HTTP endpoint as a configuration source.
func WithRemote(url string, interval time.Duration) Option {
	return func(c *Config) error {
		src := sources.NewRemoteSource(url, interval)
		c.sources = append(c.sources, src)
		return nil
	}
}

// WithValidator adds a custom validator for the configuration.
func WithValidator(v merge.Validator) Option {
	return func(c *Config) error {
		c.validators = append(c.validators, v)
		return nil
	}
}

// WithHotReload enables hot reload for file-based configuration.
func WithHotReload() Option {
	return func(c *Config) error {
		if c.filePath == "" {
			return nil // No file to watch
		}
		w, err := watch.NewWatcher(c.filePath)
		if err != nil {
			return fmt.Errorf("watcher creation: %w", err)
		}
		c.watcher = w
		go c.watchLoop()
		return nil
	}
}

// watchLoop listens for file changes and triggers reload.
func (c *Config) watchLoop() {
	if c.watcher == nil {
		return
	}
	for range c.watcher.Events() {
		if err := c.reload(); err != nil {
			continue // Log error in production
		}
		c.notifyChange()
	}
}

// reload reloads configuration from all sources.
func (c *Config) reload() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Start with defaults
	merged := merge.DeepCopy(c.defaults)

	// Sort sources by priority and merge
	sortedSources := make([]Source, len(c.sources))
	copy(sortedSources, c.sources)
	sortSourcesByPriority(sortedSources)

	for _, src := range sortedSources {
		data, err := src.Load()
		if err != nil {
			return fmt.Errorf("source %s: %w", src.Name(), err)
		}
		merged = merge.Merge(merged, data)
	}

	// Expand templates
	expanded, err := template.Expand(merged)
	if err != nil {
		return fmt.Errorf("template expansion: %w", err)
	}

	// Validate
	for _, v := range c.validators {
		if err := v.Validate(expanded); err != nil {
			return fmt.Errorf("validation: %w", err)
		}
	}

	c.data = expanded
	return nil
}

// sortSourcesByPriority sorts sources so that higher priority (lower number) sources
// are processed last and override lower priority sources.
func sortSourcesByPriority(sources []Source) {
	// Sort by descending priority number (100, 50, 10)
	// This means files (100) are processed first, then env (50), then flags (10) last
	// Since merge gives precedence to later values, flags override env, which overrides files
	for i := 0; i < len(sources)-1; i++ {
		for j := i + 1; j < len(sources); j++ {
			if sources[i].Priority() < sources[j].Priority() {
				sources[i], sources[j] = sources[j], sources[i]
			}
		}
	}
}

// OnChange registers a callback for configuration changes.
func (c *Config) OnChange(fn func(*Config)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks = append(c.callbacks, fn)
}

// notifyChange notifies all registered callbacks of a configuration change.
func (c *Config) notifyChange() {
	c.mu.RLock()
	callbacks := make([]func(*Config), len(c.callbacks))
	copy(callbacks, c.callbacks)
	c.mu.RUnlock()

	for _, fn := range callbacks {
		fn(c)
	}
}

// Get retrieves a value by key using dot notation (e.g., "server.port").
func (c *Config) Get(key string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return getNestedValue(c.data, key)
}

// GetString retrieves a string value by key.
func (c *Config) GetString(key string) string {
	val := c.Get(key)
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetInt retrieves an integer value by key.
func (c *Config) GetInt(key string) int {
	val := c.Get(key)
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

// GetInt64 retrieves an int64 value by key.
func (c *Config) GetInt64(key string) int64 {
	val := c.Get(key)
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		i, _ := strconv.ParseInt(v, 10, 64)
		return i
	default:
		return 0
	}
}

// GetFloat64 retrieves a float64 value by key.
func (c *Config) GetFloat64(key string) float64 {
	val := c.Get(key)
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}

// GetBool retrieves a boolean value by key.
func (c *Config) GetBool(key string) bool {
	val := c.Get(key)
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	case int:
		return v != 0
	default:
		return false
	}
}

// GetDuration retrieves a duration value by key.
func (c *Config) GetDuration(key string) time.Duration {
	val := c.Get(key)
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case time.Duration:
		return v
	case string:
		d, _ := time.ParseDuration(v)
		return d
	case int64:
		return time.Duration(v)
	case int:
		return time.Duration(v)
	default:
		return 0
	}
}

// GetStringSlice retrieves a string slice value by key.
func (c *Config) GetStringSlice(key string) []string {
	val := c.Get(key)
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			result[i] = fmt.Sprintf("%v", item)
		}
		return result
	default:
		return nil
	}
}

// GetStringMap retrieves a map[string]any value by key.
func (c *Config) GetStringMap(key string) map[string]any {
	val := c.Get(key)
	if val == nil {
		return nil
	}
	if m, ok := val.(map[string]any); ok {
		return m
	}
	return nil
}

// Unmarshal unmarshals the configuration into a struct using reflection.
func (c *Config) Unmarshal(v any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return unmarshalValue(c.data, v)
}

// UnmarshalKey unmarshals a specific key into a struct.
func (c *Config) UnmarshalKey(key string, v any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val := getNestedValue(c.data, key)
	if val == nil {
		return fmt.Errorf("key %s not found", key)
	}
	data, ok := val.(map[string]any)
	if !ok {
		return fmt.Errorf("key %s is not a map", key)
	}
	return unmarshalValue(data, v)
}

// Set sets a value at the given key path.
func (c *Config) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	setNestedValue(c.data, key, value)
}

// Has checks if a key exists in the configuration.
func (c *Config) Has(key string) bool {
	return c.Get(key) != nil
}

// AllKeys returns all configuration keys in dot notation.
func (c *Config) AllKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return collectKeys(c.data, "")
}

// AllSettings returns a copy of all configuration data.
func (c *Config) AllSettings() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return merge.DeepCopy(c.data)
}

// Close stops the file watcher and cleans up resources.
func (c *Config) Close() error {
	if c.watcher != nil {
		return c.watcher.Close()
	}
	return nil
}

// getNestedValue retrieves a value from a nested map using dot notation.
func getNestedValue(data map[string]any, key string) any {
	parts := strings.Split(key, ".")
	current := any(data)

	for _, part := range parts {
		if current == nil {
			return nil
		}
		switch v := current.(type) {
		case map[string]any:
			current = v[part]
		default:
			return nil
		}
	}
	return current
}

// setNestedValue sets a value in a nested map using dot notation.
func setNestedValue(data map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	current := data

	for i, part := range parts[:len(parts)-1] {
		if _, ok := current[part]; !ok {
			current[part] = make(map[string]any)
		}
		if next, ok := current[part].(map[string]any); ok {
			current = next
		} else {
			// Create intermediate maps
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
			for j := i + 1; j < len(parts)-1; j++ {
				newMap := make(map[string]any)
				current[parts[j]] = newMap
				current = newMap
			}
			break
		}
	}
	current[parts[len(parts)-1]] = value
}

// collectKeys collects all keys from a nested map.
func collectKeys(data map[string]any, prefix string) []string {
	var keys []string
	for k, v := range data {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		keys = append(keys, fullKey)
		if nested, ok := v.(map[string]any); ok {
			keys = append(keys, collectKeys(nested, fullKey)...)
		}
	}
	return keys
}

// unmarshalValue unmarshals a map into a struct using reflection.
func unmarshalValue(data map[string]any, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("unmarshal target must be a non-nil pointer")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("unmarshal target must be a pointer to struct")
	}

	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		if !field.CanSet() {
			continue
		}

		fieldType := rt.Field(i)
		tag := fieldType.Tag.Get("config")
		if tag == "" {
			tag = strings.ToLower(fieldType.Name)
		}
		if tag == "-" {
			continue
		}

		val, ok := data[tag]
		if !ok {
			continue
		}

		if err := setFieldValue(field, val); err != nil {
			return fmt.Errorf("field %s: %w", fieldType.Name, err)
		}
	}
	return nil
}

// setFieldValue sets a reflect.Value from an interface value.
func setFieldValue(field reflect.Value, val any) error {
	if val == nil {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(fmt.Sprintf("%v", val))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var intVal int64
		switch v := val.(type) {
		case int:
			intVal = int64(v)
		case int64:
			intVal = v
		case float64:
			intVal = int64(v)
		case string:
			var err error
			intVal, err = strconv.ParseInt(v, 10, 64)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("cannot convert %T to int", val)
		}
		field.SetInt(intVal)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var uintVal uint64
		switch v := val.(type) {
		case int:
			uintVal = uint64(v)
		case int64:
			uintVal = uint64(v)
		case float64:
			uintVal = uint64(v)
		case string:
			var err error
			uintVal, err = strconv.ParseUint(v, 10, 64)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("cannot convert %T to uint", val)
		}
		field.SetUint(uintVal)
	case reflect.Float32, reflect.Float64:
		var floatVal float64
		switch v := val.(type) {
		case float64:
			floatVal = v
		case int:
			floatVal = float64(v)
		case int64:
			floatVal = float64(v)
		case string:
			var err error
			floatVal, err = strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("cannot convert %T to float", val)
		}
		field.SetFloat(floatVal)
	case reflect.Bool:
		var boolVal bool
		switch v := val.(type) {
		case bool:
			boolVal = v
		case string:
			var err error
			boolVal, err = strconv.ParseBool(v)
			if err != nil {
				return err
			}
		case int:
			boolVal = v != 0
		default:
			return fmt.Errorf("cannot convert %T to bool", val)
		}
		field.SetBool(boolVal)
	case reflect.Slice:
		return setSliceValue(field, val)
	case reflect.Map:
		return setMapValue(field, val)
	case reflect.Struct:
		if m, ok := val.(map[string]any); ok {
			return unmarshalValue(m, field.Addr().Interface())
		}
		return fmt.Errorf("cannot unmarshal %T into struct", val)
	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return setFieldValue(field.Elem(), val)
	default:
		return fmt.Errorf("unsupported field kind: %s", field.Kind())
	}
	return nil
}

// setSliceValue sets a slice field from an interface value.
func setSliceValue(field reflect.Value, val any) error {
	slice, ok := val.([]any)
	if !ok {
		return fmt.Errorf("expected slice, got %T", val)
	}

	elemType := field.Type().Elem()
	newSlice := reflect.MakeSlice(field.Type(), len(slice), len(slice))

	for i, item := range slice {
		elem := reflect.New(elemType).Elem()
		if err := setFieldValue(elem, item); err != nil {
			return fmt.Errorf("slice element %d: %w", i, err)
		}
		newSlice.Index(i).Set(elem)
	}

	field.Set(newSlice)
	return nil
}

// setMapValue sets a map field from an interface value.
func setMapValue(field reflect.Value, val any) error {
	m, ok := val.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map, got %T", val)
	}

	keyType := field.Type().Key()
	elemType := field.Type().Elem()

	if keyType.Kind() != reflect.String {
		return fmt.Errorf("only string keys are supported")
	}

	newMap := reflect.MakeMap(field.Type())

	for k, v := range m {
		keyVal := reflect.ValueOf(k)
		elemVal := reflect.New(elemType).Elem()
		if err := setFieldValue(elemVal, v); err != nil {
			return fmt.Errorf("map key %s: %w", k, err)
		}
		newMap.SetMapIndex(keyVal, elemVal)
	}

	field.Set(newMap)
	return nil
}

// ToJSON converts the configuration to JSON.
func (c *Config) ToJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return json.MarshalIndent(c.data, "", "  ")
}

// FromJSON loads configuration from JSON bytes.
func FromJSON(data []byte) (*Config, error) {
	cfg := &Config{
		data:       make(map[string]any),
		defaults:   make(map[string]any),
		callbacks:  make([]func(*Config), 0),
		validators: make([]merge.Validator, 0),
	}

	if err := json.Unmarshal(data, &cfg.data); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return cfg, nil
}

// MustLoad loads configuration and panics on error.
func MustLoad(opts ...Option) *Config {
	cfg, err := Load(opts...)
	if err != nil {
		panic(fmt.Sprintf("config load failed: %v", err))
	}
	return cfg
}

// Reload forces a configuration reload from all sources.
func (c *Config) Reload() error {
	if err := c.reload(); err != nil {
		return err
	}
	c.notifyChange()
	return nil
}

// Sub returns a sub-configuration rooted at the given key.
func (c *Config) Sub(key string) *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()

	val := getNestedValue(c.data, key)
	if val == nil {
		return &Config{data: make(map[string]any)}
	}

	if m, ok := val.(map[string]any); ok {
		return &Config{
			data:       merge.DeepCopy(m),
			defaults:   make(map[string]any),
			callbacks:  make([]func(*Config), 0),
			validators: make([]merge.Validator, 0),
		}
	}

	return &Config{data: make(map[string]any)}
}

// Merge merges another config into this one.
func (c *Config) Merge(other *Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	c.data = merge.Merge(c.data, other.data)
}

// WriteToFile writes the configuration to a file in the specified format.
func (c *Config) WriteToFile(path string) error {
	c.mu.RLock()
	data := merge.DeepCopy(c.data)
	c.mu.RUnlock()

	var content []byte
	var err error

	if strings.HasSuffix(path, ".json") {
		content, err = json.MarshalIndent(data, "", "  ")
	} else if strings.HasSuffix(path, ".toml") {
		content = []byte(mapToTOML(data, ""))
	} else if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		content = []byte(mapToYAML(data, 0))
	} else {
		return fmt.Errorf("unsupported file format: %s", path)
	}

	if err != nil {
		return err
	}

	return os.WriteFile(path, content, 0644)
}

// mapToTOML converts a map to TOML format.
func mapToTOML(data map[string]any, section string) string {
	var sb strings.Builder

	// Write simple values first
	for k, v := range data {
		if _, ok := v.(map[string]any); ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s = %s\n", k, toTOMLValue(v)))
	}

	// Write sections
	for k, v := range data {
		if m, ok := v.(map[string]any); ok {
			sectionName := k
			if section != "" {
				sectionName = section + "." + k
			}
			sb.WriteString(fmt.Sprintf("\n[%s]\n", sectionName))
			sb.WriteString(mapToTOML(m, sectionName))
		}
	}

	return sb.String()
}

// toTOMLValue converts a value to TOML string representation.
func toTOMLValue(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case int, int64, float64:
		return fmt.Sprintf("%v", val)
	case []any:
		items := make([]string, len(val))
		for i, item := range val {
			items[i] = toTOMLValue(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", v))
	}
}

// mapToYAML converts a map to YAML format.
func mapToYAML(data map[string]any, indent int) string {
	var sb strings.Builder
	prefix := strings.Repeat("  ", indent)

	for k, v := range data {
		switch val := v.(type) {
		case map[string]any:
			sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			sb.WriteString(mapToYAML(val, indent+1))
		case []any:
			sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			for _, item := range val {
				if m, ok := item.(map[string]any); ok {
					sb.WriteString(fmt.Sprintf("%s  -\n", prefix))
					sb.WriteString(mapToYAML(m, indent+2))
				} else {
					sb.WriteString(fmt.Sprintf("%s  - %v\n", prefix, item))
				}
			}
		default:
			sb.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, k, v))
		}
	}

	return sb.String()
}
