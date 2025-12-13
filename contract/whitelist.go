package main

import "okinoko_dao/sdk"

// isProjectMember is a tiny helper to skip whitelist updates for existing members.
func isProjectMember(projectID uint64, addr sdk.Address) bool {
	_, exists := loadMember(projectID, addr)
	return exists
}

// addWhitelistEntries stores approvals for provided addresses and returns the successful set.
func addWhitelistEntries(projectID uint64, addresses []sdk.Address) []sdk.Address {
	added := make([]sdk.Address, 0, len(addresses))
	seen := map[string]struct{}{}
	for _, addr := range addresses {
		addrStr := AddressToString(addr)
		if _, ok := seen[addrStr]; ok {
			continue
		}
		seen[addrStr] = struct{}{}
		if isProjectMember(projectID, addr) {
			continue
		}
		if setWhitelistEntry(projectID, addr) {
			added = append(added, addr)
		}
	}
	return added
}

// removeWhitelistEntries clears approvals for provided addresses and returns the removed set.
func removeWhitelistEntries(projectID uint64, addresses []sdk.Address) []sdk.Address {
	removed := make([]sdk.Address, 0, len(addresses))
	seen := map[string]struct{}{}
	for _, addr := range addresses {
		addrStr := AddressToString(addr)
		if _, ok := seen[addrStr]; ok {
			continue
		}
		seen[addrStr] = struct{}{}
		if isProjectMember(projectID, addr) {
			continue
		}
		if deleteWhitelistEntry(projectID, addr) {
			removed = append(removed, addr)
		}
	}
	return removed
}
