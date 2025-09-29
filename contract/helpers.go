package main

import (
	"encoding/json"
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
	"strings"
	"time"
)

////////////////////////////////////////////////////////////////////////////////
// Helpers: keys, guids, time
////////////////////////////////////////////////////////////////////////////////

// TransferAllow represents arguments extracted from a transfer.allow intent.
// It specifies the allowed transfer amount (`Limit`) and the asset (`Token`).
type TransferAllow struct {
	Limit float64
	Token sdk.Asset
}

// validAssets lists the supported asset types.
var validAssets = []string{sdk.AssetHbd.String(), sdk.AssetHive.String()}

// isValidAsset checks if a given token string is one of the supported assets.
func isValidAsset(token string) bool {
	for _, a := range validAssets {
		if token == a {
			return true
		}
	}
	return false
}

// getFirstTransferAllow scans the provided intents and returns the first valid
// transfer.allow intent as a TransferAllow object. Returns nil if none found.
func getFirstTransferAllow(intents []sdk.Intent) *TransferAllow {
	for _, intent := range intents {
		if intent.Type == "transfer.allow" {
			token := intent.Args["token"]
			if !isValidAsset(token) {
				sdk.Abort("invalid intent asset")
			}
			limitStr := intent.Args["limit"]
			limit, err := strconv.ParseFloat(limitStr, 32)
			if err != nil {
				sdk.Abort("invalid intent limit")
			}
			ta := &TransferAllow{
				Limit: limit,
				Token: sdk.Asset(token),
			}
			return ta
		}
	}
	return nil
}

// getSenderAddress returns the address of the current transaction sender.
func getSenderAddress() sdk.Address {
	return sdk.GetEnv().Sender.Address
}

// projectKey builds a storage key string for a project by ID.
func projectKey(id uint64) string {
	return "prj:" + UInt64ToString(id)
}

// proposalKey builds a storage key string for a proposal by ID.
func proposalKey(id uint64) string {
	return "prpsl:" + UInt64ToString(id)
}

// nowUnix returns the current Unix timestamp.
// It prefers the chain's block timestamp from the environment if available.
func nowUnix() int64 {
	// try chain timestamp via env key
	if tsPtr := sdk.GetEnvKey("block.timestamp"); tsPtr != nil && *tsPtr != "" {
		// try parse as integer seconds
		if v, err := strconv.ParseInt(*tsPtr, 10, 64); err == nil {
			return v
		}
		// try RFC3339
		if t, err := time.Parse(time.RFC3339, *tsPtr); err == nil {
			return t.Unix()
		}
	}
	return time.Now().Unix()
}

///////////////////////////////////////////////////
// Conversions from/to JSON strings
///////////////////////////////////////////////////

// ToJSON marshals a Go value into a JSON string.
// Aborts execution if marshalling fails.
func ToJSON[T any](v T, objectType string) string {
	b, err := json.Marshal(v)
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to marshal %s\nInput data:%+v\nError: %v:", objectType, v, err))
	}
	return string(b)
}

// FromJSON unmarshals a JSON string into a Go value of type T.
// Aborts execution if unmarshalling fails.
func FromJSON[T any](data string, objectType string) *T {
	data = strings.TrimSpace(data)
	var v T
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		sdk.Abort(fmt.Sprintf(
			"failed to unmarshal %s\nInput data:%s\nError: %v:", objectType, data, err))
	}
	return &v
}

// strptr returns a pointer to the provided string.
func strptr(s string) *string { return &s }

///////////////////////////////////////////////////
// Counter helpers
///////////////////////////////////////////////////

// Index key prefixes for counting entities.
const (
	// VotesCount holds an integer counter for votes (used for generating IDs).
	VotesCount = "count:v"

	// ProposalsCount holds an integer counter for proposals (used for generating IDs).
	ProposalsCount = "count:props"

	// ProjectsCount holds an integer counter for projects (used for generating IDs).
	ProjectsCount = "count:proj"
)

// getCount retrieves the counter value stored under a given key.
// Returns 0 if the key is not set.
func getCount(key string) uint64 {
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return 0
	}
	n, _ := strconv.ParseUint(*ptr, 10, 64)
	return n
}

// setCount updates the counter value stored under a given key.
func setCount(key string, n uint64) {
	sdk.StateSetObject(key, strconv.FormatUint(n, 10))
}

// StringToUInt64 converts a string pointer to a uint64.
// Aborts if the pointer is nil or parsing fails.
func StringToUInt64(ptr *string) uint64 {
	if ptr == nil {
		sdk.Abort("input is empty")
	}
	val, err := strconv.ParseUint(*ptr, 10, 64) // base 10, 64-bit
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to parse '%s' to uint64: %w", *ptr, err))
	}
	return val
}

// UInt64ToString converts a uint64 to its decimal string representation.
func UInt64ToString(val uint64) string {
	return strconv.FormatUint(val, 10)
}

// UIntSliceToString converts a slice of uint values to a comma-separated string.
func UIntSliceToString(nums []uint) string {
	strNums := make([]string, len(nums))
	for i, n := range nums {
		strNums[i] = strconv.FormatUint(uint64(n), 10)
	}
	return strings.Join(strNums, ",")
}
