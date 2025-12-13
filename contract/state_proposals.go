package main

import "okinoko_dao/sdk"

// saveProposalOption stores each option separately to avoid rewriting the whole proposal blob.
func saveProposalOption(proposalID uint64, idx uint32, opt *ProposalOption) {
	key := proposalOptionKey(proposalID, idx)
	data := EncodeProposalOption(opt)
	sdk.StateSetObject(key, string(data))
}

// loadProposalOption decodes a single option and aborts loudly when missing.
func loadProposalOption(proposalID uint64, idx uint32) *ProposalOption {
	key := proposalOptionKey(proposalID, idx)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("proposal option not found")
	}
	opt, err := DecodeProposalOption([]byte(*ptr))
	if err != nil {
		sdk.Abort("failed to decode proposal option")
	}
	return opt
}

// loadProposalOptions iterates indexes and returns a flat slice for tallying.
func loadProposalOptions(proposalID uint64, count uint32) []ProposalOption {
	opts := make([]ProposalOption, count)
	for i := uint32(0); i < count; i++ {
		opt := loadProposalOption(proposalID, i)
		opts[i] = *opt
	}
	return opts
}
