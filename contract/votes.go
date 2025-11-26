package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"okinoko_dao/contract/dao"
	"okinoko_dao/sdk"
)

// -----------------------------------------------------------------------------
// Voting
// -----------------------------------------------------------------------------

// proposalVoteKey generates a unique storage key for a vote
// based on the proposal ID and the voter's address.
func proposalVoteKey(id uint64, voter sdk.Address) string {
	addr := voter.String()
	buf := make([]byte, 0, 1+8+len(addr))
	buf = append(buf, kVoteReceipt)
	buf = packU64LE(id, buf)
	buf = append(buf, addr...)
	return string(buf)
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
	data := encodeVoteRecord(choices, weight)
	sdk.StateSetObject(proposalVoteKey(id, voter), data)
}

func encodeVoteRecord(choices []uint, weight float64) string {
	var buf bytes.Buffer
	var tmp [binary.MaxVarintLen64]byte
	count := binary.PutUvarint(tmp[:], uint64(len(choices)))
	buf.Write(tmp[:count])
	for _, choice := range choices {
		n := binary.PutUvarint(tmp[:], uint64(choice))
		buf.Write(tmp[:n])
	}
	var floatBuf [8]byte
	binary.BigEndian.PutUint64(floatBuf[:], math.Float64bits(weight))
	buf.Write(floatBuf[:])
	return buf.String()
}

// VoteProposal allows a member of a project to cast a vote on an active proposal.
// Validates membership, proposal state, stake, and choice indices before recording the vote.
//
//go:wasmexport proposals_vote
func VoteProposal(payload *string) *string {
	input := decodeVoteProposalArgs(payload)
	prpsl := loadProposal(input.ProposalID)

	if prpsl.State != dao.ProposalActive {
		sdk.Abort("proposal not active")
	}
	if nowUnix() > prpsl.CreatedAt+int64(prpsl.DurationHours)*3600 {
		sdk.Abort("proposal expired")
	}

	prj := loadProject(prpsl.ProjectID)
	voter := getSenderAddress()
	voterAddr := dao.AddressFromString(voter.String())
	member := getMember(prj.ID, voterAddr)
	if hasVoted(input.ProposalID, voter) {
		sdk.Abort("already voted")
	}
	// check if member joined after proposal
	if member.JoinedAt > prpsl.CreatedAt {
		sdk.Abort("proposal was created before joining the project")
	}

	// check if stakemin is still the same (it can get modified by proposals)
	if prj.Config.StakeMinAmt > dao.AmountToFloat(member.Stake) {
		sdk.Abort("minimum stake has changed since membership - increase stake")
	}

	weight := member.Stake

	// check if all voted options are valid
	seen := map[uint]bool{}
	for _, idx := range input.Choices {
		if idx >= uint(prpsl.OptionCount) {
			sdk.Abort("invalid option index")
		}
		// avoid double-counting same option in one vote
		if seen[idx] {
			continue
		}
		seen[idx] = true
		option := loadProposalOption(prpsl.ID, uint32(idx))
		option.WeightTotal += weight
		option.VoterCount++
		saveProposalOption(prpsl.ID, uint32(idx), option)
	}

	saveVote(input.ProposalID, voter, input.Choices, dao.AmountToFloat(weight))
	emitVoteCasted(input.ProposalID, dao.AddressToString(voterAddr), input.Choices, dao.AmountToFloat(weight))
	return strptr("voted")
}
