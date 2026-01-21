package main

import (
	"strconv"
	"strings"
	"time"

	"okinoko_dao/sdk"
)

// -----------------------------------------------------------------------------
// State Utilities
// -----------------------------------------------------------------------------

// stateSetIfChanged avoids unnecessary writes so we dont thrash storage fees.
func stateSetIfChanged(key, value string) {
	if existing := sdk.StateGetObject(key); existing != nil && *existing == value {
		return
	}
	sdk.StateSetObject(key, value)
}

// -----------------------------------------------------------------------------
// Counter Operations
// -----------------------------------------------------------------------------

// getCount reads the string counter under the key and defaults to zero, nothing magical here.
func getCount(key string) uint64 {
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return 0
	}
	n, _ := strconv.ParseUint(*ptr, 10, 64)
	return n
}

// setCount stores uint64 counters back as decimal strings for the host kv.
func setCount(key string, n uint64) {
	sdk.StateSetObject(key, strconv.FormatUint(n, 10))
}

// -----------------------------------------------------------------------------
// String Conversion Helpers
// -----------------------------------------------------------------------------

// UInt64ToString turns an id back into decimal text for logs or env payload building.
// Example payload: UInt64ToString(9001)
func UInt64ToString(val uint64) string {
	return strconv.FormatUint(val, 10)
}

// UIntSliceToString helps event logging since we encode []uint choices as 1,2,5 etc.
// Example payload: UIntSliceToString([]uint{0,2,3})
func UIntSliceToString(nums []uint) string {
	strNums := make([]string, len(nums))
	for i, n := range nums {
		strNums[i] = strconv.FormatUint(uint64(n), 10)
	}
	return strings.Join(strNums, ",")
}

// -----------------------------------------------------------------------------
// Timestamp Helpers
// -----------------------------------------------------------------------------

// nowUnix returns the current Unix timestamp.
// It prefers the chain's block timestamp from the environment if available.
func nowUnix() int64 {
	if ts := currentEnv().Timestamp; ts != "" {
		if v, ok := parseTimestamp(ts); ok {
			return v
		}
	}
	if tsPtr := sdk.GetEnvKey("block.timestamp"); tsPtr != nil && *tsPtr != "" {
		if v, ok := parseTimestamp(*tsPtr); ok {
			return v
		}
	}
	return time.Now().Unix()
}

// parseTimestamp accepts unix seconds or iso-ish strings since the env flips formats sometimes.
func parseTimestamp(val string) (int64, bool) {
	if v, err := strconv.ParseInt(val, 10, 64); err == nil {
		return v, true
	}
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t.Unix(), true
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", val, time.UTC); err == nil {
		return t.Unix(), true
	}
	return 0, false
}
