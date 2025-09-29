package main

import (
	"encoding/json"
	"fmt"
	"math"
	"okinoko_dao/sdk"
	"time"
)

// -----------------------------------------------------------------------------
// Proposal-related types
// -----------------------------------------------------------------------------

// Proposal represents a proposal created within a project.
// Proposals can be polls, payouts, or metadata changes.
type Proposal struct {
	ID                  uint64           `json:"id"`
	ProjectID           uint64           `json:"projectId"`
	Creator             sdk.Address      `json:"creator"`
	Name                string           `json:"name"`
	Description         string           `json:"description"`
	Options             []ProposalOption `json:"options"`
	DurationHours       uint64           `json:"duration"`
	CreatedAt           int64            `json:"createdAt"`
	State               ProposalState    `json:"state"`
	ProposalOutcome     *ProposalOutcome `json:"outcome"`
	Tx                  string           `json:"tx"`
	StakeSnapshot       float64          `json:"snapshot"`
	MemberCountSnapshot uint             `json:"memberSnapshot"`
	JsonMetadata        map[string]any   `json:"jsonMeta,omitempty"` // TODO: add max & check length
	IsPoll              bool             `json:"IsPoll"`
	ResultOptionId      int              `json:"ResultOptionId"`
}

// ProposalOutcome defines the result of a proposal, including
// metadata updates and payout distributions.
type ProposalOutcome struct {
	Meta   map[string]string       `json:"meta,omitempty"`
	Payout map[sdk.Address]float64 `json:"payout,omitempty"`
}

// ProposalState defines the lifecycle state of a proposal.
type ProposalState string

const (
	// ProposalActive indicates the proposal is still open for voting.
	ProposalActive ProposalState = "active"

	// ProposalClosed means the proposal finished but no transfer/meta change occurred.
	ProposalClosed ProposalState = "closed"

	// ProposalPassed means the proposal passed and awaits execution (transfer/meta change).
	ProposalPassed ProposalState = "passed"

	// ProposalExecuted indicates the proposal has been executed successfully.
	ProposalExecuted ProposalState = "executed"

	// ProposalFailed means the proposal did not reach quorum/threshold and failed.
	ProposalFailed ProposalState = "failed"
)

// ProposalOption represents a voting option within a proposal.
type ProposalOption struct {
	Text  string    `json:"text"`
	Votes []float64 `json:"votes"`
}

// -----------------------------------------------------------------------------
// Create Proposal
// -----------------------------------------------------------------------------

// CreateProposalArgs defines the JSON payload for creating a proposal.
//
// Fields:
//   - ProjectID: The project to which the proposal belongs.
//   - Name: Name/title of the proposal.
//   - Description: Human-readable description.
//   - OptionsList: Voting options (for polls). If empty, defaults to ["no","yes"].
//   - ProposalOutcome: Outcome instructions (payout/meta changes).
//   - ProposalDuration: Duration in hours (falls back to project defaults).
//   - JsonMetadata: Optional extensibility metadata.
type CreateProposalArgs struct {
	ProjectID        uint64           `json:"projectId"`
	Name             string           `json:"name"`
	Description      string           `json:"desc"`
	OptionsList      []string         `json:"options"`
	ProposalOutcome  *ProposalOutcome `json:"outcome"`
	ProposalDuration uint64           `json:"duration"`
	JsonMetadata     map[string]any   `json:"jsonMeta,omitempty"`
}

// CreateProposal creates a new proposal within a project.
// Only members may create proposals. If no options are provided,
// defaults to a binary yes/no vote.
//
//go:wasmexport proposal_create
func CreateProposal(payload *string) *string {
	input := FromJSON[CreateProposalArgs](*payload, "CreateProposalArgs")
	// TODO: validate input.Payout & input.Meta
	if input.ProposalOutcome != nil {

	}

	caller := getSenderAddress()
	prj := loadProject(input.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}

	// Only members can create
	if _, ok := prj.Members[caller]; !ok {
		sdk.Abort("only members can create proposals")
	}

	isPoll := true
	// default to no/yes
	if len(input.OptionsList) == 0 {
		input.OptionsList = []string{"no", "yes"}
		isPoll = false
	}

	opts := make([]ProposalOption, len(input.OptionsList))
	for i, txt := range input.OptionsList {
		opts[i] = ProposalOption{Text: txt, Votes: []float64{}}
	}

	id := getCount(ProposalsCount)

	var duration uint64
	if input.ProposalDuration > 0 {
		if input.ProposalDuration < prj.Config.ProposalDurationHours {
			sdk.Abort("Duration must be higher or equal to project defined proposal duration")
		}
		duration = input.ProposalDuration
	} else {
		duration = prj.Config.ProposalDurationHours * 3600
	}

	now := time.Now().Unix()

	// Count members
	memberSnap := uint(len(prj.Members))

	// Sum stakes
	var stakeSnap float64
	for _, m := range prj.Members {
		stakeSnap += m.Stake
	}

	prpsl := &Proposal{
		ID:                  id,
		ProjectID:           input.ProjectID,
		Creator:             caller,
		Name:                input.Name,
		Description:         input.Description,
		JsonMetadata:        input.JsonMetadata,
		Options:             opts,
		ProposalOutcome:     input.ProposalOutcome,
		CreatedAt:           now,
		DurationHours:       duration,
		State:               ProposalActive,
		Tx:                  *sdk.GetEnvKey("tx.id"),
		MemberCountSnapshot: memberSnap,
		StakeSnapshot:       stakeSnap,
		IsPoll:              isPoll,
	}

	saveProposal(prpsl)
	setCount(ProposalsCount, id+1)
	emitProposalCreatedEvent(id, caller.String())
	emitProposalStateChangedEvent(id, ProposalActive)
	return strptr(fmt.Sprintf("proposal %d created", id))
}

// -----------------------------------------------------------------------------
// Tally
// -----------------------------------------------------------------------------

// TallyProposal closes voting on a proposal and determines its result.
// Checks quorum and threshold rules, then marks the proposal as passed,
// closed, or failed.
//
//go:wasmexport proposal_tally
func TallyProposal(proposalId *uint64) *string {
	prpsl := loadProposal(*proposalId)
	prj := loadProject(prpsl.ProjectID)

	if prpsl.State != ProposalActive {
		sdk.Abort("proposal not active")
	}
	if time.Now().Unix() < prpsl.CreatedAt+int64(prpsl.DurationHours)*3600 {
		sdk.Abort("proposal still running")
	}

	// find best option
	var totalVotes float64
	var voterCount uint64
	highestOptionId := -1
	highestOptionValue := float64(0)

	for i, opt := range prpsl.Options {
		var optionSum float64
		for _, v := range opt.Votes {
			optionSum += v
		}

		totalVotes += optionSum              // sum all votes across options
		voterCount += uint64(len(opt.Votes)) // count how many votes were cast for this option

		if optionSum > highestOptionValue {
			highestOptionValue = optionSum
			highestOptionId = i
		}
	}

	// default to failed
	prpsl.State = ProposalFailed

	if highestOptionId >= 0 && highestOptionValue > 0 {
		// calculate quorum threshold (round up)
		quorumThreshold := uint64(math.Ceil(float64(prpsl.MemberCountSnapshot) * (prj.Config.QuorumPercent / 100)))
		// Check quorum
		quorumMet := voterCount >= quorumThreshold
		// Check threshold (fraction of total stake at creation)
		thresholdMet := (highestOptionValue / prpsl.StakeSnapshot) >= prj.Config.ThresholdPercent/100

		if quorumMet && thresholdMet {
			prpsl.ResultOptionId = highestOptionId
			if prpsl.IsPoll && highestOptionId == 1 {
				prpsl.State = ProposalPassed // make it executable
			} else {
				prpsl.State = ProposalClosed // just close
			}
		}
	}
	saveProposal(prpsl)
	emitProposalStateChangedEvent(prpsl.ID, prpsl.State)
	return strptr("tallied")
}

// -----------------------------------------------------------------------------
// Execute
// -----------------------------------------------------------------------------

// ExecuteProposal executes a previously passed proposal.
// It can transfer funds or update project metadata as defined
// in the proposal outcome.
//
//go:wasmexport proposal_execute
func ExecuteProposal(proposalID *uint64) *string {
	prpsl := loadProposal(*proposalID)
	prj := loadProject(prpsl.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}
	if prpsl.State != ProposalPassed {
		sdk.Abort(fmt.Sprintf("proposal is %s", prpsl.State))
	}
	fundsTransferred := false
	metaChanged := false
	if prpsl.ProposalOutcome != nil {
		if prpsl.ProposalOutcome.Payout != nil {
			totalAsked := float64(0)
			for _, stake := range prpsl.ProposalOutcome.Payout {
				totalAsked += stake
			}
			if prj.Funds < totalAsked {
				sdk.Abort("insufficient funds")
			}
			for addr, amount := range prpsl.ProposalOutcome.Payout {
				mAmount := int64(amount * 1000)
				prj.Funds -= amount
				sdk.HiveTransfer(addr, mAmount, sdk.Asset(prj.FundsAsset))
				emitFundsRemoved(prj.ID, addr.String(), amount, prj.FundsAsset.String(), false)
				fundsTransferred = true

			}
		}
		if prpsl.ProposalOutcome.Meta != nil {
			// meta change
			for action, value := range prpsl.ProposalOutcome.Meta {
				switch action {
				// todo: add more
				case "update_threshold":
					var v float64
					_ = json.Unmarshal([]byte(value), &v)
					prj.Config.ThresholdPercent = v
					metaChanged = true
				case "update_quorum":
					var v float64
					_ = json.Unmarshal([]byte(value), &v)
					prj.Config.QuorumPercent = v
					metaChanged = true
				case "update_proposalDuration":
					var v uint64
					_ = json.Unmarshal([]byte(value), &v)
					prj.Config.ProposalDurationHours = v
					metaChanged = true
				case "update_executionDelay":
					var v uint64
					_ = json.Unmarshal([]byte(value), &v)
					prj.Config.ExecutionDelayHours = v
					metaChanged = true
				case "update_leaveCooldown":
					var v uint64
					_ = json.Unmarshal([]byte(value), &v)
					prj.Config.LeaveCooldownHours = v
					metaChanged = true
				case "update_proposalCost":
					var v float64
					_ = json.Unmarshal([]byte(value), &v)
					prj.Config.ProposalCost = v
					metaChanged = true

				case "toggle_pause":
					prj.Paused = !prj.Paused
					metaChanged = true
				}
			}
		}
	}

	prpsl.State = ProposalExecuted
	saveProposal(prpsl)
	saveProject(prj)
	emitProposalStateChangedEvent(prpsl.ID, prpsl.State)
	if metaChanged {
		emitProposalResultEvent(prj.ID, prpsl.ID, "meta changed")
	}
	if fundsTransferred {
		emitProposalResultEvent(prj.ID, prpsl.ID, "funds transferred")
	}
	return strptr("executed")
}

// -----------------------------------------------------------------------------
// Save/Load
// -----------------------------------------------------------------------------

// saveProposal persists a proposal in contract state.
func saveProposal(prpsl *Proposal) {
	key := proposalKey(prpsl.ID)
	b := ToJSON(prpsl, "proposal")
	sdk.StateSetObject(key, b)
}

// loadProposal retrieves a proposal from contract state by ID.
// Aborts if not found or if unmarshalling fails.
func loadProposal(id uint64) *Proposal {
	key := proposalKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("proposal not found")
	}
	var prpsl Proposal
	if err := json.Unmarshal([]byte(*ptr), &prpsl); err != nil {
		sdk.Abort("failed to unmarshal proposal")
	}
	return &prpsl
}
