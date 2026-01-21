package main

import (
	"fmt"
	"strconv"
	"strings"

	"okinoko_dao/sdk"
)

// StakeHistoryEntry represents a single stake snapshot in time.
type StakeHistoryEntry struct {
	Stake     Amount
	Timestamp int64
}

// saveStakeHistory appends a new stake history entry for a member.
// Increments the member's StakeIncrement counter.
func saveStakeHistory(projectID uint64, addr sdk.Address, stake Amount, timestamp int64, increment uint64) {
	key := memberStakeHistoryKey(projectID, addr, increment)
	value := fmt.Sprintf("%d_%d", stake, timestamp)
	sdk.StateSetObject(key, value)
}

// loadStakeHistory retrieves a specific stake history entry by increment.
func loadStakeHistory(projectID uint64, addr sdk.Address, increment uint64) *StakeHistoryEntry {
	key := memberStakeHistoryKey(projectID, addr, increment)
	dataPtr := sdk.StateGetObject(key)
	if dataPtr == nil {
		return nil
	}

	// Parse format: {stake}_{timestamp}
	parts := strings.Split(*dataPtr, "_")
	if len(parts) != 2 {
		return nil
	}

	stake, err1 := strconv.ParseInt(parts[0], 10, 64)
	timestamp, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil {
		return nil
	}

	return &StakeHistoryEntry{
		Stake:     Amount(stake),
		Timestamp: timestamp,
	}
}

// getStakeAtTime finds the member's stake at a specific timestamp by searching backwards
// through their stake history from their current increment.
func getStakeAtTime(projectID uint64, addr sdk.Address, targetTime int64, currentIncrement uint64) Amount {
	// Search backwards from current increment to 0
	for i := int64(currentIncrement); i >= 0; i-- {
		entry := loadStakeHistory(projectID, addr, uint64(i))
		if entry == nil {
			continue
		}

		// Found an entry at or before the target time
		if entry.Timestamp <= targetTime {
			return entry.Stake
		}
	}

	// Should never happen if stake history is properly maintained
	return 0
}

// deleteAllStakeHistory removes all stake history entries for a member when they leave.
func deleteAllStakeHistory(projectID uint64, addr sdk.Address, maxIncrement uint64) {
	for i := uint64(0); i <= maxIncrement; i++ {
		key := memberStakeHistoryKey(projectID, addr, i)
		sdk.StateDeleteObject(key)
	}
}
