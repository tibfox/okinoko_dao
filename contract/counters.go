package main

import (
	"okinoko_dao/sdk"
	"strconv"
	"strings"
)

// Index key prefixes for counting entities.
const (
	// VotesCount holds an integer counter for votes (used for generating IDs).
	VotesCount = "count:v"
	// ProposalsCount holds an integer counter for proposals (used for generating IDs).
	ProposalsCount = "count:props"
	// ProjectsCount holds an integer counter for projects (used for generating IDs).
	ProjectsCount = "count:proj"
)

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
