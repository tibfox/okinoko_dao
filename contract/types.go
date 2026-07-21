package main

import (
	"math"

	"okinoko_dao/sdk"
)

// -----------------------------------------------------------------------------
// Core Types
// -----------------------------------------------------------------------------

type Amount int64

// maxAmount / minAmount are the int64 bounds of a scaled Amount, used by the
// overflow-safe math helpers and by FloatToAmount's range check.
const (
	maxAmount Amount = math.MaxInt64
	minAmount Amount = math.MinInt64
)

// VotingSystem defines the vote weighting model for a project.
type VotingSystem uint8

// ProposalState captures a proposal's lifecycle.
type ProposalState uint8

// ContractConfig stores contract-level settings set during initialization.
type ContractConfig struct {
	Owner                 sdk.Address // Contract owner who initialized the contract
	ProjectCreationPublic bool        // If false, only owner can create projects
}

// FloatToAmount scales human floats by AmountScale and rounds to int64 so storage stays precise.
// It aborts on NaN/Inf or out-of-range values instead of wrapping the int64 into
// a bogus (possibly negative) balance.
// Example payload: FloatToAmount(1.234)
func FloatToAmount(v float64) Amount {
	scaled := math.Round(v * AmountScale)
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
		sdk.Abort("invalid amount")
	}
	// float64(math.MaxInt64) rounds UP to 2^63, which is not a valid int64. Use >=
	// so a scaled value of exactly 2^63 is rejected instead of wrapping negative
	// (native) or trapping (wasm i64.trunc). MinInt64 == -2^63 is exact, so keep <.
	if scaled >= float64(math.MaxInt64) || scaled < float64(math.MinInt64) {
		sdk.Abort("amount out of range")
	}
	return Amount(scaled)
}

// AmountToFloat converts back to float64 for reporting or events.
// Example payload: AmountToFloat(FloatToAmount(2.5))
func AmountToFloat(v Amount) float64 {
	return float64(v) / AmountScale
}

// AmountToInt64 exposes the raw scaled int64 for Hive transfer functions.
// Example payload: AmountToInt64(FloatToAmount(3.14))
func AmountToInt64(v Amount) int64 {
	return int64(v)
}

// String serializes the VotingSystem enum into the short log-friendly codes.
// Example payload: VotingSystemStake.String()
func (vs VotingSystem) String() string {
	switch vs {
	case VotingSystemDemocratic:
		return "0"
	case VotingSystemStake:
		return "1"
	default:
		return "0"
	}
}

// String prints the proposal state as lower-case text for events and logs.
// Example payload: ProposalPassed.String()
func (ps ProposalState) String() string {
	switch ps {
	case ProposalActive:
		return "active"
	case ProposalClosed:
		return "closed"
	case ProposalPassed:
		return "passed"
	case ProposalExecuted:
		return "executed"
	case ProposalFailed:
		return "failed"
	case ProposalCancelled:
		return "cancelled"
	default:
		return "unspecified"
	}
}

type ProjectConfig struct {
	VotingSystem                  VotingSystem
	ThresholdPercent              float64
	QuorumPercent                 float64
	ProposalDurationHours         uint64
	ExecutionDelayHours           uint64
	LeaveCooldownHours            uint64
	ProposalCost                  float64
	StakeMinAmt                   float64
	MembershipNFTContract         *string
	MembershipNFTContractFunction *string
	MembershipNFT                 *uint64
	MembershipNftPayloadFormat    string
	ProposalsMembersOnly          bool
	WhitelistOnly                 bool
}

type Member struct {
	Address        sdk.Address
	Stake          Amount
	JoinedAt       int64
	LastActionAt   int64
	ExitRequested  int64
	Reputation     int64
	StakeIncrement uint64 // Counter incremented on each stake change
	// JoinSeq is this member's position in the project's monotonic join sequence.
	// Vote eligibility compares it against Proposal.JoinSeqSnapshot instead of
	// comparing timestamps: every transaction in a block shares one block
	// timestamp, so JoinedAt cannot distinguish "joined before this proposal" from
	// "joined after it in the same block". The founding member is 0.
	JoinSeq uint64
	// VoteLockUntil is the deadline of the latest still-undecided proposal this
	// member has voted on. They cannot complete a leave before it. See VoteProposal.
	VoteLockUntil int64
}

type Project struct {
	ID          uint64
	Owner       sdk.Address
	Name        string
	Description string
	Config      ProjectConfig
	URL         string
	FundsAsset  sdk.Asset // Main project asset (used for staking)
	Paused      bool
	Tx          string
	Metadata    string
	StakeTotal  Amount
	MemberCount uint64
}

// ProjectMeta stores immutable/general metadata for a project.
type ProjectMeta struct {
	Owner       sdk.Address
	Name        string
	Description string
	Paused      bool
	Tx          string
	Metadata    string
	URL         string
}

// ProjectFinance keeps track of treasury and aggregate staking data.
type ProjectFinance struct {
	FundsAsset  sdk.Asset // Main project asset (used for staking)
	StakeTotal  Amount
	MemberCount uint64
	Treasury    map[sdk.Asset]Amount // Multi-asset treasury balances
}

type ProposalOption struct {
	Text        string
	URL         string
	WeightTotal Amount
	VoterCount  uint64
}

// PayoutEntry represents a single payout with address and asset specification
type PayoutEntry struct {
	Address sdk.Address
	Amount  Amount
	Asset   sdk.Asset
}

// InterContractCall represents a single inter-contract call with asset transfers
type InterContractCall struct {
	ContractAddress string               // Target contract address
	Function        string               // Function/method to call
	Payload         string               // JSON payload string for the function
	Assets          map[sdk.Asset]Amount // Asset transfers to include (e.g., map[HIVE]1000, map[HBD]500)
}

type ProposalOutcome struct {
	Meta   map[string]string
	Payout []PayoutEntry       // Supports multiple payouts per address with different assets
	ICC    []InterContractCall // Inter-contract calls to execute
}

type Proposal struct {
	ID                  uint64
	ProjectID           uint64
	Creator             sdk.Address
	Name                string
	Description         string
	DurationHours       uint64
	CreatedAt           int64
	State               ProposalState
	Outcome             *ProposalOutcome
	Tx                  string
	StakeSnapshot       Amount
	MemberCountSnapshot uint
	Metadata            string
	URL                 string
	IsPoll              bool
	ResultOptionID      int32
	OptionCount         uint32
	ExecutableAt        int64
	VoterCount          uint64 // distinct voters (for quorum; NOT the per-option tally)
	CostPaid            Amount // proposal cost actually charged at creation (refund basis)
	// JoinSeqSnapshot is the project's join counter at creation time. A member may
	// vote only if their JoinSeq is strictly below it, which matches exactly the
	// membership captured by MemberCountSnapshot/StakeSnapshot.
	JoinSeqSnapshot uint64
}

type CreateProjectArgs struct {
	Name          string
	ProjectConfig ProjectConfig
	Description   string
	Metadata      string
	URL           string
}

type ProposalOptionInput struct {
	Text string
	URL  string
}

type CreateProposalArgs struct {
	ProjectID        uint64
	Name             string
	Description      string
	OptionsList      []ProposalOptionInput
	ProposalOutcome  *ProposalOutcome
	ProposalDuration uint64
	Metadata         string
	ForcePoll        bool
	URL              string
}

type VoteProposalArgs struct {
	ProposalID uint64
	Choices    []uint
}

type AddFundsArgs struct {
	ProjectID uint64
	ToStake   bool
}
