package contract_test

// Round 11 — regression tests for the fixes made after the independent review
// (4 external reviewers). Covers the testable subset; the reentrancy ordering,
// the confused-deputy guard and the ICC intent-arg fix cannot be exercised in a
// single-node harness (they need a hostile/real callee contract) and are
// verified by construction + the devnet.

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// R11-1: the owner must NOT be able to cancel the members' escape hatch.
// Otherwise: owner pauses (blocking all exits), then cancels every toggle_pause /
// remove_owner proposal on sight => member stake is confiscated permanently.
func TestBreak_OwnerCannotCancelPauseEscapeProposal(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// a member proposes the escape hatch
	fields := []string{strconv.FormatUint(pid, 10), "unfreeze", "d", "1", "", "0", "", "toggle_pause=1", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someoneelse", "esc")
	assert.True(t, ok, "escape proposal could not be created")
	// the owner must not be able to veto it
	res := rawCallAt(ct, "proposal_cancel", PayloadUint64(propID), nil, "hive:someone", defaultTimestamp, "veto")
	assertAborts(t, res, "owner cannot cancel a pause/ownership recovery proposal", "owner vetoed the pause/ownership recovery proposal")
	// its own creator may still withdraw it
	own := rawCallAt(ct, "proposal_cancel", PayloadUint64(propID), nil, "hive:someoneelse", defaultTimestamp, "self")
	assert.True(t, own.Success, "creator could not cancel their own proposal: %s", own.Ret)
}

// R11-2: a cancel refund must return what was ACTUALLY paid, not the currently
// configured cost. Exploit: create proposals while cost is 0, raise the cost via
// governance, then have the owner cancel them and mint the difference.
func TestBreak_CancelRefundsAmountPaidNotCurrentCost(t *testing.T) {
	ct := SetupContractTest()
	// project with proposal cost 0
	f := []string{"dao", "desc", "0", "50.001", "50.001", "1", "0", "10", "0", "1", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	pid := parseCreatedID(t, res.Ret, "project")
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "10.000")

	// someoneelse creates a FREE proposal (cost 0 => CostPaid 0)
	free := []string{strconv.FormatUint(pid, 10), "free", "d", "1", "", "0", "", "", ""}
	freeID, ok := createProposalRaw(ct, free, "hive:someoneelse", "free")
	assert.True(t, ok)

	// governance raises the cost to 5.000
	raise := createPollProposal(t, ct, pid, "1", "", "update_proposalCost=5.0")
	assert.True(t, voteRaw(ct, raise, "hive:someone", "1", "r1").Success)
	assert.True(t, voteRaw(ct, raise, "hive:someoneelse", "1", "r2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(raise), nil, "hive:someone", lateTS, "rt")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", raise)), nil, "hive:someone", lateTS, "rx").Success)

	// owner cancels the free proposal — must refund NOTHING (it cost nothing)
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_cancel", PayloadUint64(freeID), nil, "hive:someone", lateTS, "c").Success)
	assert.Equal(t, before, hiveBal(ct, "hive:someoneelse"),
		"cancel refunded the CURRENT cost for a proposal that was created for free")
}

// R11-3: voting must re-arm the leave cooldown. A pre-armed exit request would
// otherwise let a member vote and withdraw in the very next call (vote-and-run).
func TestBreak_VotingReArmsLeaveCooldown(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // leaveCooldown = 10h
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// pre-arm the exit
	assert.True(t, rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", defaultTimestamp, "arm").Success)
	// ... cooldown lapses, then the member votes on a live proposal
	propID := createSimpleProposal(t, ct, pid, "1")
	assert.True(t, rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", propID)), nil, "hive:someoneelse", lateTS, "v").Success)
	// the next leave must NOT finalize (the vote re-armed the cooldown)
	before := hiveBal(ct, "hive:someoneelse")
	rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", lateTS, "exit")
	assert.Equal(t, before, hiveBal(ct, "hive:someoneelse"),
		"member withdrew stake immediately after voting (vote-and-run)")
}

// R11-4: a stake top-up landing in the SAME block as a proposal's creation must
// not count toward that proposal — otherwise a voter's weight can exceed 100% of
// the (pre-top-up) threshold denominator.
func TestBreak_SameBlockTopUpDoesNotCountForThatProposal(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1") // stake, threshold 50%, low quorum
	joinWithStake(t, ct, pid, "hive:someoneelse", "40.000")
	joinWithStake(t, ct, pid, "hive:member2", "60.000") // silent; total 101
	addTreasuryFunds(t, ct, pid, "5.000")
	// proposal created at defaultTimestamp
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:2.000:hive", "")
	// top-up in the SAME block (same timestamp) — must not boost weight for propID
	CallContract(t, ct, "project_funds", PayloadString(fmt.Sprintf("%d|true", pid)), transferIntent("100.000"), "hive:someoneelse", true, uint(1_000_000_000))
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assertAborts(t, exec, "proposal is failed", "same-block top-up pushed a 40/101 voter over a 50%% threshold")
	assert.Equal(t, before, after, "same-block top-up bought a payout")
}

// R11-5: a one-character quote in the meta field must abort cleanly, not trap.
func TestBreak_SingleQuoteMetaDoesNotTrap(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	for _, q := range []string{"\"", "'"} {
		res := rawCallAt(ct, "proposal_create",
			PayloadString(fmt.Sprintf("%d|p|d|1||0||%s|", pid, q)),
			transferIntent("1.000"), "hive:someone", defaultTimestamp, "q"+q)
		assertAborts(t, res, "invalid metadata entry (use key=value)", "single-quote meta %q was accepted", q)
	}
}

// R11-6: a vote choice at/above 2^32 must be rejected, not silently truncated to
// option 0 (uint is 32-bit on the wasm target).
func TestBreak_HugeVoteChoiceRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	propID := createSimpleProposal(t, ct, pid, "1")
	for _, choice := range []string{"4294967296", "4294967297", "18446744073709551615"} {
		res := rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|%s", propID, choice)), nil, "hive:someone", defaultTimestamp, "c"+choice)
		assertAborts(t, res, "invalid option index", "vote choice %s was accepted (32-bit truncation)", choice)
	}
}

// R11-7: contract_init must reject an unrecognized permission mode rather than
// silently (and permanently) degrading to owner-only.
func TestBreak_ContractInitRejectsUnknownMode(t *testing.T) {
	for _, mode := range []string{"Public", "pub", "owner_only", "", "PUBLIC"} {
		ct := SetupContractTestUninitialized()
		res := rawCallAt(ct, "contract_init", PayloadString(mode), nil, "hive:someone", defaultTimestamp, "i")
		assertAborts(t, res, "permission mode must be exactly", "contract_init accepted bogus mode %q", mode)
	}
}

// R11-8: the two valid modes still work (positive control for R11-7).
func TestBreak_ContractInitAcceptsValidModes(t *testing.T) {
	for _, mode := range []string{"public", "owner-only"} {
		ct := SetupContractTestUninitialized()
		res := rawCallAt(ct, "contract_init", PayloadString(mode), nil, "hive:someone", defaultTimestamp, "i")
		assert.True(t, res.Success, "contract_init rejected valid mode %q: %s", mode, res.Ret)
	}
}
