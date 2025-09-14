package main

import (
	"encoding/json"
	"fmt"
	"okinoko_dao/sdk"
	"strings"
)

// -----------------------------------------------------------------------------
// Create Proposal (returns full proposal as JSON)
// Exports only primitive params; slices come in as JSON strings
// -----------------------------------------------------------------------------
//

// Proposal - stored separately at proposal:<id>
type Proposal struct {
	ID              int64             `json:"id"`
	ProjectID       int64             `json:"project_id"`
	Creator         sdk.Address       `json:"creator"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	JsonMetadata    map[string]string `json:"meta,omitempty"`
	Type            VotingType        `json:"type"`
	Options         []string          `json:"options"` // for polls
	Receiver        sdk.Address       `json:"receiver,omitempty"`
	Amount          float64           `json:"amount,omitempty"`
	CreatedAt       int64             `json:"created_at"`
	DurationSeconds int64             `json:"duration_seconds"` // if 0 => use project default
	State           ProposalState     `json:"state"`
	Passed          bool              `json:"passed"`
	Executed        bool              `json:"executed"`
	FinalizedAt     int64             `json:"finalized_at,omitempty"`
	PassTimestamp   int64             `json:"pass_timestamp,omitempty"`
	SnapshotTotal   float64           `json:"snapshot_total,omitempty"` // total voting power snapshot if enabled
	TxID            string            `json:"tx_id,omitempty"`
}

// Proposal state lifecycle
type ProposalState string

const (
	StateActive     ProposalState = "active"     // default state for new proposals
	StateExecutable ProposalState = "executable" // quorum is reached - final
	StateExecuted   ProposalState = "executed"   // proposal passed
	StateFailed     ProposalState = "failed"     // proposal failed to gather enough votes within the proposal duration
)

// Proposal types
type VotingType string

const (
	VotingTypeBoolVote     VotingType = "bool_vote"     // proposals with boolean vote - these can also execute transfers
	VotingTypeSingleChoice VotingType = "single_choice" // proposals with only one answer as vote
	VotingTypeMultiChoice  VotingType = "multi_choice"  // proposals with multiple possible answers as vote
	VotingTypeMetaProposal VotingType = "meta"          // meta proposals to change project settings
)

type VoteRecord struct {
	ID          int64   `json:"id"`
	ProposalID  int64   `json:"proposal_id"`
	Voter       string  `json:"voter"`
	ChoiceIndex []int   `json:"choice_index"` // indexes for options; for yes/no -> [0] or [1]
	Weight      float64 `json:"weight"`
	VotedAt     int64   `json:"voted_at"`
}

type CreateProposalArgs struct {
	ProjectID        int64             `json:"prjId"`
	Name             string            `json:"name"`
	Description      string            `json:"desc"`
	JsonMetadata     map[string]string `json:"meta,omitempty"`
	VotingTypeString string            `json:"project_id"` // VotingType as string (e.g., "bool_vote", "single_choice", ...)
	OptionsJSON      string            `json:"options"`    // JSON array of strings, e.g. '["opt1","opt2"]'
	Receiver         sdk.Address       `json:"receiver"`
	Amount           float64           `json:"amount"`
}

//go:wasmexport proposals_create
func CreateProposal(payload *string) *string {
	input := FromJSON[CreateProposalArgs](*payload, "CreateProposalArgs")
	caller := getSenderAddress() // TODO: review caller vs. sender in whole contract
	prj := loadProject(input.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}
	// permission checks
	switch prj.Config.ProposalPermission {
	case PermCreatorOnly:
		if caller != prj.Owner {
			sdk.Abort("only project owner can create proposals")
		}
	case PermAnyMember:
		if _, ok := prj.Members[caller]; !ok {
			sdk.Abort("only members can create proposals")
		}
	}

	// Parse VotingType
	vtype := VotingType(input.VotingTypeString)
	switch vtype {
	case VotingTypeBoolVote, VotingTypeSingleChoice, VotingTypeMultiChoice, VotingTypeMetaProposal:
		// vtype is valid
	default:
		sdk.Abort("invalid voting type")
	}

	// Parse options JSON (for polls)
	var options []string
	if strings.TrimSpace(input.OptionsJSON) != "" {
		if err := json.Unmarshal([]byte(input.OptionsJSON), &options); err != nil {
			sdk.Abort("invalid options json")
		}
	}
	// Options validation
	if (vtype == VotingTypeSingleChoice || vtype == VotingTypeMultiChoice) && len(options) == 0 {
		sdk.Abort("poll proposals require options")
	}

	// Charge proposal cost (from caller to contract)
	if prj.Config.ProposalCost > 0 {
		ta := getFirstTransferAllow(sdk.GetEnv().Intents)
		if ta.Token != prj.FundsAsset {
			sdk.Abort("intents token != project asset")
		}
		if ta.Limit < prj.Config.ProposalCost {
			sdk.Abort("intents limit < proposal costs")
		}
		mProposalCost := int64(prj.Config.ProposalCost * 1000)
		sdk.HiveDraw(mProposalCost, prj.FundsAsset)
		prj.Funds += prj.Config.ProposalCost
		saveProject(prj)
	}

	// Create proposal
	id := getCount(ProposalsCount)
	now := nowUnix()
	duration := prj.Config.ProposalDurationSecs
	if duration <= 0 {
		duration = 60 * 60 * 24 * 7 // default 7 days
	}
	prpsl := &Proposal{
		ID:              id,
		ProjectID:       input.ProjectID,
		Creator:         getSenderAddress(),
		Name:            input.Name,
		Description:     input.Description,
		JsonMetadata:    input.JsonMetadata,
		Type:            vtype,
		Options:         options,
		Receiver:        input.Receiver,
		Amount:          input.Amount,
		CreatedAt:       now,
		DurationSeconds: duration,
		State:           StateActive,
		Passed:          false,
		Executed:        false,
		TxID:            getTxID(),
	}
	saveProposal(prpsl)
	AddIDToIndex(idxProjectProposalsOpen+string(input.ProjectID), id)
	setCount(ProposalsCount, id+1)
	return nil

}

// -----------------------------------------------------------------------------
// Vote Proposal (choices as JSON array) -> returns {"success":true} or {"error":...}
// -----------------------------------------------------------------------------
//

type VoteProposalArgs struct {
	ProjectID  int64  `json:"prjId"`
	ProposalID int64  `json:"propId"`
	Choice     string `json:"choice"`
}

//go:wasmexport proposals_vote
func VoteProposal(payload *string) *string {
	input := FromJSON[VoteProposalArgs](*payload, "VoteProposalArgs")
	now := nowUnix()

	prj := loadProject(input.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}
	prpsl := loadProposal(input.ProposalID)
	if prpsl.State != StateActive {
		sdk.Abort("proposal inactive")
	}

	// Time window
	if prpsl.DurationSeconds > 0 && now > prpsl.CreatedAt+prpsl.DurationSeconds {
		// finalize instead
		_ = TallyProposal(prpsl.ID) // ignore result here
		sdk.Abort("voting period ended (tallied)")
	}

	sender := getSenderAddress()
	// Member check
	member, ok := prj.Members[sender]
	if !ok {
		sdk.Abort("only members may vote")
	}

	// Compute weight
	weight := float64(1)
	if prj.Config.VotingSystem == SystemStake {
		weight = member.Stake
		if weight <= 0 { // should never happen ut good to check
			sdk.Abort("member stake zero")
		}
	}

	// Parse choices
	var choices []int
	if err := json.Unmarshal([]byte(input.Choices), &choices); err != nil {
		sdk.Abort("invalid choice json")
	}

	// Validate choices by type
	switch prpsl.Type {
	case VotingTypeBoolVote:
		if len(choices) != 1 || (choices[0] != 0 && choices[0] != 1) {
			sdk.Abort("bool_vote requires single choice 0 or 1")
		}
	case VotingTypeSingleChoice:
		if len(choices) != 1 {
			sdk.Abort("single_choice requires exactly 1 index")
		}
		if choices[0] < 0 || choices[0] >= len(prpsl.Options) {
			sdk.Abort("option index out of range")
		}
	case VotingTypeMultiChoice:
		if len(choices) == 0 {
			sdk.Abort("multi_choice requires >=1 choices")
		}
		for _, idx := range choices {
			if idx < 0 || idx >= len(prpsl.Options) {
				sdk.Abort("option index out of range")
			}
		}
	case VotingTypeMetaProposal:
		// no extra validation here
	default:
		sdk.Abort("unknown proposal type")
	}
	vote := VoteRecord{
		ProjectID:   input.ProjectID,
		ProposalID:  input.ProposalID,
		Voter:       sender.String(),
		ChoiceIndex: choices,
		Weight:      weight,
		VotedAt:     now,
	}
	saveVote(&vote)
	return strptr("vote casted")
}

// -----------------------------------------------------------------------------
// Tally Proposal -> returns {"success":true,"passed":bool,"proposal":{...}}
// -----------------------------------------------------------------------------
//
//go:wasmexport proposals_tally
func TallyProposal(proposalID int64) *string {

	prpsl := loadProposal(proposalID)
	prj := loadProject(prpsl.ProjectID)

	// Use proposal-specific duration if set, else project default
	duration := prpsl.DurationSeconds
	if duration <= 0 {
		duration = prj.Config.ProposalDurationSecs
	}
	// Only tally if proposal duration has passed
	if nowUnix() < prpsl.CreatedAt+duration {
		sdk.Abort("proposal duration not over yet")
	}
	// compute total possible voting power
	var totalPossible float64 = 0
	if prj.Config.VotingSystem == SystemDemocratic {
		totalPossible = float64(len(prj.Members))
	} else {
		for _, m := range prj.Members {
			totalPossible += m.Stake
		}
	}

	// gather votes
	votes := loadVotesForProposal(prpsl.ProjectID, *proposalID)
	// compute participation and option counts
	var participation int64 = 0
	optionCounts := make(map[int]int64)
	for _, v := range votes {
		participation += v.Weight
		for _, idx := range v.ChoiceIndex {
			optionCounts[idx] += v.Weight
		}
	}

	// check quorum
	required := int64(0)
	if prj.Config.QuorumPercent > 0 {
		required = (int64(prj.Config.QuorumPercent)*totalPossible + 99) / 100 // ceil
	}
	if required > 0 && participation < required {
		prpsl.Passed = false
		prpsl.State = StateFailed
		prpsl.FinalizedAt = nowUnix()
		saveProposal(prpsl)
		// sdk.Log("TallyProposal: quorum not reached")
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "quorum not reached",
			},
		)
	}

	// evaluate result by type
	switch prpsl.Type {
	case VotingTypeBoolVote:
		yes := optionCounts[1]
		if yes*100 >= int64(prj.Config.ThresholdPercent)*totalPossible {
			prpsl.Passed = true
			prpsl.State = StateExecutable
			prpsl.PassTimestamp = nowUnix()
		} else {
			prpsl.Passed = false
			prpsl.State = StateFailed
		}
	case VotingTypeSingleChoice, VotingTypeMultiChoice, VotingTypeMetaProposal:
		// find best option weight
		bestIdx := -1
		var bestVal int64 = 0
		for idx, cnt := range optionCounts {
			if cnt > bestVal {
				bestVal = cnt
				bestIdx = idx
			}
		}
		if bestIdx >= 0 && bestVal*100 >= int64(prj.Config.ThresholdPercent)*totalPossible {
			prpsl.Passed = true
			prpsl.State = StateExecutable
			prpsl.PassTimestamp = nowUnix()
		} else {
			prpsl.Passed = false
			prpsl.State = StateFailed
		}
	default:
		prpsl.Passed = false
		prpsl.State = StateFailed
	}

	prpsl.FinalizedAt = nowUnix()
	saveProposal(prpsl)
	return strptr("propsal passed")

}

// -----------------------------------------------------------------------------
// Execute Proposal -> returns JSON with action details or error
// (keeps your original flow; only return type changed)
// -----------------------------------------------------------------------------
//
//go:wasmexport proposals_execute
func ExecuteProposal(projectID, proposalID, asset string) *string {
	sender := getSenderAddress()
	prj := loadProject(projectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}
	prpsl := loadProposal(proposalID)
	if prpsl.State != StateExecutable {
		sdk.Abort("proposal not executable")

	}
	// check execution delay
	if prj.Config.ExecutionDelaySecs > 0 && nowUnix() < prpsl.PassTimestamp+prj.Config.ExecutionDelaySecs {
		sdk.Abort("execution delay not passed")
	}
	// check permission to execute
	if prj.Config.ExecutePermission == PermCreatorOnly && sender != prpsl.Creator {
		sdk.Abort("only creator (" + prpsl.Creator.String() + ") can execute")

	}
	if prj.Config.ExecutePermission == PermAnyMember {
		if _, ok := prj.Members[sender]; !ok {
			sdk.Abort("only members can execute")
		}
	}

	// For transfers: only yes/no allowed
	if prpsl.Type == VotingTypeBoolVote && prpsl.Amount > 0 && prpsl.Receiver.String() != "" {
		// ensure funds
		if prj.Funds < prpsl.Amount {
			sdk.Abort(fmt.Sprintf("insufficient project funds: %d (asked %d) %s", prj.Funds, prpsl.Amount, prj.FundsAsset))

		}
		// transfer from contract to receiver
		mAmount := int64(prpsl.Amount * 1000)
		prj.Funds -= prpsl.Amount
		sdk.HiveTransfer(sdk.Address(prpsl.Receiver), mAmount, sdk.Asset(asset))

		prpsl.Executed = true
		prpsl.State = StateExecuted
		prpsl.FinalizedAt = nowUnix()
		saveProposal(prpsl)
		saveProject(prj)

		return strptr("ok")

	}

	// Meta proposals: interpret json_metadata to perform allowed changes
	if prpsl.Type == VotingTypeMetaProposal {
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(prpsl.JsonMetadata), &meta); err != nil {
			sdk.Abort("failed to unmarshal proposal metadata")
		}
		action, _ := meta["action"].(string)
		switch action {
		// TODO: add more project properties here
		case "update_threshold":
			if v, ok := meta["value"].(float64); ok {
				newv := int(v)
				if newv < 0 || newv > 100 {
					sdk.Abort("update_threshold; value out of range")

				}
				prj.Config.ThresholdPercent = newv
				prpsl.Executed = true
				prpsl.State = StateExecuted
				prpsl.FinalizedAt = nowUnix()
				saveProject(prj)
				saveProposal(prpsl)
				// sdk.Log("ExecuteProposal: updated threshold")
				return strptr(fmt.Sprintf("theshold updated to %d", newv))

			}
		case "toggle_pause":
			if val, ok := meta["value"].(bool); ok {
				prj.Paused = val
			} else {
				// flip
				prj.Paused = !prj.Paused
			}
			prpsl.Executed = true
			prpsl.State = StateExecuted
			prpsl.FinalizedAt = nowUnix()
			saveProject(prj)
			saveProposal(prpsl)
			// sdk.Log("ExecuteProposal: toggled pause")
			return strptr(fmt.Sprintf("pause: %s", prj.Paused))
			// TODO: add more meta actions here (update quorum, proposal cost, reward setting, etc.)

		}
	}

	// If nothing to execute, mark executed
	prpsl.Executed = true
	prpsl.State = StateExecuted
	prpsl.FinalizedAt = nowUnix()
	saveProposal(prpsl)
	// sdk.Log("ExecuteProposal: marked executed without transfer " + proposalID)
	// TODO: add field in proposal to show the result...
	return strptr("executed without meta change or transfer")

}

// -----------------------------------------------------------------------------
// Get single proposal -> returns {"success":true,"proposal":{...}}
// -----------------------------------------------------------------------------
//
//go:wasmexport proposals_get_one
func GetProposal(proposalID *string) *string {
	prpsl := loadProposal(*proposalID)
	prpslString := ToJSON(prpsl, "proposal")
	return strptr(prpslString)
}

// -----------------------------------------------------------------------------
// Get all proposals for a project -> returns {"success":true,"proposals":[...]}
// -----------------------------------------------------------------------------
//
//go:wasmexport proposals_get_all
func GetProjectProposals(projectID *string) *string {
	// load open propsals
	proposals := make([]Proposal, 0)
	idsOpen := GetIDsFromIndex(idxProjectProposalsOpen + *projectID)
	idsClosed := GetIDsFromIndex(idxProjectProposalsClosed + *projectID)
	allIDs := append(idsOpen, idsClosed...)
	for _, id := range allIDs {
		prpsl := loadProposal(id)
		proposals = append(proposals, *prpsl)
	}
	proposalsJSON := ToJSON(proposals, "proposals")
	return strptr(proposalsJSON)
}

func saveProposal(prpsl *Proposal) {
	key := proposalKey(prpsl.ID)
	b := ToJSON(prpsl, "proposal")
	sdk.StateSetObject(key, b)
}

func loadProposal(id int64) *Proposal {
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

func saveVote(vote *VoteRecord) {
	// save vote itself
	key := voteKey(vote.ProposalID, vote.Voter)
	b := ToJSON(vote, "vote")
	sdk.StateSetObject(key, b)

	// add vote to index
	AddIDToIndex(idxProposalVotes+vote.ProposalID, vote.Voter)

	ptr := sdk.StateGetObject(votersKey)
	var voters []string
	if ptr != nil {
		json.Unmarshal([]byte(*ptr), &voters)
	}
	seen := false
	for _, a := range voters {
		if a == vote.Voter {
			seen = true
			break
		}
	}
	if !seen {
		voters = append(voters, vote.Voter)
		nb, _ := json.Marshal(voters)
		sdk.StateSetObject(votersKey, string(nb))
	}
}

func loadVotesForProposal(proposalID string) []VoteRecord {
	voteList := GetIDsFromIndex(idxProposalVotes + proposalID)

	if voteList == nil || len(voteList) == 0 {
		return []VoteRecord{}
	}

	out := make([]VoteRecord, 0, len(voteList))
	for _, v := range voteList {
		vk := voteKey(proposalID, v)
		vp := sdk.StateGetObject(vk)
		if vp != nil || *vp != "" {
			vr := FromJSON[VoteRecord](*vp, "vote record")
			out = append(out, *vr)
		}
	}
	return out
}
