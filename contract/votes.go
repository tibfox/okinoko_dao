package main

import (
	"bytes"
	"encoding/binary"
	"math"
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

type voteRecord struct {
	Choices []uint
	Weight  float64
}

func loadVoteRecord(id uint64, voter sdk.Address) *voteRecord {
	key := proposalVoteKey(id, voter)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return nil
	}
	rec := decodeVoteRecord(*ptr)
	return rec
}

func decodeVoteRecord(data string) *voteRecord {
	reader := bytes.NewReader([]byte(data))
	count, err := binary.ReadUvarint(reader)
	if err != nil {
		sdk.Abort("failed to decode vote record")
	}
	choices := make([]uint, 0, count)
	for i := uint64(0); i < count; i++ {
		val, err := binary.ReadUvarint(reader)
		if err != nil {
			sdk.Abort("failed to decode vote choice")
		}
		choices = append(choices, uint(val))
	}
	var floatBuf [8]byte
	if _, err := reader.Read(floatBuf[:]); err != nil {
		sdk.Abort("failed to decode vote weight")
	}
	weight := math.Float64frombits(binary.BigEndian.Uint64(floatBuf[:]))
	return &voteRecord{Choices: choices, Weight: weight}
}

// VoteProposal validates membership + weight, then updates options and stores the vote receipt.
// Example payload: VoteProposal(strptr("12|0,1"))
//
//go:wasmexport proposals_vote
func VoteProposal(payload *string) *string {
	input := decodeVoteProposalArgs(payload)
	prpsl := loadProposal(input.ProposalID)

	if prpsl.State != ProposalActive {
		sdk.Abort("proposal not active")
	}
	prj := loadProject(prpsl.ProjectID)
	voter := getSenderAddress()
	voterAddr := voter
	member := getMember(prj.ID, voterAddr)

	prevVote := loadVoteRecord(input.ProposalID, voter)
	// check if member joined after proposal
	if member.JoinedAt > prpsl.CreatedAt {
		sdk.Abort("proposal was created before joining the project")
	}

	// check if stakemin is still the same (it can get modified by proposals)
	if FloatToAmount(prj.Config.StakeMinAmt) > member.Stake {
		sdk.Abort("minimum stake has changed since membership - increase stake")
	}

	weight := member.Stake

	// Load all options once to avoid repeated storage reads
	optionCache := make(map[uint32]*ProposalOption)

	if prevVote != nil {
		prevWeight := FloatToAmount(prevVote.Weight)
		seenPrev := map[uint]bool{}
		for _, idx := range prevVote.Choices {
			if seenPrev[idx] {
				continue
			}
			seenPrev[idx] = true
			if idx >= uint(prpsl.OptionCount) {
				continue
			}
			idx32 := uint32(idx)
			if optionCache[idx32] == nil {
				optionCache[idx32] = loadProposalOption(prpsl.ID, idx32)
			}
			option := optionCache[idx32]
			if option.WeightTotal > prevWeight {
				option.WeightTotal -= prevWeight
			} else {
				option.WeightTotal = 0
			}
			if option.VoterCount > 0 {
				option.VoterCount--
			}
		}
	}

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
		idx32 := uint32(idx)
		if optionCache[idx32] == nil {
			optionCache[idx32] = loadProposalOption(prpsl.ID, idx32)
		}
		option := optionCache[idx32]
		option.WeightTotal += weight
		option.VoterCount++
	}

	// Save all modified options
	for idx32, option := range optionCache {
		saveProposalOption(prpsl.ID, idx32, option)
	}

	saveVote(input.ProposalID, voter, input.Choices, AmountToFloat(weight))
	emitVoteCasted(input.ProposalID, AddressToString(voterAddr), input.Choices, AmountToFloat(weight))
	return strptr("voted")
}
