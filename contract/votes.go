package main

import (
	"okinoko_dao/sdk"
	"time"
)

// -----------------------------------------------------------------------------
// Voting
// -----------------------------------------------------------------------------

func proposalVoteKey(id uint64, voter sdk.Address) string {
	return "v:" + UInt64ToString(id) + ":" + voter.String()
}

func hasVoted(id uint64, voter sdk.Address) bool {
	key := proposalVoteKey(id, voter)
	ptr := sdk.StateGetObject(key)
	return ptr != nil && *ptr != ""
}

func saveVote(id uint64, voter sdk.Address, choices []uint, weight float64) {
	// save vote
	voteData := map[string]interface{}{
		"choices": choices,
		"weight":  weight,
	}
	sdk.StateSetObject(proposalVoteKey(id, voter), ToJSON(voteData, "vote"))
}

type VoteProposalArgs struct {
	ProposalId uint64 `json:"id"`
	Choices    []uint `json:"choices"`
}

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
	member, ok := prj.Members[voter]
	if !ok {
		sdk.Abort("only members can vote")
	}
	if hasVoted(input.ProposalId, voter) {
		sdk.Abort("already voted")
	}
	// check if member joined after proposal
	if prpsl.CreatedAt > member.JoinedAt {
		sdk.Abort("proposal was created before joining the project")
	}

	weight := member.Stake

	// check if all voted options are valid
	for _, idx := range input.Choices {
		if idx >= uint(len(prpsl.Options)) {
			sdk.Abort("invalid option index")
		}
		prpsl.Options[idx].Votes += weight
	}

	saveVote(input.ProposalId, voter, input.Choices, weight)
	saveProposal(prpsl)
	emitVoteCasted(input.ProposalId, voter.String(), input.Choices, weight)
	return strptr("voted")
}
