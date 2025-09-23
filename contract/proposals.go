package main

import (
	"encoding/json"
	"fmt"
	"math"
	"okinoko_dao/sdk"
	"time"
)

// -----------------------------------------------------------------------------
// Proposal struct
// -----------------------------------------------------------------------------
type Proposal struct {
	ID                  uint64                  `json:"id"`
	ProjectID           uint64                  `json:"project_id"`
	Creator             sdk.Address             `json:"creator"`
	Name                string                  `json:"name"`
	Description         string                  `json:"description"`
	Options             []ProposalOption        `json:"options"`
	DurationHours       uint64                  `json:"duration"`
	CreatedAt           int64                   `json:"created_at"`
	State               ProposalState           `json:"state"`
	Meta                map[string]string       `json:"meta,omitempty"`
	Payout              map[sdk.Address]float64 `json:"payout,omitempty"`
	Tx                  string                  `json:"tx"`
	StakeSnapshot       float64                 `json:"snapshot"`
	MemberCountSnapshot uint                    `json:"memberSnapshot"`
}

type ProposalState string

const (
	ProposalActive   ProposalState = "active"
	ProposalExecuted ProposalState = "executed"
	ProposalPassed   ProposalState = "passed"
	ProposalFailed   ProposalState = "failed"
)

type ProposalOption struct {
	Text  string    `json:"text"`
	Votes []float64 `json:"votes"`
}

// -----------------------------------------------------------------------------
// Create Proposal
// -----------------------------------------------------------------------------
type CreateProposalArgs struct {
	ProjectID        uint64                  `json:"project_id"`
	Name             string                  `json:"name"`
	Description      string                  `json:"description"`
	OptionsList      []string                `json:"options"` // only fill for polls - keep empty for payout or meta proposals
	Meta             map[string]string       `json:"meta,omitempty"`
	Payout           map[sdk.Address]float64 `json:"payout,omitempty"`
	ProposalDuration uint64                  `json:"duration"`
}

//go:wasmexport proposal_create
func CreateProposal(payload *string) *string {
	input := FromJSON[CreateProposalArgs](*payload, "CreateProposalArgs")
	// TODO: validate input.Payout & input.Meta
	caller := getSenderAddress()
	prj := loadProject(input.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}

	// Only members can create
	if _, ok := prj.Members[caller]; !ok {
		sdk.Abort("only members can create proposals")
	}

	// default to no/yes
	if len(input.OptionsList) == 0 {
		input.OptionsList = []string{"no", "yes"}
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
		Options:             opts,
		Payout:              input.Payout,
		CreatedAt:           now,
		DurationHours:       duration,
		State:               ProposalActive,
		Meta:                input.Meta,
		Tx:                  *sdk.GetEnvKey("tx.id"),
		MemberCountSnapshot: memberSnap,
		StakeSnapshot:       stakeSnap,
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
		// Check quorum
		// calculate quorum threshold (round up)
		quorumThreshold := uint64(math.Ceil(float64(prpsl.MemberCountSnapshot) * (prj.Config.QuorumPercent / 100)))

		quorumMet := voterCount >= quorumThreshold

		// Check threshold (fraction of total stake at creation)
		thresholdMet := (highestOptionValue / prpsl.StakeSnapshot) >= prj.Config.ThresholdPercent/100

		if quorumMet && thresholdMet {
			prpsl.State = ProposalPassed
		}
	}
	saveProposal(prpsl)
	emitProposalStateChangedEvent(prpsl.ID, prpsl.State)
	return strptr("tallied")
}

// -----------------------------------------------------------------------------
// Execute
// -----------------------------------------------------------------------------
//
//go:wasmexport proposal_execute
func ExecuteProposal(proposalID *uint64) *string {
	prpsl := loadProposal(*proposalID)
	prj := loadProject(prpsl.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}
	// TODO add abort if not talllied yet
	if prpsl.State != ProposalPassed {
		sdk.Abort("proposal not passed")
	}

	fundsTransferred := false
	metaChanged := false
	// fund transfer
	totalAsked := float64(0)
	for _, stake := range prpsl.Payout {
		totalAsked += stake
	}
	if prj.Funds < totalAsked {
		sdk.Abort("insufficient funds")
	}
	for addr, amount := range prpsl.Payout {
		mAmount := int64(amount * 1000)
		prj.Funds -= amount
		sdk.HiveTransfer(addr, mAmount, sdk.Asset(prj.FundsAsset))
		emitFundsRemoved(prj.ID, addr.String(), amount, prj.FundsAsset.String(), false)
		fundsTransferred = true

	}
	// meta change
	for action, value := range prpsl.Meta {
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
func saveProposal(prpsl *Proposal) {
	key := proposalKey(prpsl.ID)
	b := ToJSON(prpsl, "proposal")
	sdk.StateSetObject(key, b)
}

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
