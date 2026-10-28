// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package wotanerr

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Severity is the 3-bit severity level (draft §Error Severity Levels).
type Severity uint8

// Severity levels. 5-7 are undefined.
const (
	SeverityInfo     Severity = 0
	SeverityWarning  Severity = 1
	SeverityError    Severity = 2
	SeverityCritical Severity = 3
	SeverityFatal    Severity = 4
)

var severityNames = [...]string{"INFO", "WARNING", "ERROR", "CRITICAL", "FATAL"}

// severityActions is the draft's Action column, word for word.
var severityActions = [...]string{
	"Log, continue processing",
	"Log, emit metric, continue",
	"Log, retry or degrade",
	"Log, alert, isolate",
	"Log, halt, require restart",
}

// Defined reports whether s is one of the five levels.
func (s Severity) Defined() bool { return s <= SeverityFatal }

func (s Severity) String() string {
	if !s.Defined() {
		return fmt.Sprintf("SEVERITY_%d", uint8(s))
	}
	return severityNames[s]
}

// Recovery is the draft's recommended action for the level.
func (s Severity) Recovery() string {
	if !s.Defined() {
		return ""
	}
	return severityActions[s]
}

// Origin is the 5-bit subsystem that raised the error (draft origin table).
type Origin uint8

// Origins. 0x0D-0x1E are reserved.
const (
	OriginCore           Origin = 0x00
	OriginL1             Origin = 0x01
	OriginL2             Origin = 0x02
	OriginL3             Origin = 0x03
	OriginGRPC           Origin = 0x04
	OriginTopic          Origin = 0x05
	OriginSettings       Origin = 0x06
	OriginGoaway         Origin = 0x07
	OriginSophiaLookup   Origin = 0x08
	OriginSophiaUpdate   Origin = 0x09
	OriginShieldIngress  Origin = 0x0A
	OriginShieldEgress   Origin = 0x0B
	OriginShimExec       Origin = 0x0C
	OriginVendorSpecific Origin = 0x1F
)

var originNames = map[Origin]string{
	OriginCore:           "WOTAN_CORE",
	OriginL1:             "WOTAN_L1",
	OriginL2:             "WOTAN_L2",
	OriginL3:             "WOTAN_L3",
	OriginGRPC:           "WOTAN_GRPC",
	OriginTopic:          "WOTAN_TOPIC",
	OriginSettings:       "WOTAN_SETTINGS",
	OriginGoaway:         "WOTAN_GOAWAY",
	OriginSophiaLookup:   "SOPHIA_LOOKUP",
	OriginSophiaUpdate:   "SOPHIA_UPDATE",
	OriginShieldIngress:  "SHIELD_INGRESS",
	OriginShieldEgress:   "SHIELD_EGRESS",
	OriginShimExec:       "SHIM_EXEC",
	OriginVendorSpecific: "VENDOR_SPECIFIC",
}

// Origins returns every defined origin in code order.
func Origins() []Origin {
	out := make([]Origin, 0, len(originNames))
	for o := Origin(0); o <= 0x1F; o++ {
		if o.Defined() {
			out = append(out, o)
		}
	}
	return out
}

// Defined reports whether o is a registered origin.
func (o Origin) Defined() bool { _, ok := originNames[o]; return ok }

func (o Origin) String() string {
	if n, ok := originNames[o]; ok {
		return n
	}
	return fmt.Sprintf("ORIGIN_0x%02X", uint8(o))
}

// Category is the 8-bit error category (draft category table).
type Category uint8

// Categories. 0x0C-0xFE are reserved.
const (
	CategoryNone           Category = 0x00
	CategoryAccessControl  Category = 0x01
	CategoryBoundsCheck    Category = 0x02
	CategoryResource       Category = 0x03
	CategoryIntegrity      Category = 0x04
	CategoryProtocol       Category = 0x05
	CategoryTimeout        Category = 0x06
	CategoryConcurrency    Category = 0x07
	CategoryConfiguration  Category = 0x08
	CategoryDependency     Category = 0x09
	CategoryDataCorruption Category = 0x0A
	CategoryRateLimit      Category = 0x0B
	CategoryVendorSpecific Category = 0xFF
)

var categoryNames = map[Category]string{
	CategoryNone:           "NONE",
	CategoryAccessControl:  "ACCESS_CONTROL",
	CategoryBoundsCheck:    "BOUNDS_CHECK",
	CategoryResource:       "RESOURCE",
	CategoryIntegrity:      "INTEGRITY",
	CategoryProtocol:       "PROTOCOL",
	CategoryTimeout:        "TIMEOUT",
	CategoryConcurrency:    "CONCURRENCY",
	CategoryConfiguration:  "CONFIGURATION",
	CategoryDependency:     "DEPENDENCY",
	CategoryDataCorruption: "DATA_CORRUPTION",
	CategoryRateLimit:      "RATE_LIMIT",
	CategoryVendorSpecific: "VENDOR_SPECIFIC",
}

// Categories returns every defined category in code order.
func Categories() []Category {
	out := make([]Category, 0, len(categoryNames))
	for c := 0; c <= 0xFF; c++ {
		if Category(c).Defined() {
			out = append(out, Category(c))
		}
	}
	return out
}

// Defined reports whether c is a registered category.
func (c Category) Defined() bool { _, ok := categoryNames[c]; return ok }

func (c Category) String() string {
	if n, ok := categoryNames[c]; ok {
		return n
	}
	return fmt.Sprintf("CATEGORY_0x%02X", uint8(c))
}

// Code is one structured error code. Detail is interpreted per category.
type Code struct {
	Severity Severity
	Origin   Origin
	Category Category
	Detail   uint8
}

// The codes Wotan's API emits. UnauthorizedPublish is the draft's gRPC row
// "[ERROR, GRPC, ACCESS, 0x01] Unauthorized topic publish"; the others are
// this implementation's assignments under the draft's origin and category
// tables (detail numbering per category is the implementation's, as the
// draft leaves it). The origin is the subsystem that refused: WOTAN_GRPC
// for the gRPC publish path, WOTAN_TOPIC for the HTTP topic API and the
// signed-prefix policy.
var (
	UnauthorizedPublish   = Code{SeverityError, OriginGRPC, CategoryAccessControl, 0x01}
	TopicAccessDenied     = Code{SeverityError, OriginTopic, CategoryAccessControl, 0x01}
	UnknownTopicEndpoint  = Code{SeverityError, OriginTopic, CategoryProtocol, 0x01}
	SignatureRequired     = Code{SeverityError, OriginTopic, CategoryIntegrity, 0x01}
	SignatureInvalid      = Code{SeverityError, OriginTopic, CategoryIntegrity, 0x02}
	SignatureUnverifiable = Code{SeverityError, OriginTopic, CategoryConfiguration, 0x01}
)

// Emitted lists every code Wotan's API can send.
func Emitted() []Code {
	return []Code{UnauthorizedPublish, TopicAccessDenied, UnknownTopicEndpoint, SignatureRequired, SignatureInvalid, SignatureUnverifiable}
}

// Validate reports an undefined severity, origin or category.
func (c Code) Validate() error {
	switch {
	case !c.Severity.Defined():
		return fmt.Errorf("wotanerr: undefined severity %d", uint8(c.Severity))
	case !c.Origin.Defined():
		return fmt.Errorf("wotanerr: undefined origin 0x%02X", uint8(c.Origin))
	case !c.Category.Defined():
		return fmt.Errorf("wotanerr: undefined category 0x%02X", uint8(c.Category))
	}
	return nil
}

// Encode packs the code into its 32-bit wire form. Out-of-range severity
// or origin bits are masked; call Validate first for an untrusted Code.
func (c Code) Encode() uint32 {
	return uint32(c.Severity&0x7)<<29 | uint32(c.Origin&0x1F)<<24 | uint32(c.Category)<<16 | uint32(c.Detail)
}

// Decode unpacks a 32-bit code. Bits 15-8 are not part of the format and
// must be zero; an undefined severity, origin or category is refused.
func Decode(v uint32) (Code, error) {
	if v&0xFF00 != 0 {
		return Code{}, fmt.Errorf("wotanerr: bits 15-8 set in 0x%08X", v)
	}
	c := Code{
		Severity: Severity(v >> 29 & 0x7),
		Origin:   Origin(v >> 24 & 0x1F),
		Category: Category(v >> 16 & 0xFF),
		Detail:   uint8(v & 0xFF),
	}
	if err := c.Validate(); err != nil {
		return Code{}, err
	}
	return c, nil
}

// Hex is the header form: "0x" and eight upper-case hex digits.
func (c Code) Hex() string { return fmt.Sprintf("0x%08X", c.Encode()) }

// ParseHex parses the header form strictly (exactly "0x" + 8 hex digits).
func ParseHex(s string) (Code, error) {
	if len(s) != 10 || !strings.HasPrefix(s, "0x") {
		return Code{}, errors.New("wotanerr: want 0x and 8 hex digits")
	}
	for _, r := range s[2:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return Code{}, errors.New("wotanerr: want 0x and 8 hex digits")
		}
	}
	v, err := strconv.ParseUint(s[2:], 16, 32)
	if err != nil {
		return Code{}, fmt.Errorf("wotanerr: %w", err)
	}
	return Decode(uint32(v))
}

// String is the draft's bracket notation with full names, e.g.
// "[ERROR, WOTAN_GRPC, ACCESS_CONTROL, 0x01]".
func (c Code) String() string {
	return fmt.Sprintf("[%s, %s, %s, 0x%02X]", c.Severity, c.Origin, c.Category, c.Detail)
}

// Error makes a *Code usable as an error (errors.As target *Code).
func (c Code) Error() string { return "wotan error " + c.Hex() + " " + c.String() }

// Recovery is the draft's recommended action for the code's severity.
func (c Code) Recovery() string { return c.Severity.Recovery() }

// The draft's tables write short names in bracket codes
// ("[WARN, L1, BOUNDS, 0x01]"); ParseBracket takes those and the full ones.
var (
	severityShort = map[string]Severity{"WARN": SeverityWarning}
	originShort   = map[string]Origin{"SOPHIA": OriginSophiaLookup}
	categoryShort = map[string]Category{"ACCESS": CategoryAccessControl, "BOUNDS": CategoryBoundsCheck}
)

// ParseBracket parses the draft's notation "[SEVERITY, ORIGIN, CATEGORY,
// 0xDD]". Origins may drop the "WOTAN_" prefix; "SOPHIA" is SOPHIA_LOOKUP
// (the draft's "Sophia Lookup Errors" table).
func ParseBracket(s string) (Code, error) {
	if len(s) > 80 || !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return Code{}, fmt.Errorf("wotanerr: %q is not a bracket code", s)
	}
	parts := strings.Split(s[1:len(s)-1], ",")
	if len(parts) != 4 {
		return Code{}, fmt.Errorf("wotanerr: %q has %d fields, want 4", s, len(parts))
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	var c Code
	var ok bool
	if c.Severity, ok = lookupSeverity(parts[0]); !ok {
		return Code{}, fmt.Errorf("wotanerr: unknown severity %q", parts[0])
	}
	if c.Origin, ok = lookupOrigin(parts[1]); !ok {
		return Code{}, fmt.Errorf("wotanerr: unknown origin %q", parts[1])
	}
	if c.Category, ok = lookupCategory(parts[2]); !ok {
		return Code{}, fmt.Errorf("wotanerr: unknown category %q", parts[2])
	}
	d := parts[3]
	if len(d) != 4 || !strings.HasPrefix(d, "0x") {
		return Code{}, fmt.Errorf("wotanerr: detail %q, want 0x and 2 hex digits", d)
	}
	v, err := strconv.ParseUint(d[2:], 16, 8)
	if err != nil {
		return Code{}, fmt.Errorf("wotanerr: detail %q: %w", d, err)
	}
	c.Detail = uint8(v)
	return c, nil
}

func lookupSeverity(n string) (Severity, bool) {
	if s, ok := severityShort[n]; ok {
		return s, true
	}
	for i, name := range severityNames {
		if name == n {
			return Severity(i), true // #nosec G115 -- i < len(severityNames) == 5
		}
	}
	return 0, false
}

func lookupOrigin(n string) (Origin, bool) {
	if o, ok := originShort[n]; ok {
		return o, true
	}
	for o, name := range originNames {
		if name == n || name == "WOTAN_"+n {
			return o, true
		}
	}
	return 0, false
}

func lookupCategory(n string) (Category, bool) {
	if c, ok := categoryShort[n]; ok {
		return c, true
	}
	for c, name := range categoryNames {
		if name == n {
			return c, true
		}
	}
	return 0, false
}

// HeaderName is the HTTP response header that carries a code.
const HeaderName = "X-Wotan-Error"

// TrailerKey is the gRPC trailer metadata key that carries a code.
const TrailerKey = "x-wotan-error"

// SetHeader puts c on an HTTP response's headers.
func SetHeader(h http.Header, c Code) { h.Set(HeaderName, c.Hex()) }

// FromHeader decodes the X-Wotan-Error header; false when it is absent or
// not a valid code (a malformed value is ignored, never trusted).
func FromHeader(h http.Header) (*Code, bool) {
	return fromValue(h.Get(HeaderName))
}

// FromValues decodes the first value of a gRPC trailer's x-wotan-error.
func FromValues(vals []string) (*Code, bool) {
	if len(vals) == 0 {
		return nil, false
	}
	return fromValue(vals[0])
}

func fromValue(v string) (*Code, bool) {
	if v == "" {
		return nil, false
	}
	c, err := ParseHex(v)
	if err != nil {
		return nil, false
	}
	return &c, true
}

// Wrap returns err with c attached: errors.Is(result, err) holds and
// errors.As(result, &code) with code *Code finds c. Wrap(nil, c) is nil.
func Wrap(err error, c Code) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", err, &c)
}
