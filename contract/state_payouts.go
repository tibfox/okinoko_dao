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

// incrementPayoutLocks loops the payout map so each beneficiary gets a lock entry.
func incrementPayoutLocks(projectID uint64, payout map[sdk.Address]Amount) {
	if payout == nil {
		return
	}
	for addr := range payout {
		incrementPayoutLock(projectID, addr)
	}
}

// decrementPayoutLocks removes all locks once a proposal outcome resolves.
func decrementPayoutLocks(projectID uint64, payout map[sdk.Address]Amount) {
	if payout == nil {
		return
	}
	for addr := range payout {
		decrementPayoutLock(projectID, addr)
	}
}
