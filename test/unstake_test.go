package contract_test

// Partial unstake (project_unstake) — withdraw part of a member's stake while
// keeping membership, two-phase with the owner-configured leave cooldown.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// U-1: request -> (cooldown) -> finalize withdraws the requested amount and
// refunds it, while the member stays in the project.
func TestUnstake_PartialWithdrawRefunds(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1") // stake, min 1.000, cooldown 10h
	joinWithStake(t, ct, pid, "hive:member2", "100.000")

	req := rawCallAt(ct, "project_unstake", PayloadString(fmt.Sprintf("%d|40.000", pid)), nil, "hive:member2", defaultTimestamp, "u1")
	assert.True(t, req.Success, "unstake request failed: %s", req.Ret)

	// Finalizing before the cooldown must fail.
	early := rawCallAt(ct, "project_unstake", PayloadUint64(pid), nil, "hive:member2", defaultTimestamp, "u2")
	assertAborts(t, early, "cooldown not passed", "finalized before cooldown")

	before := hiveBal(ct, "hive:member2")
	fin := rawCallAt(ct, "project_unstake", PayloadUint64(pid), nil, "hive:member2", lateTS, "u3")
	assert.True(t, fin.Success, "unstake finalize failed: %s", fin.Ret)
	after := hiveBal(ct, "hive:member2")
	assert.Equal(t, before+40000, after, "40.000 was not refunded")

	// Still a member: a fresh partial-unstake request is accepted (60.000 left).
	again := rawCallAt(ct, "project_unstake", PayloadString(fmt.Sprintf("%d|10.000", pid)), nil, "hive:member2", lateTS, "u4")
	assert.True(t, again.Success, "member was removed by a partial unstake: %s", again.Ret)
}

// U-2: cannot withdraw so much that the remaining stake drops below StakeMinAmt —
// that path is a full leave, not a partial unstake.
func TestUnstake_RejectsDropBelowMinimum(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1") // min 1.000
	joinWithStake(t, ct, pid, "hive:member2", "5.000")
	res := rawCallAt(ct, "project_unstake", PayloadString(fmt.Sprintf("%d|4.500", pid)), nil, "hive:member2", defaultTimestamp, "u")
	assertAborts(t, res, "remaining stake would fall below the minimum", "allowed unstake below the minimum")
}

// U-3: one-member-one-vote projects hold a fixed stake, so partial unstake is
// rejected outright.
func TestUnstake_RejectedInDemocraticProject(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "0", "50.000", "50.000") // democratic
	joinProjectMember(t, ct, pid, "hive:member2")
	res := rawCallAt(ct, "project_unstake", PayloadString(fmt.Sprintf("%d|0.100", pid)), nil, "hive:member2", defaultTimestamp, "u")
	assertAborts(t, res, "partial unstake not supported", "partial unstake allowed in a democratic project")
}

// U-4: a member who withdraws stake after a proposal is created must not vote
// with the higher stake-at-creation snapshot — the weight is capped at what they
// currently hold, so a payout that would pass at the old weight fails.
func TestUnstake_ReducedStakeCapsVotingWeight(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1") // stake, threshold 50%, quorum 1%, min 1.000, cooldown 10h
	joinWithStake(t, ct, pid, "hive:member2", "100.000")
	addTreasuryFunds(t, ct, pid, "2.000")

	// Proposal with a 20h window (> the 10h cooldown) so member2 can unstake mid-vote.
	fields := []string{fmt.Sprintf("%d", pid), "payout", "d", "20", "", "0", "hive:member2:0.500:hive", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "p")
	assert.True(t, ok, "proposal create failed")

	const tMid = "2025-09-03T11:00:00" // +11h: past the 10h cooldown, inside the 20h window
	req := rawCallAt(ct, "project_unstake", PayloadString(fmt.Sprintf("%d|99.000", pid)), nil, "hive:member2", defaultTimestamp, "u1")
	assert.True(t, req.Success, "unstake request failed: %s", req.Ret)
	fin := rawCallAt(ct, "project_unstake", PayloadUint64(pid), nil, "hive:member2", tMid, "u2")
	assert.True(t, fin.Success, "unstake finalize failed: %s", fin.Ret)

	// Vote with only 1.000 stake left — weight must be capped at 1.000, not 100.000.
	assert.True(t, rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", propID)), nil, "hive:member2", tMid, "v").Success)

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:member2")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:member2")
	assertAborts(t, exec, "proposal is failed", "capped vote still passed the payout")
	assert.Equal(t, before, after, "payout executed despite the capped weight")
}
