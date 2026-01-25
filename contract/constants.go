package main

import "okinoko_dao/sdk"

// -----------------------------------------------------------------------------
// Supported Assets
// -----------------------------------------------------------------------------

// validAssets lists the supported asset types for treasury and transfers.
var validAssets = []string{
	sdk.AssetHbd.String(),
	sdk.AssetHive.String(),
	sdk.AssetHbdSavings.String(),
}

// -----------------------------------------------------------------------------
// Amount Scaling
// -----------------------------------------------------------------------------

// AmountScale defines the precision multiplier for converting floats to int64.
const AmountScale = 1000

// -----------------------------------------------------------------------------
// Validation Limits
// -----------------------------------------------------------------------------

const (
	// MaxNameLength limits the size of project and proposal names.
	MaxNameLength = 128
	// MaxDescriptionLength limits the size of project and proposal descriptions.
	MaxDescriptionLength = 512
	// MaxOptionTextLength limits the size of proposal option text.
	MaxOptionTextLength = 500
	// MaxURLLength limits the size of URLs (for projects, proposals, and options).
	MaxURLLength = 500
	// MaxProposalOptions limits the number of options per proposal.
	MaxProposalOptions = 50
	// MaxPayoutReceivers limits the number of payout entries per proposal.
	MaxPayoutReceivers = 50
	// MaxWhitelistAddresses limits the number of addresses per whitelist operation.
	MaxWhitelistAddresses = 50
	// MaxKickAddresses limits the number of addresses per kick_member operation.
	MaxKickAddresses = 50
	// MinProposalDurationHours enforces a minimum voting period.
	MinProposalDurationHours = 1
	// MinThresholdPercent is the minimum allowed threshold percentage.
	MinThresholdPercent = 1.0
	// MaxThresholdPercent is the maximum allowed threshold percentage.
	MaxThresholdPercent = 100.0
	// MinQuorumPercent is the minimum allowed quorum percentage.
	MinQuorumPercent = 1.0
	// MaxQuorumPercent is the maximum allowed quorum percentage.
	MaxQuorumPercent = 100.0
)

// -----------------------------------------------------------------------------
// Default/Fallback Values
// -----------------------------------------------------------------------------

const (
	FallbackNFTContract                 = ""
	FallbackNFTFunction                 = ""
	FallbackThresholdPercent            = 50.001
	FallbackQuorumPercent               = 50.001
	FallbackProposalDurationHours       = 24
	FallbackExecutionDelayHours         = 4
	FallbackLeaveCooldownHours          = 24
	FallbackProposalCost                = 1
	FallbackProposalCreatorsMembersOnly = true
	FallbackMembershipPayloadFormat     = "{nft}|{caller}"
)

// -----------------------------------------------------------------------------
// Counter Keys
// -----------------------------------------------------------------------------

const (
	// VotesCount holds an integer counter for votes (used for generating IDs).
	VotesCount = "count:v"
	// ProposalsCount holds an integer counter for proposals (used for generating IDs).
	ProposalsCount = "count:props"
	// ProjectsCount holds an integer counter for projects (used for generating IDs).
	ProjectsCount = "count:proj"
)

// -----------------------------------------------------------------------------
// Storage Key Prefixes
// -----------------------------------------------------------------------------

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
	// kProjectTreasury stores per-asset balances in multi-asset treasury.
	kProjectTreasury byte = 0x07
	// kProposalMeta contains encoded Proposal records.
	kProposalMeta byte = 0x10
	// kProposalOption stores ProposalOption entries indexed by proposal+option index.
	kProposalOption byte = 0x11
	// kVoteReceipt is reserved for future vote receipts (unused today but kept for layout clarity).
	kVoteReceipt byte = 0x20
	// kMemberStakeHistory stores historical stake snapshots: {stake}_{timestamp}
	kMemberStakeHistory byte = 0x22
)

// -----------------------------------------------------------------------------
// Voting Systems
// -----------------------------------------------------------------------------

const (
	VotingSystemDemocratic VotingSystem = 0
	VotingSystemStake      VotingSystem = 1
)

// -----------------------------------------------------------------------------
// Proposal States
// -----------------------------------------------------------------------------

const (
	ProposalStateUnspecified ProposalState = 0
	ProposalActive           ProposalState = 1
	ProposalClosed           ProposalState = 2
	ProposalPassed           ProposalState = 3
	ProposalExecuted         ProposalState = 4
	ProposalFailed           ProposalState = 5
	ProposalCancelled        ProposalState = 6
)
