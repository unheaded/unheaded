package watch

import (
	"sync"
	"time"
)

// Notifier manages configuration change notifications.
type Notifier struct {
	mu        sync.RWMutex
	callbacks []CallbackEntry
	debounce  time.Duration
	lastEvent time.Time
	pending   bool
	timer     *time.Timer
}

// CallbackEntry represents a registered callback.
type CallbackEntry struct {
	ID       string
	Callback func(Event)
	Filter   func(Event) bool
	Once     bool
	called   bool
}

// NewNotifier creates a new notifier.
func NewNotifier() *Notifier {
	return &Notifier{
		callbacks: make([]CallbackEntry, 0),
		debounce:  100 * time.Millisecond,
	}
}

// NewNotifierWithDebounce creates a new notifier with custom debounce duration.
func NewNotifierWithDebounce(debounce time.Duration) *Notifier {
	return &Notifier{
		callbacks: make([]CallbackEntry, 0),
		debounce:  debounce,
	}
}

// OnChange registers a callback for change events.
func (n *Notifier) OnChange(id string, callback func(Event)) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.callbacks = append(n.callbacks, CallbackEntry{
		ID:       id,
		Callback: callback,
	})
}

// OnChangeFiltered registers a filtered callback for change events.
func (n *Notifier) OnChangeFiltered(id string, callback func(Event), filter func(Event) bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.callbacks = append(n.callbacks, CallbackEntry{
		ID:       id,
		Callback: callback,
		Filter:   filter,
	})
}

// OnChangeOnce registers a one-time callback.
func (n *Notifier) OnChangeOnce(id string, callback func(Event)) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.callbacks = append(n.callbacks, CallbackEntry{
		ID:       id,
		Callback: callback,
		Once:     true,
	})
}

// RemoveCallback removes a callback by ID.
func (n *Notifier) RemoveCallback(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	newCallbacks := make([]CallbackEntry, 0, len(n.callbacks))
	for _, cb := range n.callbacks {
		if cb.ID != id {
			newCallbacks = append(newCallbacks, cb)
		}
	}
	n.callbacks = newCallbacks
}

// Notify sends an event to all registered callbacks.
func (n *Notifier) Notify(event Event) {
	n.mu.Lock()

	// Debounce logic
	if n.debounce > 0 {
		n.lastEvent = time.Now()
		if n.pending {
			n.mu.Unlock()
			return
		}
		n.pending = true

		if n.timer != nil {
			n.timer.Stop()
		}

		n.timer = time.AfterFunc(n.debounce, func() {
			n.mu.Lock()
			n.pending = false
			callbacks := n.getActiveCallbacks()
			n.mu.Unlock()

			n.invokeCallbacks(callbacks, event)
		})

		n.mu.Unlock()
		return
	}

	callbacks := n.getActiveCallbacks()
	n.mu.Unlock()

	n.invokeCallbacks(callbacks, event)
}

// getActiveCallbacks returns callbacks that should be invoked.
func (n *Notifier) getActiveCallbacks() []CallbackEntry {
	result := make([]CallbackEntry, 0, len(n.callbacks))
	for i := range n.callbacks {
		if n.callbacks[i].Once && n.callbacks[i].called {
			continue
		}
		result = append(result, n.callbacks[i])
	}
	return result
}

// invokeCallbacks invokes all matching callbacks.
func (n *Notifier) invokeCallbacks(callbacks []CallbackEntry, event Event) {
	for i := range callbacks {
		cb := &callbacks[i]

		// Apply filter
		if cb.Filter != nil && !cb.Filter(event) {
			continue
		}

		// Invoke callback
		cb.Callback(event)

		// Mark once callbacks as called
		if cb.Once {
			n.mu.Lock()
			for j := range n.callbacks {
				if n.callbacks[j].ID == cb.ID {
					n.callbacks[j].called = true
					break
				}
			}
			n.mu.Unlock()
		}
	}
}

// Clear removes all callbacks.
func (n *Notifier) Clear() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.callbacks = make([]CallbackEntry, 0)
	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}
	n.pending = false
}

// SetDebounce sets the debounce duration.
func (n *Notifier) SetDebounce(d time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.debounce = d
}

// Count returns the number of registered callbacks.
func (n *Notifier) Count() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.callbacks)
}

// ChangeTracker tracks configuration changes over time.
type ChangeTracker struct {
	mu      sync.RWMutex
	changes []ChangeRecord
	maxSize int
}

// ChangeRecord represents a recorded change.
type ChangeRecord struct {
	Event     Event
	Timestamp time.Time
	Data      map[string]any
}

// NewChangeTracker creates a new change tracker.
func NewChangeTracker(maxSize int) *ChangeTracker {
	return &ChangeTracker{
		changes: make([]ChangeRecord, 0, maxSize),
		maxSize: maxSize,
	}
}

// Record records a change event.
func (ct *ChangeTracker) Record(event Event, data map[string]any) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	record := ChangeRecord{
		Event:     event,
		Timestamp: time.Now(),
		Data:      data,
	}

	ct.changes = append(ct.changes, record)

	// Trim if exceeds max size
	if len(ct.changes) > ct.maxSize {
		ct.changes = ct.changes[len(ct.changes)-ct.maxSize:]
	}
}

// History returns the change history.
func (ct *ChangeTracker) History() []ChangeRecord {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	result := make([]ChangeRecord, len(ct.changes))
	copy(result, ct.changes)
	return result
}

// Since returns changes since the given time.
func (ct *ChangeTracker) Since(t time.Time) []ChangeRecord {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var result []ChangeRecord
	for _, change := range ct.changes {
		if change.Timestamp.After(t) {
			result = append(result, change)
		}
	}
	return result
}

// Last returns the last n changes.
func (ct *ChangeTracker) Last(n int) []ChangeRecord {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	if n > len(ct.changes) {
		n = len(ct.changes)
	}

	result := make([]ChangeRecord, n)
	copy(result, ct.changes[len(ct.changes)-n:])
	return result
}

// Clear clears all recorded changes.
func (ct *ChangeTracker) Clear() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.changes = make([]ChangeRecord, 0, ct.maxSize)
}

// Count returns the number of recorded changes.
func (ct *ChangeTracker) Count() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return len(ct.changes)
}

// ConfigReloader handles configuration reloading with notifications.
type ConfigReloader struct {
	notifier   *Notifier
	tracker    *ChangeTracker
	loader     func() (map[string]any, error)
	current    map[string]any
	mu         sync.RWMutex
	reloadLock sync.Mutex
}

// NewConfigReloader creates a new config reloader.
func NewConfigReloader(loader func() (map[string]any, error)) *ConfigReloader {
	return &ConfigReloader{
		notifier: NewNotifier(),
		tracker:  NewChangeTracker(100),
		loader:   loader,
		current:  make(map[string]any),
	}
}

// Reload reloads the configuration.
func (cr *ConfigReloader) Reload() error {
	cr.reloadLock.Lock()
	defer cr.reloadLock.Unlock()

	newConfig, err := cr.loader()
	if err != nil {
		return err
	}

	cr.mu.Lock()
	oldConfig := cr.current
	cr.current = newConfig
	cr.mu.Unlock()

	// Track changes
	event := Event{
		Op:        Write,
		Timestamp: time.Now(),
	}
	cr.tracker.Record(event, newConfig)

	// Notify
	cr.notifier.Notify(event)

	_ = oldConfig // Could compare for detailed change tracking
	return nil
}

// OnChange registers a change callback.
func (cr *ConfigReloader) OnChange(id string, callback func(Event)) {
	cr.notifier.OnChange(id, callback)
}

// Current returns the current configuration.
func (cr *ConfigReloader) Current() map[string]any {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	result := make(map[string]any)
	for k, v := range cr.current {
		result[k] = v
	}
	return result
}

// History returns the reload history.
func (cr *ConfigReloader) History() []ChangeRecord {
	return cr.tracker.History()
}

// DiffDetector detects differences between configurations.
type DiffDetector struct {
	keyFilters []string
}

// NewDiffDetector creates a new diff detector.
func NewDiffDetector() *DiffDetector {
	return &DiffDetector{}
}

// SetKeyFilters sets keys to include in diff detection.
func (dd *DiffDetector) SetKeyFilters(keys []string) {
	dd.keyFilters = keys
}

// Diff returns the differences between two configurations.
func (dd *DiffDetector) Diff(old, new map[string]any) []ConfigDiff {
	var diffs []ConfigDiff

	// Check for added and modified keys
	dd.diffMaps("", old, new, &diffs)

	return diffs
}

// diffMaps recursively diffs two maps.
func (dd *DiffDetector) diffMaps(prefix string, old, new map[string]any, diffs *[]ConfigDiff) {
	for key, newVal := range new {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		// Check key filter
		if len(dd.keyFilters) > 0 && !dd.matchesFilter(fullKey) {
			continue
		}

		oldVal, exists := old[key]
		if !exists {
			*diffs = append(*diffs, ConfigDiff{
				Key:      fullKey,
				Type:     DiffAdd,
				OldValue: nil,
				NewValue: newVal,
			})
			continue
		}

		// Recursive diff for nested maps
		oldMap, oldIsMap := oldVal.(map[string]any)
		newMap, newIsMap := newVal.(map[string]any)

		if oldIsMap && newIsMap {
			dd.diffMaps(fullKey, oldMap, newMap, diffs)
			continue
		}

		// Compare values
		if !equalValues(oldVal, newVal) {
			*diffs = append(*diffs, ConfigDiff{
				Key:      fullKey,
				Type:     DiffModify,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}

	// Check for removed keys
	for key, oldVal := range old {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		if len(dd.keyFilters) > 0 && !dd.matchesFilter(fullKey) {
			continue
		}

		if _, exists := new[key]; !exists {
			*diffs = append(*diffs, ConfigDiff{
				Key:      fullKey,
				Type:     DiffRemove,
				OldValue: oldVal,
				NewValue: nil,
			})
		}
	}
}

// matchesFilter checks if a key matches any filter.
func (dd *DiffDetector) matchesFilter(key string) bool {
	for _, filter := range dd.keyFilters {
		if key == filter || len(key) > len(filter) && key[:len(filter)+1] == filter+"." {
			return true
		}
	}
	return false
}

// equalValues compares two values for equality.
func equalValues(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return false
		}
		if len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !equalValues(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return false
		}
		if len(av) != len(bv) {
			return false
		}
		for i, v := range av {
			if !equalValues(v, bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// ConfigDiff represents a configuration difference.
type ConfigDiff struct {
	Key      string
	Type     DiffType
	OldValue any
	NewValue any
}

// DiffType represents the type of configuration difference.
type DiffType int

const (
	// DiffAdd represents an added key.
	DiffAdd DiffType = iota
	// DiffModify represents a modified key.
	DiffModify
	// DiffRemove represents a removed key.
	DiffRemove
)

// String returns a string representation of the diff type.
func (dt DiffType) String() string {
	switch dt {
	case DiffAdd:
		return "ADD"
	case DiffModify:
		return "MODIFY"
	case DiffRemove:
		return "REMOVE"
	default:
		return "UNKNOWN"
	}
}
