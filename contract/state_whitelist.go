package main

import "okinoko_dao/sdk"

// setWhitelistEntry stores a short-lived approval entry.
func setWhitelistEntry(projectID uint64, addr sdk.Address) bool {
	key := whitelistKey(projectID, addr)
	if existing := sdk.StateGetObject(key); existing != nil && *existing != "" {
		return false
	}
	sdk.StateSetObject(key, "1")
	return true
}

// deleteWhitelistEntry removes a pending approval and reports whether it existed.
func deleteWhitelistEntry(projectID uint64, addr sdk.Address) bool {
	key := whitelistKey(projectID, addr)
	existing := sdk.StateGetObject(key)
	if existing == nil || *existing == "" {
		return false
	}
	sdk.StateDeleteObject(key)
	return true
}

// isWhitelisted reports whether an address holds a pending approval.
func isWhitelisted(projectID uint64, addr sdk.Address) bool {
	key := whitelistKey(projectID, addr)
	existing := sdk.StateGetObject(key)
	return existing != nil && *existing != ""
}
