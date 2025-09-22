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

// New struct for transfer.allow args
type TransferAllow struct {
	Limit float64
	Token sdk.Asset
}

var validAssets = []string{sdk.AssetHbd.String(), sdk.AssetHive.String()}

// Helper function to validate token
func isValidAsset(token string) bool {
	for _, a := range validAssets {
		if token == a {
			return true
		}
	}
	return false
}

// Helper function to get the first transfer.allow intent
func getFirstTransferAllow(intents []sdk.Intent) *TransferAllow {
	for _, intent := range intents {
		if intent.Type == "transfer.allow" {
			token := intent.Args["token"]
			if !isValidAsset(token) {
				sdk.Abort("invalid intent asset")
			}
			limitStr := intent.Args["limit"]
			limit, err := strconv.ParseFloat(limitStr, 64)
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

func getSenderAddress() sdk.Address {
	return sdk.GetEnv().Sender.Address
}

func projectKey(id uint64) string {
	return "prj:" + UInt64ToString(id)
}

func proposalKey(id uint64) string {
	return "prpsl:" + UInt64ToString(id)
}

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
// Conversions from/to json strings
///////////////////////////////////////////////////

func ToJSON[T any](v T, objectType string) string {
	b, err := json.Marshal(v)
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to marshal %s\nInput data:%+v\nError: %v:", objectType, v, err))
	}
	return string(b)
}

func FromJSON[T any](data string, objectType string) *T {
	data = strings.TrimSpace(data)
	var v T
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		sdk.Abort(fmt.Sprintf(
			"failed to unmarshal %s\nInput data:%s\nError: %v:", objectType, data, err))
	}
	return &v
}

// Convenience helper
func strptr(s string) *string { return &s }

// COUNT stuff

// index key prefixes
const (
	VotesCount     = "count:v"     // 					// holds a int counter for votes (to create new ids)
	ProposalsCount = "count:props" // 					// holds a int counter for proposals (to create new ids)
	ProjectsCount  = "count:proj"  // 					// holds a int counter for projects (to create new ids)

)

func getCount(key string) uint64 {
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return 0
	}
	n, _ := strconv.ParseUint(*ptr, 10, 64)
	return n
}

func setCount(key string, n uint64) {
	sdk.StateSetObject(key, strconv.FormatUint(n, 10))
}

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

func UInt64ToString(val uint64) string {
	return strconv.FormatUint(val, 10)
}

// UIntSliceToString converts a slice of ints to a comma-separated string
func UIntSliceToString(nums []uint) string {
	strNums := make([]string, len(nums))
	for i, n := range nums {
		strNums[i] = strconv.FormatUint(uint64(n), 10)
	}
	return strings.Join(strNums, ",")
}
