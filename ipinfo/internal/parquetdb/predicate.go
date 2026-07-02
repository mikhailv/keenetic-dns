package parquetdb

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/parquet-go/parquet-go"
)

// Predicate is a single condition. Predicates passed to DB.Query are combined with logical AND.
//
// Construct predicates with the typed constructors: EqInt64, LEInt64, GEInt64, EqString, Wildcard, RangeContainsInt64.
type Predicate struct {
	op        op
	field     string         // primary column
	field2    string         // secondary column (only used by range predicates)
	intVal    int64          // payload for int predicates
	strVal    string         // payload for string predicates
	strValInt bool           // indicates that a string is a valid integer
	pattern   *regexp.Regexp // compiled glob (opWildcard only)
}

type op uint8

const (
	opEqInt64 op = iota + 1
	opLEInt64
	opGEInt64
	opEqString
	opWildcard
	opRangeContainsInt64
)

// EqInt64 matches rows where field == v.
func EqInt64(field string, v int64) Predicate {
	return Predicate{op: opEqInt64, field: field, intVal: v}
}

// LEInt64 matches rows where field <= v.
func LEInt64(field string, v int64) Predicate {
	return Predicate{op: opLEInt64, field: field, intVal: v}
}

// GEInt64 matches rows where field >= v.
func GEInt64(field string, v int64) Predicate {
	return Predicate{op: opGEInt64, field: field, intVal: v}
}

// EqString matches rows where field == v (case-sensitive).
//
// If v is a canonical decimal integer (parses with strconv.ParseInt and round-trips through FormatInt), the parsed
// value is cached so int columns queried by stringified value can be pruned via min/max bounds.
func EqString(field string, v string) Predicate {
	n, err := strconv.ParseInt(v, 10, 64)
	canonical := err == nil && strconv.FormatInt(n, 10) == v
	return Predicate{op: opEqString, field: field, intVal: n, strVal: v, strValInt: canonical}
}

// Wildcard matches rows where field matches the shell-style glob pattern. '*' matches any (possibly empty) sequence,
// '?' matches exactly one rune. Matching is case-sensitive and anchored. If the pattern contains no glob
// metacharacters, this degrades to an exact-string equality (see EqString).
func Wildcard(field string, pattern string) Predicate {
	if !strings.ContainsAny(pattern, "*?") {
		return EqString(field, pattern)
	}
	return Predicate{op: opWildcard, field: field, strVal: pattern, pattern: compileGlob(pattern)}
}

// RangeContainsInt64 matches rows whose [startCol, endCol] range contains v (startCol <= v <= endCol).
// Used for IP-range lookups.
//
// When startCol is sorted ascending, DB.Query switches to an O(log N) binary search; this fast path assumes ranges in
// the data are non-overlapping (the usual case for IP geolocation tables). If ranges may overlap, the fast path can
// miss earlier matches.
func RangeContainsInt64(startCol, endCol string, v int64) Predicate {
	return Predicate{op: opRangeContainsInt64, field: startCol, field2: endCol, intVal: v}
}

func compileGlob(pat string) *regexp.Regexp {
	var b strings.Builder
	b.Grow(len(pat) + 4)
	b.WriteString("^")
	for _, r := range pat {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// Field returns the predicate's primary column.
func (p Predicate) Field() string { return p.field }

// Fields returns every column the predicate reads (one for single-column predicates, two for range predicates).
func (p Predicate) Fields() []string {
	if p.field2 != "" {
		return []string{p.field, p.field2}
	}
	return []string{p.field}
}

func (p Predicate) String() string {
	switch p.op {
	case opEqInt64:
		return fmt.Sprintf("%s == %d", p.field, p.intVal)
	case opLEInt64:
		return fmt.Sprintf("%s <= %d", p.field, p.intVal)
	case opGEInt64:
		return fmt.Sprintf("%s >= %d", p.field, p.intVal)
	case opEqString:
		return fmt.Sprintf("%s == %q", p.field, p.strVal)
	case opWildcard:
		return fmt.Sprintf("%s ~~ %q", p.field, p.strVal)
	case opRangeContainsInt64:
		return fmt.Sprintf("%s <= %d <= %s", p.field, p.intVal, p.field2)
	}
	return fmt.Sprintf("<unknown op %d>", p.op)
}

// matchInt64 evaluates a single-column int predicate against v.
func (p Predicate) matchInt64(v int64) bool {
	switch p.op {
	case opEqInt64:
		return v == p.intVal
	case opLEInt64:
		return v <= p.intVal
	case opGEInt64:
		return v >= p.intVal
	case opEqString, opWildcard, opRangeContainsInt64:
		return false
	}
	return false
}

// matchString evaluates a single-column string predicate against v.
func (p Predicate) matchString(v string) bool {
	switch p.op {
	case opEqString:
		return v == p.strVal
	case opWildcard:
		return p.pattern.MatchString(v)
	case opEqInt64, opLEInt64, opGEInt64, opRangeContainsInt64:
		return false
	}
	return false
}

// isStringOp reports whether the predicate operates on a string value. Used when a row's field is numeric: the engine
// stringifies it before matching.
func (p Predicate) isStringOp() bool {
	return p.op == opEqString || p.op == opWildcard
}

// matchRangeInt64 evaluates a range-contains predicate given the start and end column values from one row.
func (p Predicate) matchRangeInt64(start, end int64) bool {
	return p.op == opRangeContainsInt64 && start <= p.intVal && p.intVal <= end
}

// matchProjectedValue evaluates a single-column predicate against a raw value read directly from a column chunk during
// a projection scan. kind is the column's physical type. A null value is interpreted as the Go zero value (0 or "") so
// results match the full-decode path, where an absent optional column decodes to the struct field's zero value.
//
// Range predicates span two columns and are handled by the caller, not here.
func (p Predicate) matchProjectedValue(v parquet.Value, kind parquet.Kind) bool {
	switch kind { //nolint:exhaustive // unsupported kinds fall through to false, matching matchValue
	case parquet.Int32, parquet.Int64:
		var n int64
		if !v.IsNull() {
			if kind == parquet.Int32 {
				n = int64(v.Int32())
			} else {
				n = v.Int64()
			}
		}
		if p.isStringOp() {
			return p.matchString(strconv.FormatInt(n, 10))
		}
		return p.matchInt64(n)
	case parquet.ByteArray, parquet.FixedLenByteArray:
		var s string
		if !v.IsNull() {
			s = string(v.ByteArray())
		}
		return p.matchString(s)
	default:
		return false
	}
}

// canMatchPage reports whether a page of column `field` whose bounds are [lo, hi] could contain a row that satisfies
// this predicate. Used for row-group pruning. `field` must be one of p.Fields().
func (p Predicate) canMatchPage(field string, lo, hi parquet.Value) bool {
	switch p.op {
	case opEqInt64:
		return p.intVal >= lo.Int64() && p.intVal <= hi.Int64()
	case opLEInt64:
		return lo.Int64() <= p.intVal
	case opGEInt64:
		return hi.Int64() >= p.intVal
	case opEqString:
		switch lo.Kind() { //nolint:exhaustive // other kinds fall through to no-prune
		case parquet.ByteArray, parquet.FixedLenByteArray:
			return string(lo.ByteArray()) <= p.strVal && p.strVal <= string(hi.ByteArray())
		case parquet.Int32, parquet.Int64:
			if p.strValInt {
				return p.intVal >= lo.Int64() && p.intVal <= hi.Int64()
			}
			return false
		}
	case opWildcard:
		return true
	case opRangeContainsInt64:
		if field == p.field {
			// Start column: some row could satisfy start <= v iff min(start) <= v.
			return lo.Int64() <= p.intVal
		}
		// End column: some row could satisfy end >= v iff max(end) >= v.
		return hi.Int64() >= p.intVal
	}
	return false
}
