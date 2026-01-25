package main

import (
	"okinoko_dao/sdk"
	"strconv"
)

// getPayoutLockCount reads how many pending payouts block withdrawals for this addr.
func getPayoutLockCount(projectID uint64, addr sdk.Address) uint64 {
	key := payoutLockKey(projectID, addr)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return 0
	}
	val, err := strconv.ParseUint(*ptr, 10, 64)
	if err != nil {
		sdk.Abort("invalid payout lock value")
	}
	return val
}

// incrementPayoutLock bumps the counter when a new payout is tied to the member.
func incrementPayoutLock(projectID uint64, addr sdk.Address) {
	key := payoutLockKey(projectID, addr)
	count := getPayoutLockCount(projectID, addr) + 1
	sdk.StateSetObject(key, strconv.FormatUint(count, 10))
}

// decrementPayoutLock lowers the counter and deletes key when it reaches zero.
func decrementPayoutLock(projectID uint64, addr sdk.Address) {
	key := payoutLockKey(projectID, addr)
	count := getPayoutLockCount(projectID, addr)
	if count == 0 {
		return
	}
	count--
	if count == 0 {
		sdk.StateDeleteObject(key)
	} else {
		sdk.StateSetObject(key, strconv.FormatUint(count, 10))
	}
}

// incrementPayoutLocks loops the payout slice so each unique beneficiary gets a lock entry.
func incrementPayoutLocks(projectID uint64, payout []PayoutEntry) {
	if len(payout) == 0 {
		return
	}
	// Track unique addresses to avoid double-incrementing
	seen := make(map[string]bool)
	for _, entry := range payout {
		addrStr := entry.Address.String()
		if seen[addrStr] {
			continue
		}
		seen[addrStr] = true
		incrementPayoutLock(projectID, entry.Address)
	}
}

// decrementPayoutLocks removes all locks once a proposal outcome resolves.
func decrementPayoutLocks(projectID uint64, payout []PayoutEntry) {
	if len(payout) == 0 {
		return
	}
	// Track unique addresses to avoid double-decrementing
	seen := make(map[string]bool)
	for _, entry := range payout {
		addrStr := entry.Address.String()
		if seen[addrStr] {
			continue
		}
		seen[addrStr] = true
		decrementPayoutLock(projectID, entry.Address)
	}
}
