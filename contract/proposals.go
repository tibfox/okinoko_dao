package main

import (
	"encoding/json"
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
	"strings"
)

//go:wasmexport proposals_create
func CreateProposal(projectID string, name string, description string, jsonMetadata string,
	vtype VotingType, options []string, receiver string, amount int64) (string, error) {

	caller := getSenderAddress()

	prj, err := loadProject(projectID)
	if err != nil {
		return "", err
	}
	if prj.Paused {
		return "", fmt.Errorf("project paused")
	}

	// permission check
	if prj.Config.ProposalPermission == PermCreatorOnly && caller != prj.Owner {
		return "", fmt.Errorf("only project owner can create proposals")
	}
	if prj.Config.ProposalPermission == PermAnyMember {
		if _, ok := prj.Members[caller]; !ok {
			return "", fmt.Errorf("only members can create proposals")
		}
	}

	// options validation
	if (vtype == VotingTypeSingleChoice || vtype == VotingTypeMultiChoice) && len(options) == 0 {
		return "", fmt.Errorf("poll proposals require options")
	}

	// charge proposal cost (draw funds from caller to contract)
	if prj.Config.ProposalCost > 0 {
		sdk.HiveDraw(prj.Config.ProposalCost, sdk.Asset("VSC")) // TODO: what happens when not enough funds?!
		prj.Funds += prj.Config.ProposalCost
		saveProject(prj)
	}

	// create proposal
	id := generateGUID()
	now := nowUnix()
	duration := prj.Config.ProposalDurationSecs
	if duration <= 0 {
		duration = 60 * 60 * 24 * 7 // default 7 days
	}
	prpsl := Proposal{
		ID:              id,
		ProjectID:       projectID,
		Creator:         caller,
		Name:            name,
		Description:     description,
		JsonMetadata:    jsonMetadata,
		Type:            vtype,
		Options:         options,
		Receiver:        receiver,
		Amount:          amount,
		CreatedAt:       now,
		DurationSeconds: duration,
		State:           StateActive,
		Passed:          false,
		Executed:        false,
		TxID:            getTxID(),
	}

	// If snapshot enabled, compute snapshot total voting power
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

	saveProposal(&prpsl)
	addProposalToProjectIndex(projectID, id)
	sdk.Log("CreateProposal: " + id + " in project " + projectID)
	return id, nil
}

//go:wasmexport proposals_vote
func VoteProposal(projectID, proposalID string, choices []int, commitHash string) error {
	caller := getSenderAddress()
	now := nowUnix()

	prj, err := loadProject(projectID)
	if err != nil {
		return err
	}
	if prj.Paused {
		return fmt.Errorf("project paused")
	}
	prpsl, err := loadProposal(proposalID)
	if err != nil {
		return err
	}
	if prpsl.State != StateActive {
		return fmt.Errorf("proposal not active")
	}
	// time window
	if prpsl.DurationSeconds > 0 && now > prpsl.CreatedAt+prpsl.DurationSeconds {
		// finalize instead
		TallyProposal(projectID, proposalID)
		return fmt.Errorf("voting period ended (tallied)")
	}
	// member check
	member, ok := prj.Members[caller]
	if !ok {
		return fmt.Errorf("only members may vote")
	}
	// compute weight
	var weight int64 = 1
	if prj.Config.VotingSystem == SystemStake {
		weight = member.Stake
		if weight <= 0 {
			return fmt.Errorf("member stake zero")
		}
	}

	// validate choices by type
	switch prpsl.Type {
	case VotingTypeBoolVote:
		if len(choices) != 1 || (choices[0] != 0 && choices[0] != 1) {
			return fmt.Errorf("yes_no requires single choice 0 or 1")
		}
	case VotingTypeSingleChoice:
		if len(choices) != 1 {
			return fmt.Errorf("single_choice requires exactly 1 index")
		}
		if choices[0] < 0 || choices[0] >= len(prpsl.Options) {
			return fmt.Errorf("option index out of range")
		}
	case VotingTypeMultiChoice:
		if len(choices) == 0 {
			return fmt.Errorf("multi_choice requires >=1 choices")
		}
		for _, idx := range choices {
			if idx < 0 || idx >= len(prpsl.Options) {
				return fmt.Errorf("option index out of range")
			}
		}
	case VotingTypeMetaProposal:
		// same validations as polls depending on meta semantics
	default:
		return fmt.Errorf("unknown proposal type")
	}

	vote := VoteRecord{
		ProjectID:   projectID,
		ProposalID:  proposalID,
		Voter:       caller,
		ChoiceIndex: choices,
		Weight:      weight,

		VotedAt: now,
	}

	saveVote(&vote)
	sdk.Log("VoteProposal: voter " + caller + " for " + proposalID)
	return nil
}

//go:wasmexport proposals_tally
func TallyProposal(projectID, proposalID string) (bool, error) {
	prj, err := loadProject(projectID)
	if err != nil {
		return false, err
	}
	prpsl, err := loadProposal(proposalID)
	if err != nil {
		return false, err
	}

	// Only tally if proposal duration has passed
	if nowUnix() < prpsl.CreatedAt+prj.Config.ProposalDurationSecs {
		// Proposal duration not yet over, do not change state
		sdk.Log("TallyProposal: proposal duration not over yet")
		return false, nil
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
		sdk.Log("TallyProposal: quorum not reached")
		return false, nil
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
	sdk.Log("TallyProposal: tallied - passed=" + strconv.FormatBool(prpsl.Passed))
	return prpsl.Passed, nil
}

//go:wasmexport proposals_execute
func ExecuteProposal(projectID, proposalID, asset string) error {
	caller := getSenderAddress()

	prj, err := loadProject(projectID)
	if err != nil {
		return err
	}
	if prj.Paused {
		return fmt.Errorf("project paused")
	}
	prpsl, err := loadProposal(proposalID)
	if err != nil {
		return err
	}
	if prpsl.State != StateExecutable {
		return fmt.Errorf("proposal not executable (state=%s)", prpsl.State)
	}
	// check execution delay
	if prj.Config.ExecutionDelaySecs > 0 && nowUnix() < prpsl.PassTimestamp+prj.Config.ExecutionDelaySecs {
		return fmt.Errorf("execution delay not passed")
	}
	// check permission to execute
	if prj.Config.ExecutePermission == PermCreatorOnly && caller != prpsl.Creator {
		return fmt.Errorf("only creator can execute")
	}
	if prj.Config.ExecutePermission == PermAnyMember {
		if _, ok := prj.Members[caller]; !ok {
			return fmt.Errorf("only members can execute")
		}
	}
	// For transfers: only yes/no allowed
	if prpsl.Type == VotingTypeBoolVote && prpsl.Amount > 0 && strings.TrimSpace(prpsl.Receiver) != "" {
		// ensure funds
		if prj.Funds < prpsl.Amount {
			return fmt.Errorf("insufficient project funds")
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
		sdk.Log("ExecuteProposal: transfer executed " + proposalID)
		return nil
	}
	// Meta proposals: interpret json_metadata to perform allowed changes
	if prpsl.Type == VotingTypeMetaProposal {
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(prpsl.JsonMetadata), &meta); err != nil {
			return fmt.Errorf("invalid meta json")
		}
		action, _ := meta["action"].(string)
		switch action {
		// TODO: add more project properties here
		case "update_threshold":
			if v, ok := meta["value"].(float64); ok {
				newv := int(v)
				if newv < 0 || newv > 100 {
					return fmt.Errorf("threshold out of range")
				}
				prj.Config.ThresholdPercent = newv
				prpsl.Executed = true
				prpsl.State = StateExecuted
				prpsl.FinalizedAt = nowUnix()
				saveProject(prj)
				saveProposal(prpsl)
				sdk.Log("ExecuteProposal: updated threshold")
				return nil
			}
		case "toggle_pause":
			if val, ok := meta["value"].(bool); ok {
				prj.Paused = val
				prpsl.Executed = true
				prpsl.State = StateExecuted
				prpsl.FinalizedAt = nowUnix()
				saveProject(prj)
				saveProposal(prpsl)
				sdk.Log("ExecuteProposal: toggled pause")
				return nil
			} else {
				// flip
				prj.Paused = !prj.Paused
				prpsl.Executed = true
				prpsl.State = StateExecuted
				prpsl.FinalizedAt = nowUnix()
				saveProject(prj)
				saveProposal(prpsl)
				sdk.Log("ExecuteProposal: toggled pause (flip)")
				return nil
			}
		// TODO: add more meta actions here (update quorum, proposal cost, reward setting, etc.)
		default:
			return fmt.Errorf("unknown meta action")
		}
	}
	// If nothing to execute, mark executed
	prpsl.Executed = true
	prpsl.State = StateExecuted
	prpsl.FinalizedAt = nowUnix()
	saveProposal(prpsl)
	sdk.Log("ExecuteProposal: marked executed without transfer " + proposalID)
	return nil
}

//go:wasmexport proposals_get_one
func GetProposal(proposalID string) *Proposal {
	prpsl, err := loadProposal(proposalID)
	if err != nil {
		return nil
	}
	return prpsl
}

// AddFunds - draw funds from caller and add to project's treasury pool
// If the project is a stake based system & the sender is a valid mamber then the stake of the member will get updated accordingly.

//go:wasmexport projects_add_funds
func AddFunds(projectID string, amount int64, asset string) {
	if amount <= 0 {
		sdk.Log("AddFunds: amount must be > 0")
		return
	}
	prj, err := loadProject(projectID)
	if err != nil {
		sdk.Log("AddFunds: project not found")
		return
	}
	caller := getSenderAddress()

	sdk.HiveDraw(amount, sdk.Asset(asset))
	prj.Funds += amount

	// if stake based
	if prj.Config.VotingSystem == SystemStake {
		// check if member
		m, ismember := prj.Members[caller]
		if ismember {
			now := nowUnix()
			m.Stake = m.Stake + amount
			m.LastActionAt = now
			// add member with exact stake
			prj.Members[caller] = m
		}
	}

	saveProject(prj)
	sdk.Log("AddFunds: added " + strconv.FormatInt(amount, 10))
}

//go:wasmexport projects_join
func JoinProject(projectID string, amount int64, assetString string) {
	caller := getSenderAddress()

	prj, err := loadProject(projectID)
	asset := sdk.Asset(assetString)

	if err != nil {
		sdk.Log("JoinProject: project not found")
		return
	}
	if prj.Paused {
		sdk.Log("JoinProject: project paused")
		return
	}
	if amount <= 0 {
		sdk.Log("JoinProject: amount must be > 0")
		return
	}
	if asset != prj.FundsAsset {
		sdk.Log(fmt.Sprintf("JoinProject: asset must match the project main asset: %s", prj.FundsAsset.String()))
		return
	}

	now := nowUnix()
	if prj.Config.VotingSystem == SystemDemocratic {
		if amount != prj.Config.DemocraticExactAmt {
			sdk.Log(fmt.Sprintf("JoinProject: democratic projects need an exact amount to join: %d %s", prj.Config.DemocraticExactAmt, prj.FundsAsset.String()))
			return
		}
		// transfer funds into contract
		sdk.HiveDraw(amount, sdk.Asset(asset)) // TODO: what if not enough funds?!

		// add member with stake 1
		prj.Members[caller] = Member{
			Address:      caller,
			Stake:        1,
			Role:         RoleMember,
			JoinedAt:     now,
			LastActionAt: now,
			Reputation:   0,
		}
		prj.Funds += amount
	} else { // if the project is a stake based system
		if amount < prj.Config.StakeMinAmt {
			sdk.Log(fmt.Sprintf("JoinProject: the sent amount < than the minimum projects entry fee: %d %s", prj.Config.StakeMinAmt, prj.FundsAsset.String()))

			return
		}
		_, ok := prj.Members[caller]
		if ok {
			sdk.Log("JoinProject: already member")
			return
		} else {
			// transfer funds into contract
			sdk.HiveDraw(amount, sdk.Asset(asset)) // TODO: what if not enough funds?!
			// add member with exact stake
			prj.Members[caller] = Member{
				Address:      caller,
				Stake:        amount,
				Role:         RoleMember,
				JoinedAt:     now,
				LastActionAt: now,
			}
		}
		prj.Funds += amount
	}
	saveProject(prj)
	sdk.Log("JoinProject: " + projectID + " by " + caller)
}

//go:wasmexport projects_leave
func LeaveProject(projectID string, withdrawAmount int64, asset string) {
	caller := getSenderAddress()

	prj, err := loadProject(projectID)
	if err != nil {
		sdk.Log("LeaveProject: project not found")
		return
	}
	if prj.Paused {
		sdk.Log("LeaveProject: project paused")
		return
	}
	member, ok := prj.Members[caller]
	if !ok {
		sdk.Log("LeaveProject: not a member")
		return
	}

	now := nowUnix()
	// if exit requested previously -> try to withdraw
	if member.ExitRequested > 0 {
		if now-member.ExitRequested < prj.Config.LeaveCooldownSecs {
			sdk.Log("LeaveProject: cooldown not passed")
			return
		}
		// withdraw stake (for stake-based) or refund democratic amount
		if prj.Config.VotingSystem == SystemDemocratic {
			refund := prj.Config.DemocraticExactAmt
			if prj.Funds < refund {
				sdk.Log("LeaveProject: insufficient project funds")
				return
			}
			prj.Funds -= refund
			// transfer back to caller
			sdk.HiveTransfer(sdk.Address(caller), refund, sdk.Asset(asset))
			delete(prj.Members, caller)
			// remove votes
			for _, pid := range listProposalIDsForProject(projectID) {
				removeVote(projectID, pid, caller)
			}
			saveProject(prj)
			sdk.Log("LeaveProject: democratic refunded")
			return
		}
		// stake-based
		withdraw := member.Stake
		if withdraw <= 0 {
			sdk.Log("LeaveProject: nothing to withdraw")
			return
		}
		if prj.Funds < withdraw {
			sdk.Log("LeaveProject: insufficient project funds")
			return
		}
		prj.Funds -= withdraw
		sdk.HiveTransfer(sdk.Address(caller), withdraw, sdk.Asset(asset))
		delete(prj.Members, caller)
		for _, pid := range listProposalIDsForProject(projectID) {
			removeVote(projectID, pid, caller)
		}
		saveProject(prj)
		sdk.Log("LeaveProject: withdrew stake")
		return
	}

	// otherwise set exit requested timestamp
	member.ExitRequested = now
	prj.Members[caller] = member
	saveProject(prj)
	sdk.Log("LeaveProject: exit requested")
}

//go:wasmexport proposals_get_all
func GetProjectProposals(projectID string) []Proposal {
	ids := listProposalIDsForProject(projectID)
	out := make([]Proposal, 0, len(ids))
	for _, id := range ids {
		if prpsl, err := loadProposal(id); err == nil {
			out = append(out, *prpsl)
		}
	}
	return out
}

//go:wasmexport projects_transfer_ownership
func TransferProjectOwnership(projectID, newOwner string) error {
	caller := getSenderAddress()

	prj, err := loadProject(projectID)
	if err != nil {
		return err
	}
	if caller != prj.Owner {
		return fmt.Errorf("only creator")
	}
	prj.Owner = newOwner
	// ensure new owner exists as member
	if _, ok := prj.Members[newOwner]; !ok {
		prj.Members[newOwner] = Member{
			Address:      newOwner,
			Stake:        0,
			Role:         RoleMember,
			JoinedAt:     nowUnix(),
			LastActionAt: nowUnix(),
		}
	}
	saveProject(prj)
	sdk.Log("TransferProjectOwnership: " + projectID + " -> " + newOwner)
	return nil
}

//go:wasmexport projects_pause
func EmergencyPauseImmediate(projectID string, pause bool) error {
	caller := getSenderAddress()
	prj, err := loadProject(projectID)
	if err != nil {
		return err
	}
	if caller != prj.Owner {
		return fmt.Errorf("only the project owner can pause / unpause without dedicated meta proposal")
	}
	prj.Paused = pause
	saveProject(prj)
	sdk.Log("EmergencyPauseImmediate: set paused=" + strconv.FormatBool(pause))
	return nil
}
