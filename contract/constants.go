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

// validAddressPrefixes lists the address namespaces this contract accepts. VSC
// account addresses are "hive:<username>"; contracts and keys use a "did:" URI.
// validateAddress requires one of these plus a non-empty body, so a malformed
// beneficiary cannot silently receive an irreversible treasury payout.
var validAddressPrefixes = []string{"hive:", "did:"}

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
	// MaxProposalOptions limits the number of options per proposal. Each option is
	// a separate state write plus an entry in the creation event; the wasm heap
	// exhausts (nil-deref trap) around ~46 options, so this is capped well below
	// that so the advertised maximum is always reachable, not just parseable.
	MaxProposalOptions = 40
	// MaxPayoutReceivers limits the number of payout entries per proposal.
	MaxPayoutReceivers = 50
	// MaxWhitelistAddresses limits the number of addresses per whitelist operation.
	MaxWhitelistAddresses = 50
	// MaxAddressLength bounds any user-supplied address string. Hive addresses
	// ("hive:<username>") are short; this cap keeps forged keys/records from
	// bloating state.
	MaxAddressLength = 128
	// MaxKickAddresses limits the number of addresses per kick_member operation.
	MaxKickAddresses = 50
	// MaxMetaLength bounds the outcome-meta blob. It must accommodate the largest
	// LEGITIMATE directive, which is whitelist_add/kick_member carrying
	// MaxWhitelistAddresses (50) x MaxAddressLength (128) plus separators, so it is
	// necessarily much larger than MaxDescriptionLength.
	MaxMetaLength = 8192
	// MaxICCCalls limits inter-contract calls per proposal. Each one is an external
	// call plus a treasury debit executed inside a single ExecuteProposal.
	MaxICCCalls = 20
	// MinProposalDurationHours enforces a minimum voting period.
	MinProposalDurationHours = 1
	// MaxDurationHours caps execution delay and leave cooldown.
	// Any value * 3600 must stay well within int64, so this also prevents the
	// deadline/execution-time integer overflow. 10 years is far beyond any real use.
	MaxDurationHours = 87600
	// MaxProposalDurationHours caps a single proposal's VOTING period at 90 days.
	//
	// This is tighter than MaxDurationHours for a security reason, not a UX one.
	// Creating a proposal takes a payout lock on every named beneficiary
	// (CreateProposal -> incrementPayoutLocks), which blocks their project_leave and
	// their removal via kick_member until the proposal is tallied — and tallying is
	// impossible before the deadline the CREATOR chose. Since anyone may name anyone
	// as a beneficiary without consent, the voting period is the exact length of time
	// one member can freeze another member's stake. At MaxDurationHours that was ten
	// years; this bounds it to 90 days.
	//
	// NOTE: this bounds the griefing window, it does not close it. A hostile member
	// can still freeze a victim for up to 90 days, and stacking proposals defeats a
	// per-proposal cancel. Full mitigation requires either taking the lock only once
	// a proposal PASSES, or letting a named beneficiary decline their own payout —
	// both change deliberate, test-encoded behaviour and need a product decision.
	MaxProposalDurationHours = 2160
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
// Contract-Level Keys
// -----------------------------------------------------------------------------

const (
	// ContractConfigKey stores the contract configuration (owner, permissions).
	ContractConfigKey = "contract:cfg"
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
	// kContractConfig stores the contract-level configuration (owner, permissions).
	kContractConfig byte = 0x00
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
	// ApproveOptionIndex is the "yes"/approve slot of the default [no, yes] ballot.
	// Only actionable (non-poll) proposals — which always use that default ballot —
	// may execute their outcome, and only when this option wins.
	ApproveOptionIndex = 1

	ProposalActive    ProposalState = 1
	ProposalClosed    ProposalState = 2
	ProposalPassed    ProposalState = 3
	ProposalExecuted  ProposalState = 4
	ProposalFailed    ProposalState = 5
	ProposalCancelled ProposalState = 6
)
