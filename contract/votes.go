package main

import (
	"okinoko_dao/sdk"
	"time"
)

// -----------------------------------------------------------------------------
// Voting
// -----------------------------------------------------------------------------

// proposalVoteKey generates a unique storage key for a vote
// based on the proposal ID and the voter's address.
func proposalVoteKey(id uint64, voter sdk.Address) string {
	return "v:" + UInt64ToString(id) + ":" + voter.String()
}

// hasVoted checks whether a voter has already cast a vote
// for a given proposal.
func hasVoted(id uint64, voter sdk.Address) bool {
	key := proposalVoteKey(id, voter)
	ptr := sdk.StateGetObject(key)
	return ptr != nil && *ptr != ""
}

// saveVote persists a voter's choices and voting weight
// for a specific proposal.
func saveVote(id uint64, voter sdk.Address, choices []uint, weight float64) {
	// save vote
	voteData := map[string]interface{}{
		"choices": choices,
		"weight":  weight,
	}
	sdk.StateSetObject(proposalVoteKey(id, voter), ToJSON(voteData, "vote"))
}

// VoteProposalArgs defines the JSON payload required to cast a vote.
//
// Fields:
//   - ProposalId: The ID of the proposal being voted on.
//   - Choices: A slice of option indices being voted for.
type VoteProposalArgs struct {
	ProposalId uint64 `json:"id"`
	Choices    []uint `json:"choices"` // TODO: add test for -1 as option
}

// VoteProposal allows a member of a project to cast a vote on an active proposal.
// Validates membership, proposal state, stake, and choice indices before recording the vote.
//
//go:wasmexport proposals_vote
func VoteProposal(payload *string) *string {
	input := FromJSON[VoteProposalArgs](*payload, "VoteProposalArgs")
	prpsl := loadProposal(input.ProposalId)

	if prpsl.State != ProposalActive {
		sdk.Abort("proposal not active")
	}
	if time.Now().Unix() > prpsl.CreatedAt+int64(prpsl.DurationHours)*3600 {
		sdk.Abort("proposal expired")
	}

	prj := loadProject(prpsl.ProjectID)
	voter := getSenderAddress()
	member := getMember(voter, prj.Members)
	if hasVoted(input.ProposalId, voter) {
		sdk.Abort("already voted")
	}
	// check if member joined after proposal
	if prpsl.CreatedAt > member.JoinedAt {
		sdk.Abort("proposal was created before joining the project")
	}

	// check if stakemin is still the same (it can get modified by proposals)
	if prj.Config.StakeMinAmt > member.Stake {
		sdk.Abort("minimum stake has changed since membership - increase stake")
	}

	weight := member.Stake

	// check if all voted options are valid
	for _, idx := range input.Choices {
		if idx >= uint(len(prpsl.Options)) {
			sdk.Abort("invalid option index")
		}
		prpsl.Options[idx].Votes = append(prpsl.Options[idx].Votes, weight)
	}

	saveVote(input.ProposalId, voter, input.Choices, weight)
	saveProposal(prpsl)
	emitVoteCasted(input.ProposalId, voter.String(), input.Choices, weight)
	return strptr("voted")
}
