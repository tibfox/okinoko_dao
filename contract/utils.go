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
	// NEVER fall back to wall-clock time. nowUnix() feeds consensus-critical state
	// (proposal CreatedAt, member JoinedAt, stake-history timestamps), so a
	// per-node time.Now() would stamp divergent values into state on every
	// validator — an immediate chain fork. A missing/unparseable block timestamp
	// is an unrecoverable environment error, so fail deterministically instead.
	sdk.Abort("block timestamp unavailable")
	return 0
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

// -----------------------------------------------------------------------------
// Type Conversion Helpers
// -----------------------------------------------------------------------------

// AddressFromString converts a human string to the platform-specific address wrapper.
func AddressFromString(s string) sdk.Address { return sdk.Address(s) }

// AddressToString turns the wrapped type back into the underlying string.
func AddressToString(a sdk.Address) string { return a.String() }

// validateAddress rejects user-supplied addresses that could forge state keys
// or corrupt the delimiter-based records this contract encodes. State keys pack
// the address as their trailing (variable-length) field, and several payload /
// event encodings split on the bytes below; a well-formed address
// ("hive:<username>") never contains them. Colons are intentionally allowed
// because they are part of the address itself. Call this at every point where an
// address first enters from an untrusted payload.
func validateAddress(addr sdk.Address) {
	s := addr.String()
	if s == "" {
		sdk.Abort("address required")
	}
	if len(s) > MaxAddressLength {
		sdk.Abort("address exceeds maximum length")
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		// Reject the structural delimiters ('|' state keys/config, ';' and ','
		// record separators) and any control/whitespace byte (<= 0x20).
		if c == '|' || c == ';' || c == ',' || c <= ' ' {
			sdk.Abort("invalid character in address")
		}
	}
}

// -----------------------------------------------------------------------------
// Overflow-safe Amount Math
// -----------------------------------------------------------------------------

// safeAddAmount adds two scaled Amounts and aborts on int64 overflow rather than
// silently wrapping into a negative balance (which would bypass the
// insufficient-funds guards downstream).
func safeAddAmount(a, b Amount) Amount {
	if b > 0 && a > maxAmount-b {
		sdk.Abort("amount overflow")
	}
	if b < 0 && a < minAmount-b {
		sdk.Abort("amount underflow")
	}
	return a + b
}

// AssetFromString wraps a ticker string so type checking keeps us honest.
func AssetFromString(s string) sdk.Asset { return sdk.Asset(s) }

// AssetToString unwraps the ticker string for logs or SDK calls.
func AssetToString(a sdk.Asset) string { return a.String() }

// hasOwner checks if a project has an owner (is not autonomous).
// Projects can become autonomous via the "remove_owner" proposal meta option.
func hasOwner(prj *Project) bool {
	return prj.Owner.String() != ""
}

// -----------------------------------------------------------------------------
// Contract Existence Helpers
// -----------------------------------------------------------------------------

// contractExists checks if a contract is registered by reading its state.
// Returns true if the contract exists, false otherwise.
func contractExists(contractId string) bool {
	result := sdk.ContractStateGet(contractId, "a")
	return result != nil
}
