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
	ID              string            `json:"id"`
	ProjectID       string            `json:"project_id"`
	Creator         sdk.Address       `json:"creator"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	JsonMetadata    map[string]string `json:"meta,omitempty"`
	Type            VotingType        `json:"type"`
	Options         []string          `json:"options"` // for polls
	Receiver        sdk.Address       `json:"receiver,omitempty"`
	Amount          int64             `json:"amount,omitempty"`
	CreatedAt       int64             `json:"created_at"`
	DurationSeconds int64             `json:"duration_seconds"` // if 0 => use project default
	State           ProposalState     `json:"state"`
	Passed          bool              `json:"passed"`
	Executed        bool              `json:"executed"`
	FinalizedAt     int64             `json:"finalized_at,omitempty"`
	PassTimestamp   int64             `json:"pass_timestamp,omitempty"`
	SnapshotTotal   int64             `json:"snapshot_total,omitempty"` // total voting power snapshot if enabled
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
	ProjectID   string `json:"project_id"`
	ProposalID  string `json:"proposal_id"`
	Voter       string `json:"voter"`
	ChoiceIndex []int  `json:"choice_index"` // indexes for options; for yes/no -> [0] or [1]
	Weight      int64  `json:"weight"`
	VotedAt     int64  `json:"voted_at"`
}

type CreateProposalArgs struct {
	ProjectID        string            `json:"prjId"`
	Name             string            `json:"name"`
	Description      string            `json:"desc"`
	JsonMetadata     map[string]string `json:"meta,omitempty"`
	VotingTypeString string            `json:"project_id"` // VotingType as string (e.g., "bool_vote", "single_choice", ...)
	OptionsJSON      string            `json:"options"`    // JSON array of strings, e.g. '["opt1","opt2"]'
	Receiver         sdk.Address       `json:"receiver"`
	Amount           int64             `json:"amount"`
}

//go:wasmexport proposals_create
func CreateProposal(payload *string) *string {
	input, err := FromJSON[CreateProposalArgs](*payload)
	abortOnError(err, "invalid args")
	caller := getSenderAddress()

	prj := loadProject(input.ProjectID)
	if prj.Paused {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "project is paused",
			},
		)
	}

	// permission checks
	switch prj.Config.ProposalPermission {
	case PermCreatorOnly:
		if caller != prj.Owner {
			abortCustom("only project owner can create proposals")
		}
	case PermAnyMember:
		if _, ok := prj.Members[caller]; !ok {
			abortCustom("only members can create proposals")
		}
	}

	// Parse VotingType
	vtype := VotingType(input.VotingTypeString)
	switch vtype {
	case VotingTypeBoolVote, VotingTypeSingleChoice, VotingTypeMultiChoice, VotingTypeMetaProposal:
	default:
		abortCustom("invalid voting type")
	}

	// Parse options JSON (for polls)
	var options []string
	if strings.TrimSpace(input.OptionsJSON) != "" {
		if err := json.Unmarshal([]byte(input.OptionsJSON), &options); err != nil {
			abortCustom("invalid options json")
		}
	}
	// Options validation
	if (vtype == VotingTypeSingleChoice || vtype == VotingTypeMultiChoice) && len(options) == 0 {
		abortCustom("poll proposals require options")
	}

	// Charge proposal cost (from caller to contract)
	if prj.Config.ProposalCost > 0 {
		ta := getFirstTransferAllow(sdk.GetEnv().Intents)
		if ta.Token != prj.FundsAsset {
			abortCustom("intents token != project asset")
		}
		if ta.Limit < prj.Config.ProposalCost {
			abortCustom("intents limit < proposal costs")
		}
		sdk.HiveDraw(prj.Config.ProposalCost, prj.FundsAsset)
		prj.Funds += prj.Config.ProposalCost
		saveProject(prj)
	}

	// Create proposal
	id := generateGUID() // TODO: make these int
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

	// Snapshot voting power (if enabled)
	if prj.Config.EnableSnapshot {
		var total int64 = 0
		if prj.Config.VotingSystem == SystemDemocratic {
			total = int64(len(prj.Members))
		} else {
			for _, m := range prj.Members {
				total += m.Stake
			}
		}
		prpsl.SnapshotTotal = total
	}

	saveProposal(prpsl)
	AddIDToIndex(idxProjectProposalsOpen+input.ProjectID, id)

	return nil

}

// -----------------------------------------------------------------------------
// Vote Proposal (choices as JSON array) -> returns {"success":true} or {"error":...}
// -----------------------------------------------------------------------------
//

type VoteProposalArgs struct {
	ProjectID  string   `json:"prjId"`
	PrposalID  string   `json:"propId"`
	choices    []string `json:"choices"`
	commitHash string   `json:"commit"`
}

//go:wasmexport proposals_vote
func VoteProposal(payload *string) *string {
	input, err := FromJSON[VoteProposalArgs](*payload)
	abortOnError(err, "invalid args")
	now := nowUnix()

	prj := loadProject(input.ProjectID)
	if prj.Paused {
		abortCustom("project is paused")
	}

	prpsl := loadProposal(input.PrposalID)

	if prpsl.State != StateActive {
		abortCustom("proposal inactive")
	}

	// Time window
	if prpsl.DurationSeconds > 0 && now > prpsl.CreatedAt+prpsl.DurationSeconds {
		// finalize instead
		_ = TallyProposal(prpsl.ID) // ignore result here
		abortCustom("voting period ended (tallied)")
	}

	sender := getSenderAddress()
	// Member check
	member, ok := prj.Members[sender]
	if !ok {
		abortCustom("only members may vote")
	}

	// Compute weight
	weight := int64(1)
	if prj.Config.VotingSystem == SystemStake {
		weight = member.Stake
		if weight <= 0 { // should never happen ut good to check
			abortCustom("member stake zero")
		}
	}

	// Parse choices
	var choices []int
	if err := json.Unmarshal([]byte(input.choices), &choices); err != nil {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "invalid choice json",
			},
		)
	}

	// Validate choices by type
	switch prpsl.Type {
	case VotingTypeBoolVote:
		if len(choices) != 1 || (choices[0] != 0 && choices[0] != 1) {
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "bool_vote requires single choice 0 or 1",
				},
			)

		}
	case VotingTypeSingleChoice:
		if len(choices) != 1 {
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "single_choice requires exactly 1 index",
				},
			)

		}
		if choices[0] < 0 || choices[0] >= len(prpsl.Options) {

			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "option index out of range",
				},
			)

		}
	case VotingTypeMultiChoice:
		if len(choices) == 0 {
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "multi_choice requires >=1 choices",
				},
			)
		}
		for _, idx := range choices {
			if idx < 0 || idx >= len(prpsl.Options) {
				return returnJsonResponse(
					false, map[string]interface{}{
						"details": "option index out of range",
					},
				)

			}
		}
	case VotingTypeMetaProposal:
		// no extra validation here
	default:
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "unknown proposal type",
			},
		)
	}

	vote := VoteRecord{
		ProjectID:   projectID,
		ProposalID:  proposalID,
		Voter:       caller,
		ChoiceIndex: choices,
		Weight:      weight,
		VotedAt:     now,
	}

	saveVote(&vote)
	// sdk.Log("VoteProposal: voter " + caller + " for " + proposalID)
	return returnJsonResponse(
		true, map[string]interface{}{
			"vote": vote,
		},
	)
}

// -----------------------------------------------------------------------------
// Tally Proposal -> returns {"success":true,"passed":bool,"proposal":{...}}
// -----------------------------------------------------------------------------
//
//go:wasmexport proposals_tally
func TallyProposal(proposalID string) *string {

	prpsl := loadProposal(proposalID)
	prj := loadProject(prpsl.ProjectID)

	// Use proposal-specific duration if set, else project default
	duration := prpsl.DurationSeconds
	if duration <= 0 {
		duration = prj.Config.ProposalDurationSecs
	}
	// Only tally if proposal duration has passed
	if nowUnix() < prpsl.CreatedAt+duration {
		abortCustom("proposal duration not over yet")
	}
	// compute total possible voting power
	var totalPossible int64 = 0
	if prj.Config.VotingSystem == SystemDemocratic {
		totalPossible = int64(len(prj.Members))
	} else {
		for _, m := range prj.Members {
			totalPossible += m.Stake
		}
	}
	if prj.Config.EnableSnapshot && prpsl.SnapshotTotal > 0 {
		totalPossible = prpsl.SnapshotTotal
	}

	// gather votes
	votes := loadVotesForProposal(projectID, proposalID)
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
	// sdk.Log("TallyProposal: tallied - passed=" + strconv.FormatBool(prpsl.Passed))
	return returnJsonResponse(
		true, map[string]interface{}{
			"passed":   prpsl.Passed,
			"proposal": prpsl,
		},
	)

}

// -----------------------------------------------------------------------------
// Execute Proposal -> returns JSON with action details or error
// (keeps your original flow; only return type changed)
// -----------------------------------------------------------------------------
//
//go:wasmexport proposals_execute
func ExecuteProposal(projectID, proposalID, asset string) *string {

	caller := getSenderAddress()

	prj := loadProject(projectID)
	if prj.Paused {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "project is paused",
			},
		)
	}
	prpsl, err := loadProposal(proposalID)
	if err != nil {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "proposal not found",
			},
		)
	}
	if prpsl.State != StateExecutable {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details":  "proposal not executable",
				"proposal": prpsl,
			},
		)

	}
	// check execution delay
	if prj.Config.ExecutionDelaySecs > 0 && nowUnix() < prpsl.PassTimestamp+prj.Config.ExecutionDelaySecs {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "execution delay not passed",
				"delay":   prj.Config.ExecutionDelaySecs,
				//TODO: add current elapsed time
			},
		)
	}
	// check permission to execute
	if prj.Config.ExecutePermission == PermCreatorOnly && caller != prpsl.Creator {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "only creator can execute",
				"creator": prpsl.Creator,
			},
		)

	}
	if prj.Config.ExecutePermission == PermAnyMember {
		if _, ok := prj.Members[caller]; !ok {
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "only members can execute",
				},
			)
		}
	}

	// For transfers: only yes/no allowed
	if prpsl.Type == VotingTypeBoolVote && prpsl.Amount > 0 && strings.TrimSpace(prpsl.Receiver) != "" {
		// ensure funds
		if prj.Funds < prpsl.Amount {
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "insufficient project funds",
					"funds":   prj.Funds,
					"needed":  prpsl.Amount,
					"asset":   prj.FundsAsset,
				},
			)

		}
		// transfer from contract to receiver
		sdk.HiveTransfer(sdk.Address(prpsl.Receiver), prpsl.Amount, sdk.Asset(asset))
		prj.Funds -= prpsl.Amount
		prpsl.Executed = true
		prpsl.State = StateExecuted
		prpsl.FinalizedAt = nowUnix()
		saveProposal(prpsl)
		saveProject(prj)
		// reward proposer if enabled
		if prj.Config.RewardEnabled && prj.Config.RewardPayoutOnExecute && prj.Config.RewardAmount > 0 && prj.Funds >= prj.Config.RewardAmount {
			prj.Funds -= prj.Config.RewardAmount
			// transfer reward to proposer
			sdk.HiveTransfer(sdk.Address(prpsl.Creator), prj.Config.RewardAmount, sdk.Asset(asset))
			saveProject(prj)
		}
		// sdk.Log("ExecuteProposal: transfer executed " + proposalID)
		return returnJsonResponse(
			true, map[string]interface{}{
				"to":     prpsl.Receiver,
				"amount": prpsl.Amount,
				"asset":  asset,
			},
		)

	}

	// Meta proposals: interpret json_metadata to perform allowed changes
	if prpsl.Type == VotingTypeMetaProposal {
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(prpsl.JsonMetadata), &meta); err != nil {
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "invalid meta json",
				},
			)

		}
		action, _ := meta["action"].(string)
		switch action {
		// TODO: add more project properties here
		case "update_threshold":
			if v, ok := meta["value"].(float64); ok {
				newv := int(v)
				if newv < 0 || newv > 100 {
					return returnJsonResponse(
						false, map[string]interface{}{
							"property": "update_threshold",
							"details":  "value out of range",
						},
					)

				}
				prj.Config.ThresholdPercent = newv
				prpsl.Executed = true
				prpsl.State = StateExecuted
				prpsl.FinalizedAt = nowUnix()
				saveProject(prj)
				saveProposal(prpsl)
				// sdk.Log("ExecuteProposal: updated threshold")
				return returnJsonResponse(
					true, map[string]interface{}{
						"property": "update_threshold",
						"value":    newv,
					},
				)

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
			return returnJsonResponse(
				true, map[string]interface{}{
					"property": "toggle_pause",
					"value":    prj.Paused,
				},
			)
		// TODO: add more meta actions here (update quorum, proposal cost, reward setting, etc.)
		default:
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "meta property unknown",
				},
			)
		}
	}

	// If nothing to execute, mark executed
	prpsl.Executed = true
	prpsl.State = StateExecuted
	prpsl.FinalizedAt = nowUnix()
	saveProposal(prpsl)
	// sdk.Log("ExecuteProposal: marked executed without transfer " + proposalID)
	return returnJsonResponse(
		true, map[string]interface{}{
			"details": "executed without meta change or transfer",
		},
	)
}

// -----------------------------------------------------------------------------
// Get single proposal -> returns {"success":true,"proposal":{...}}
// -----------------------------------------------------------------------------
//
//go:wasmexport proposals_get_one
func GetProposal(proposalID string) *string {

	prpsl, err := loadProposal(proposalID)
	if err != nil {
		return returnJsonResponse(
			"proposals_get_one", false, map[string]interface{}{
				"details": "proposal not found",
			},
		)
	}
	return returnJsonResponse(
		"proposals_get_one", true, map[string]interface{}{
			"propsal": prpsl,
		},
	)
}

// -----------------------------------------------------------------------------
// Get all proposals for a project -> returns {"success":true,"proposals":[...]}
// -----------------------------------------------------------------------------
//
//go:wasmexport proposals_get_all
func GetProjectProposals(projectID string) *string {

	ids := GetIDsFromIndex(idxProjectProposal + projectID)
	proposals := make([]Proposal, 0, len(ids))
	for _, id := range ids {
		if prpsl, err := loadProposal(id); err == nil {
			proposals = append(proposals, *prpsl)
		}
	}
	return returnJsonResponse(
		"proposals_get_one", true, map[string]interface{}{
			"propsal": proposals,
		},
	)
}

func saveProposal(prpsl *Proposal) {
	key := proposalKey(prpsl.ID)
	b, err := json.Marshal(prpsl)
	abortOnError(err, "failed to marshal")
	sdk.StateSetObject(key, string(b))
}

func loadProposal(id string) *Proposal {
	key := proposalKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil {
		abortCustom("proposal not found")
	}
	var prpsl Proposal
	if err := json.Unmarshal([]byte(*ptr), &prpsl); err != nil {
		abortCustom("failed to unmarshal proposal")
	}
	return &prpsl
}

func saveVote(vote *VoteRecord) {
	key := voteKey(vote.ProjectID, vote.ProposalID, vote.Voter)
	b, _ := json.Marshal(vote)
	sdk.StateSetObject(key, string(b))

	// ensure voter listed in index for iteration (store list under project:proposal:voters)
	votersKey := fmt.Sprintf("proposal:%s:%s:voters", vote.ProjectID, vote.ProposalID)
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

func loadVotesForProposal(projectID, proposalID string) []VoteRecord {
	votersKey := fmt.Sprintf("proposal:%s:%s:voters", projectID, proposalID)
	ptr := sdk.StateGetObject(votersKey)
	if ptr == nil {
		return []VoteRecord{}
	}
	var voters []string
	if err := json.Unmarshal([]byte(*ptr), &voters); err != nil {
		return []VoteRecord{}
	}
	out := make([]VoteRecord, 0, len(voters))
	for _, v := range voters {
		vk := voteKey(projectID, proposalID, v)
		vp := sdk.StateGetObject(vk)
		if vp == nil {
			continue
		}
		var vr VoteRecord
		if err := json.Unmarshal([]byte(*vp), &vr); err == nil {
			out = append(out, vr)
		}
	}
	return out
}

// remove vote only needed if member leaves project while still voted on an active proposal
func removeVote(projectID string, proposalID string, voter sdk.Address) {
	key := voteKey(projectID, proposalID, voter.String())
	sdk.StateDeleteObject(key)

	// remove from voter list
	votersKey := fmt.Sprintf("proposal:%s:%s:voters", projectID, proposalID)
	ptr := sdk.StateGetObject(votersKey)
	if ptr == nil {
		return
	}
	var voters []string
	json.Unmarshal([]byte(*ptr), &voters)
	newV := make([]string, 0, len(voters))
	for _, a := range voters {
		if a != voter.String() {
			newV = append(newV, a)
		}
	}
	nb, _ := json.Marshal(newV)
	sdk.StateSetObject(votersKey, string(nb))
}
