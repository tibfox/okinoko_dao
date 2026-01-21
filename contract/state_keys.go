package main

import "okinoko_dao/sdk"

// packU64LEInline sprinkles a uint64 into dst in little-endian order so our keys stay compact.
func packU64LEInline(x uint64, dst []byte) {
	dst[0] = byte(x)
	dst[1] = byte(x >> 8)
	dst[2] = byte(x >> 16)
	dst[3] = byte(x >> 24)
	dst[4] = byte(x >> 32)
	dst[5] = byte(x >> 40)
	dst[6] = byte(x >> 48)
	dst[7] = byte(x >> 56)
}

// packU32LEInline mirrors the 64-bit helper but for smaller option indexes.
func packU32LEInline(x uint32, dst []byte) {
	dst[0] = byte(x)
	dst[1] = byte(x >> 8)
	dst[2] = byte(x >> 16)
	dst[3] = byte(x >> 24)
}

// packU64LE appends the encoded number to dst and returns the new slice.
func packU64LE(x uint64, dst []byte) []byte {
	return append(dst,
		byte(x),
		byte(x>>8),
		byte(x>>16),
		byte(x>>24),
		byte(x>>32),
		byte(x>>40),
		byte(x>>48),
		byte(x>>56),
	)
}

// projectKey builds a storage key string for a project by ID.
func projectKey(id uint64) string {
	var buf [9]byte
	buf[0] = kProjectMeta
	packU64LEInline(id, buf[1:])
	return string(buf[:])
}

// projectConfigKey uses prefix 0x02 so configs sit next to meta but not collide.
func projectConfigKey(id uint64) string {
	var buf [9]byte
	buf[0] = kProjectConfig
	packU64LEInline(id, buf[1:])
	return string(buf[:])
}

// projectFinanceKey sits in prefix 0x03 for quick aggregated lookups.
func projectFinanceKey(id uint64) string {
	var buf [9]byte
	buf[0] = kProjectFinance
	packU64LEInline(id, buf[1:])
	return string(buf[:])
}

// memberKey mixes project id plus address bytes to avoid nested maps in host storage.
func memberKey(projectID uint64, addr sdk.Address) string {
	addrStr := AddressToString(addr)
	buf := make([]byte, 0, 1+8+len(addrStr))
	buf = append(buf, kProjectMember)
	buf = packU64LE(projectID, buf)
	buf = append(buf, addrStr...)
	return string(buf)
}

// payoutLockKey counts pending payouts for a member so we can block exits safely.
func payoutLockKey(projectID uint64, addr sdk.Address) string {
	addrStr := AddressToString(addr)
	buf := make([]byte, 0, 1+8+len(addrStr))
	buf = append(buf, kProjectPayoutLock)
	buf = packU64LE(projectID, buf)
	buf = append(buf, addrStr...)
	return string(buf)
}

// whitelistKey mirrors member keys but keeps approvals in a separate prefix.
func whitelistKey(projectID uint64, addr sdk.Address) string {
	addrStr := AddressToString(addr)
	buf := make([]byte, 0, 1+8+len(addrStr))
	buf = append(buf, kProjectWhitelist)
	buf = packU64LE(projectID, buf)
	buf = append(buf, addrStr...)
	return string(buf)
}

// proposalKey builds a storage key string for a proposal by ID.
// proposalKey encodes id under 0x10 prefix keeping metadata lumps contiguous.
func proposalKey(id uint64) string {
	var buf [9]byte
	buf[0] = kProposalMeta
	packU64LEInline(id, buf[1:])
	return string(buf[:])
}

// proposalOptionKey stores options sequentially under 0x11 prefix.
func proposalOptionKey(id uint64, idx uint32) string {
	var buf [13]byte
	buf[0] = kProposalOption
	packU64LEInline(id, buf[1:])
	packU32LEInline(idx, buf[9:])
	return string(buf[:])
}

// memberStakeHistoryKey stores a member's stake history entry at a specific increment.
// Key format: kMemberStakeHistory|projectID|address|increment
// Value format: {stake}_{timestamp}
func memberStakeHistoryKey(projectID uint64, addr sdk.Address, increment uint64) string {
	addrStr := AddressToString(addr)
	buf := make([]byte, 0, 1+8+len(addrStr)+8)
	buf = append(buf, kMemberStakeHistory)
	buf = packU64LE(projectID, buf)
	buf = append(buf, addrStr...)
	buf = packU64LE(increment, buf)
	return string(buf)
}

// projectTreasuryKey stores a single asset balance in the project's multi-asset treasury.
// Key format: kProjectTreasury|projectID|asset
// Value format: {amount}
func projectTreasuryKey(projectID uint64, asset sdk.Asset) string {
	assetStr := asset.String()
	buf := make([]byte, 0, 1+8+len(assetStr))
	buf = append(buf, kProjectTreasury)
	buf = packU64LE(projectID, buf)
	buf = append(buf, assetStr...)
	return string(buf)
}
