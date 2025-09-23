package main

import (
	"encoding/json"
	"fmt"
	"okinoko_dao/sdk"
	"strings"
	"time"
)

// -----------------------------------------------------------------------------
// Proposal struct
// -----------------------------------------------------------------------------
type Proposal struct {
	ID             uint64            `json:"id"`
	ProjectID      uint64            `json:"project_id"`
	Creator        sdk.Address       `json:"creator"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Options        []ProposalOption  `json:"options"`
	Type           string            `json:"type"` // "fund" or "meta"
	Receiver       sdk.Address       `json:"receiver,omitempty"`
	Amount         float64           `json:"amount,omitempty"`
	DurationHours  uint64            `json:"duration"`
	CreatedAt      int64             `json:"created_at"`
	State          ProposalState     `json:"state"`
	Meta           map[string]string `json:"meta,omitempty"`
	Tx             string            `json:"tx"`
	StakeSnapshot  float64           `json:"snapshot"`
	MemberSnapshot uint              `json:"memberSnapshot"`
}

type ProposalState string

const (
	ProposalActive   ProposalState = "active"
	ProposalExecuted ProposalState = "executed"
	ProposalPassed   ProposalState = "passed"
	ProposalFailed   ProposalState = "failed"
)

type ProposalOption struct {
	Text  string  `json:"text"`
	Votes float64 `json:"votes"`
}

// -----------------------------------------------------------------------------
// Create Proposal
// -----------------------------------------------------------------------------
type CreateProposalArgs struct {
	ProjectID        uint64            `json:"project_id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	OptionsJSON      string            `json:"options"`
	Type             string            `json:"type"` // "fund" or "meta"
	Receiver         sdk.Address       `json:"receiver"`
	Amount           float64           `json:"amount"`
	JsonMetadata     map[string]string `json:"meta,omitempty"`
	ProposalDuration uint64            `json:"duration"`
}

//go:wasmexport proposal_create
func CreateProposal(payload *string) *string {
	input := FromJSON[CreateProposalArgs](*payload, "CreateProposalArgs")
	caller := getSenderAddress()
	prj := loadProject(input.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}

	// Only members can create
	if _, ok := prj.Members[caller]; !ok {
		sdk.Abort("only members can create proposals")
	}

	var options []string
	if strings.TrimSpace(input.OptionsJSON) != "" {
		if err := json.Unmarshal([]byte(input.OptionsJSON), &options); err != nil {
			sdk.Abort("invalid options json")
		}
	}
	if len(options) == 0 {
		sdk.Abort("must provide options")
	}

	opts := make([]ProposalOption, len(options))
	for i, txt := range options {
		opts[i] = ProposalOption{Text: txt, Votes: 0}
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
		ID:             id,
		ProjectID:      input.ProjectID,
		Creator:        caller,
		Name:           input.Name,
		Description:    input.Description,
		Options:        opts,
		Type:           input.Type,
		Receiver:       input.Receiver,
		Amount:         input.Amount,
		CreatedAt:      now,
		DurationHours:  duration,
		State:          ProposalActive,
		Meta:           input.JsonMetadata,
		Tx:             *sdk.GetEnvKey("tx.id"),
		MemberSnapshot: memberSnap,
		StakeSnapshot:  stakeSnap,
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
	best := -1
	var bestVal float64
	var totalVotes float64
	var voterCount uint64

	for i, opt := range prpsl.Options {
		totalVotes += opt.Votes
		if opt.Votes > 0 {
			voterCount++ // crude member count, adjust if multiple votes per member possible
		}
		if opt.Votes > bestVal {
			bestVal = opt.Votes
			best = i
		}
	}

	// default to failed
	prpsl.State = ProposalFailed

	if best >= 0 && bestVal > 0 {
		// Check quorum
		quorumMet := voterCount >= prj.Config.Quorum // or prpsl.MemberSnapshot * quorumPercent, depending on design

		// Check threshold (fraction of total stake at creation)
		thresholdMet := (bestVal / prpsl.StakeSnapshot) >= prj.Config.ThresholdPercent

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

	// fund transfer
	if prpsl.Type == "fund" && prpsl.Amount > 0 && prpsl.Receiver.String() != "" {
		if prj.Funds < prpsl.Amount {
			sdk.Abort("insufficient funds")
		}
		mAmount := int64(prpsl.Amount * 1000)
		prj.Funds -= prpsl.Amount
		sdk.HiveTransfer(prpsl.Receiver, mAmount, sdk.Asset(prj.FundsAsset))
		emitFundsRemoved(prj.ID, prpsl.Receiver.String(), prpsl.Amount, prj.FundsAsset.String(), false)
	}

	// meta change
	if prpsl.Type == "meta" {
		action := prpsl.Meta["action"]
		switch action {
		// todo: add more
		case "update_threshold":
			val := prpsl.Meta["value"]
			var v float64
			_ = json.Unmarshal([]byte(val), &v)
			prj.Config.ThresholdPercent = v
		case "toggle_pause":
			prj.Paused = !prj.Paused
		}
		saveProject(prj)
	}

	prpsl.State = ProposalExecuted
	saveProposal(prpsl)
	emitProposalStateChangedEvent(prpsl.ID, prpsl.State)
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
