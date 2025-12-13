package main

import "okinoko_dao/sdk"

const (
	// kProjectMeta stores serialized ProjectMeta blobs.
	kProjectMeta byte = 0x01
	// kProjectConfig stores ProjectConfig fragments so config updates touch fewer bytes.
	kProjectConfig byte = 0x02
	// kProjectFinance tracks ProjectFinance (funds, member counts, stake totals).
	kProjectFinance byte = 0x03
	// kProjectMember houses encoded Member structs (project scoped).
	kProjectMember byte = 0x04
	// kProjectPayoutLock counts pending payouts per member to guard exits.
	kProjectPayoutLock byte = 0x05
	// kProjectWhitelist flags pending manual membership approvals.
	kProjectWhitelist byte = 0x06
	// kProposalMeta contains encoded Proposal records.
	kProposalMeta byte = 0x10
	// kProposalOption stores ProposalOption entries indexed by proposal+option index.
	kProposalOption byte = 0x11
	// kVoteReceipt is reserved for future vote receipts (unused today but kept for layout clarity).
	kVoteReceipt byte = 0x20
)

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
