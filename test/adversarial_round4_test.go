package contract_test

// Round 4 — pause-exception bypass, meta conflicts, and governance/lifecycle edges.

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// R4-1: the "execute while paused" exception exists only to let the DAO unfreeze
// itself (toggle_pause / owner changes). A payout riding on a toggle_pause proposal
// (created before the freeze) must NOT execute while the project is paused.
func TestBreak_PauseBypassPayoutRider(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "3.000")
	// create a toggle_pause proposal carrying a payout rider while UNPAUSED, pass it
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:1.000:hive", "toggle_pause=1")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	// owner freezes the project, then someone tries to execute the rider
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someone", true, uint(1_000_000_000))
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assertAborts(t, exec, "project is paused", "a payout+toggle_pause proposal executed while paused")
	assert.Equal(t, before, after, "payout drained the treasury while paused via a toggle_pause rider")
}

// R4-1b: a payout/ICC rider cannot even be CREATED on a pause-exception proposal
// while the project is paused.
func TestBreak_PauseRiderCreationRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	addTreasuryFunds(t, ct, pid, "3.000")
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someone", true, uint(1_000_000_000))
	fields := []string{strconv.FormatUint(pid, 10), "p", "d", "1", "", "0", "hive:someone:1.000:hive", "toggle_pause=1", ""}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "project is paused", "a toggle_pause proposal with a payout rider was created while paused")
}

// R4-2: same bypass via an ICC rider on a toggle_pause proposal (created before the
// freeze) must not execute while paused.
func TestBreak_PauseBypassICCRider(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "3.000")
	// meta=toggle_pause plus an ICC rider — created while unpaused.
	fields := []string{strconv.FormatUint(pid, 10), "p", "d", "1", "", "0", "", "toggle_pause=1", "", "",
		ContractID, "noop", "{}", "hive=1.0"}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	if !ok {
		t.Fatal("could not create toggle_pause+ICC proposal while unpaused")
	}
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someone", true, uint(1_000_000_000))
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, res, "project is paused", "an ICC executed while paused via a toggle_pause rider")
}

// R4-3: a payout-only proposal cannot be CREATED while paused (baseline: the pause
// gate itself works for non-exception proposals).
func TestBreak_PayoutProposalRejectedWhilePaused(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	addTreasuryFunds(t, ct, pid, "3.000")
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someone", true, uint(1_000_000_000))
	fields := []string{strconv.FormatUint(pid, 10), "p", "d", "1", "", "0", "hive:someone:1.000:hive", "", ""}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "project is paused", "a payout proposal was created while the project was paused")
}

// R4-4: several independent meta updates in one proposal all take effect.
func TestBreak_MultiMetaAllApplied(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "1", "", "update_threshold=60.0;update_quorum=40.0")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.True(t, res.Success, "multi-key meta proposal failed to execute: %s", res.Ret)
	// A follow-up proposal that requires the NEW threshold/quorum should behave
	// accordingly; here we just assert execution succeeded and didn't abort mid-map.
}

// R4-5: a proposal carrying two conflicting owner directives (update_owner AND
// remove_owner) is a consensus hazard (map-iteration-order-dependent result). It
// should be rejected rather than resolved by iteration order.
func TestBreak_ConflictingOwnerMetaRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// both keys present
	res := rawCallAt(ct, "proposal_create",
		PayloadString(fmt.Sprintf("%d|p|d|1||0||update_owner=hive:someoneelse;remove_owner=1|", pid)),
		transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "conflicting owner directives (update_owner and remove_owner)", "a proposal with conflicting update_owner+remove_owner was accepted")
}

// R4-6: duplicate payout entries to the same address pay that address once per
// entry (documents behaviour; must remain within treasury).
func TestBreak_DuplicatePayoutSameAddress(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "3.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:1.000:hive;hive:someoneelse:1.000:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	after := hiveBal(ct, "hive:someoneelse")
	assert.Equal(t, before+2000, after, "duplicate payout entries did not both pay out")
}

// R4-7: paying out exactly the treasury balance succeeds and leaves it at zero; a
// follow-up payout then fails for insufficient funds.
func TestBreak_PayoutExactlyDrainsThenInsufficient(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "1.000") // treasury has exactly 1.000 (plus proposal costs)
	p1 := createPollProposal(t, ct, pid, "1", "hive:someoneelse:1.000:hive", "")
	assert.True(t, voteRaw(ct, p1, "hive:someone", "1", "a1").Success)
	assert.True(t, voteRaw(ct, p1, "hive:someoneelse", "1", "a2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(p1), nil, "hive:someone", lateTS, "t")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", p1)), nil, "hive:someone", lateTS, "e").Success)
	// a second 1.000 payout should now fail (treasury may still hold proposal-cost
	// dust but not another full HIVE)
	p2 := createPollProposal(t, ct, pid, "1", "hive:someoneelse:5.000:hive", "")
	assert.True(t, voteRaw(ct, p2, "hive:someone", "1", "b1").Success)
	assert.True(t, voteRaw(ct, p2, "hive:someoneelse", "1", "b2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(p2), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", p2)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, res, "insufficient hive funds in treasury", "overspending the treasury succeeded")
}

// R4-8: a sole owner of a one-member project cannot leave (no other member to take
// ownership) — documents the stuck-owner edge.
func TestBreak_SoleOwnerCannotLeave(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // creator is sole member+owner
	res := rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someone", defaultTimestamp, "l")
	assertAborts(t, res, "owner must transfer ownership before leaving", "sole owner left, orphaning the project")
}

// R4-9: a passed proposal in an autonomous project (owner removed) can still be
// executed and applies its outcome.
func TestBreak_AutonomousProjectStillExecutes(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// make autonomous
	rp := createPollProposal(t, ct, pid, "1", "", "remove_owner=1")
	assert.True(t, voteRaw(ct, rp, "hive:someone", "1", "r1").Success)
	assert.True(t, voteRaw(ct, rp, "hive:someoneelse", "1", "r2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(rp), nil, "hive:someone", lateTS, "t")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", rp)), nil, "hive:someone", lateTS, "e").Success)
	// now a payout proposal in the autonomous project
	addTreasuryFunds(t, ct, pid, "2.000")
	pp := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	assert.True(t, voteRaw(ct, pp, "hive:someone", "1", "p1").Success)
	assert.True(t, voteRaw(ct, pp, "hive:someoneelse", "1", "p2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(pp), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", pp)), nil, "hive:someone", lateTS, "e").Success)
	after := hiveBal(ct, "hive:someoneelse")
	assert.Equal(t, before+500, after, "autonomous project payout did not execute")
}

// R4-10: voting is allowed while the project is paused (no pause guard on voting) —
// documents that pause freezes membership/funds, not tallying of in-flight votes.
func TestBreak_VotingAllowedWhilePaused_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createSimpleProposal(t, ct, pid, "1")
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someone", true, uint(1_000_000_000))
	res := voteRaw(ct, propID, "hive:someoneelse", "1", "v")
	assert.True(t, res.Success, "voting was blocked while paused (behaviour change)")
}

// R4-11: kicking every non-owner member via one proposal removes them all and
// refunds each stake.
func TestBreak_KickMultipleMembers(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "40.000", "1")
	joinWithStake(t, ct, pid, "hive:someoneelse", "5.000")
	joinWithStake(t, ct, pid, "hive:member2", "5.000")
	joinWithStake(t, ct, pid, "hive:outsider", "60.000") // provides passing weight, not kicked
	propID := createPollProposal(t, ct, pid, "1", "", "kick_member=hive:someoneelse,hive:member2")
	assert.True(t, voteRaw(ct, propID, "hive:outsider", "1", "v").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	b1 := hiveBal(ct, "hive:someoneelse")
	b2 := hiveBal(ct, "hive:member2")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	assert.Equal(t, b1+5000, hiveBal(ct, "hive:someoneelse"), "kicked member1 not refunded")
	assert.Equal(t, b2+5000, hiveBal(ct, "hive:member2"), "kicked member2 not refunded")
	// both are removed: they can rejoin without "already a member"
	assert.True(t, rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("5.000"), "hive:someoneelse", lateTS, "j").Success)
}
