package dao

import "math"

const AmountScale = 1000

type Amount int64

// FloatToAmount scales human floats by AmountScale and rounds to int64 so storage stays precise.
// Example payload: FloatToAmount(1.234)
func FloatToAmount(v float64) Amount {
	return Amount(math.Round(v * AmountScale))
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

// VotingSystem defines the vote weighting model for a project.
type VotingSystem uint8

const (
	VotingSystemUnspecified VotingSystem = 0
	VotingSystemDemocratic  VotingSystem = 1
	VotingSystemStake       VotingSystem = 2
)

// String serializes the VotingSystem enum into the short log-friendly codes.
// Example payload: dao.VotingSystemStake.String()
func (vs VotingSystem) String() string {
	switch vs {
	case VotingSystemDemocratic:
		return "0"
	case VotingSystemStake:
		return "1"
	default:
		return "2"
	}
}

// ProposalState captures a proposal's lifecycle.
type ProposalState uint8

const (
	ProposalStateUnspecified ProposalState = 0
	ProposalActive           ProposalState = 1
	ProposalClosed           ProposalState = 2
	ProposalPassed           ProposalState = 3
	ProposalExecuted         ProposalState = 4
	ProposalFailed           ProposalState = 5
	ProposalCancelled        ProposalState = 6
)

// String prints the proposal state as lower-case text for events and logs.
// Example payload: dao.ProposalPassed.String()
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
}

type Member struct {
	Address       Address
	Stake         Amount
	JoinedAt      int64
	LastActionAt  int64
	ExitRequested int64
	Reputation    int64
}

type Project struct {
	ID          uint64
	Owner       Address
	Name        string
	Description string
	Config      ProjectConfig
	Funds       Amount
	FundsAsset  Asset
	Paused      bool
	Tx          string
	Metadata    string
	StakeTotal  Amount
	MemberCount uint64
}

// ProjectMeta stores immutable/general metadata for a project.
type ProjectMeta struct {
	Owner       Address
	Name        string
	Description string
	Paused      bool
	Tx          string
	Metadata    string
}

// ProjectFinance keeps track of treasury and aggregate staking data.
type ProjectFinance struct {
	Funds       Amount
	FundsAsset  Asset
	StakeTotal  Amount
	MemberCount uint64
}

type ProposalOption struct {
	Text        string
	WeightTotal Amount
	VoterCount  uint64
}

type ProposalOutcome struct {
	Meta   map[string]string
	Payout map[Address]Amount
}

type Proposal struct {
	ID                  uint64
	ProjectID           uint64
	Creator             Address
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
	IsPoll              bool
	ResultOptionID      int32
	OptionCount         uint32
	ExecutableAt        int64
}

type CreateProjectArgs struct {
	Name          string
	ProjectConfig ProjectConfig
	Description   string
	Metadata      string
}

type CreateProposalArgs struct {
	ProjectID        uint64
	Name             string
	Description      string
	OptionsList      []string
	ProposalOutcome  *ProposalOutcome
	ProposalDuration uint64
	Metadata         string
	ForcePoll        bool
}

type VoteProposalArgs struct {
	ProposalID uint64
	Choices    []uint
}

type AddFundsArgs struct {
	ProjectID uint64
	ToStake   bool
}

// AddressFromString converts a human string to the platform-specific address wrapper.
// Example payload: AddressFromString("hive:alice")
func AddressFromString(s string) Address { return newAddress(s) }

// AddressToString turns the wrapped type back into the underlying string.
// Example payload: AddressToString(AddressFromString("hive:bob"))
func AddressToString(a Address) string { return addressString(a) }

// AssetFromString wraps a ticker string so type checking keeps us honest.
// Example payload: AssetFromString("hive")
func AssetFromString(s string) Asset { return newAsset(s) }

// AssetToString unwraps the ticker string for logs or SDK calls.
// Example payload: AssetToString(AssetFromString("hbd"))
func AssetToString(a Asset) string { return assetString(a) }
