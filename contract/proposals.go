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
	ID          uint64            `json:"id"`
	ProjectID   uint64            `json:"project_id"`
	Creator     sdk.Address       `json:"creator"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Options     []ProposalOption  `json:"options"`
	Type        string            `json:"type"` // "fund" or "meta"
	Receiver    sdk.Address       `json:"receiver,omitempty"`
	Amount      float64           `json:"amount,omitempty"`
	CreatedAt   int64             `json:"created_at"`
	Duration    int64             `json:"duration"`
	State       string            `json:"state"`
	Passed      bool              `json:"passed"`
	Executed    bool              `json:"executed"`
	Meta        map[string]string `json:"meta,omitempty"`
	Tx          string            `json:"tx"`
}

type ProposalOption struct {
	Text  string  `json:"text"`
	Votes float64 `json:"votes"`
}

// -----------------------------------------------------------------------------
// Create Proposal
// -----------------------------------------------------------------------------
type CreateProposalArgs struct {
	ProjectID    uint64            `json:"project_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	OptionsJSON  string            `json:"options"`
	Type         string            `json:"type"` // "fund" or "meta"
	Receiver     sdk.Address       `json:"receiver"`
	Amount       float64           `json:"amount"`
	JsonMetadata map[string]string `json:"meta,omitempty"`
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
	now := time.Now().Unix()
	duration := prj.Config.ProposalDurationSecs
	if duration <= 0 {
		duration = 7 * 24 * 3600 // default 1 week
	}

	prpsl := &Proposal{
		ID:          id,
		ProjectID:   input.ProjectID,
		Creator:     caller,
		Name:        input.Name,
		Description: input.Description,
		Options:     opts,
		Type:        input.Type,
		Receiver:    input.Receiver,
		Amount:      input.Amount,
		CreatedAt:   now,
		Duration:    duration,
		State:       "active",
		Passed:      false,
		Executed:    false,
		Meta:        input.JsonMetadata,
		Tx:          *sdk.GetEnvKey("tx.id"),
	}

	saveProposal(prpsl)
	setCount(ProposalsCount, id+1)
	emitProposalCreatedEvent(id, caller.String())
	return strptr(fmt.Sprintf("proposal %d created", id))
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
	if prpsl.State != "executable" {
		sdk.Abort("proposal not executable")
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
			var v int
			_ = json.Unmarshal([]byte(val), &v)
			prj.Config.ThresholdPercent = v
		case "toggle_pause":
			prj.Paused = !prj.Paused
		}
		saveProject(prj)
	}

	prpsl.Executed = true
	prpsl.State = "executed"
	saveProposal(prpsl)
	emitProposalExecutedEvent(prpsl.ID)
	return strptr("executed")
}

// -----------------------------------------------------------------------------
// Tally
// -----------------------------------------------------------------------------
//
//go:wasmexport proposal_tally
func TallyProposal(proposalId *uint64) *string {
	prpsl := loadProposal(*proposalId)
	if prpsl.State != "active" {
		sdk.Abort("proposal not active")
	}
	if time.Now().Unix() < prpsl.CreatedAt+prpsl.Duration {
		sdk.Abort("proposal still running")
	}

	// find best option
	best := -1
	var bestVal float64
	for i, opt := range prpsl.Options {
		if opt.Votes > bestVal {
			bestVal = opt.Votes
			best = i
		}
	}

	if best >= 0 && bestVal > 0 {
		prpsl.Passed = true
		prpsl.State = "executable"
	} else {
		prpsl.Passed = false
		prpsl.State = "failed"
	}

	saveProposal(prpsl)
	return strptr("tallied")
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
