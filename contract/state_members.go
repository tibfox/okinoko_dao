package main

import "okinoko_dao/sdk"

// saveMember writes both storage and cache copy so repeated reads stay cheap.
func saveMember(projectID uint64, member *Member) {
	key := memberKey(projectID, member.Address)
	data := EncodeMember(member)
	sdk.StateSetObject(key, string(data))
	if cachedMembers != nil {
		cp := *member
		cachedMembers[key] = &cp
	}
}

// loadMember tries cache first and decodes wasm bytes when needed.
func loadMember(projectID uint64, addr sdk.Address) (*Member, bool) {
	key := memberKey(projectID, addr)
	if cachedMembers != nil {
		if cached, ok := cachedMembers[key]; ok {
			cp := *cached
			return &cp, true
		}
	}
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return nil, false
	}
	member, err := DecodeMember([]byte(*ptr))
	if err != nil {
		sdk.Abort("failed to decode member")
	}
	if cachedMembers != nil {
		cp := *member
		cachedMembers[key] = &cp
	}
	return member, true
}

// deleteMember evicts member state and removes cached clone to avoid stale reads.
func deleteMember(projectID uint64, addr sdk.Address) {
	key := memberKey(projectID, addr)
	sdk.StateDeleteObject(key)
	if cachedMembers != nil {
		delete(cachedMembers, key)
	}
}
