package contract_test

// End-to-end lifecycle scenarios: complete DAOs driven through the real wasm +
// state-processing/ledger/RC engine. Each builds a DAO, runs a multi-step
// governance lifecycle, and asserts ledger balances + state at every step.

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"vsc-node/lib/test_utils"
)

const bigGas = uint(5_000_000_000)

// passAndExecuteAt votes YES from each voter, tallies, and executes at ts.
func passAndExecuteAt(t *testing.T, ct *test_utils.ContractTest, propID uint64, ts string, yesVoters ...string) test_utils.ContractTestCallResult {
	pfx := strconv.FormatUint(propID, 10)
	for i, v := range yesVoters {
		assert.True(t, voteRaw(ct, propID, v, "1", pfx+"y"+strconv.Itoa(i)).Success, "vote by %s failed", v)
	}
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", ts, pfx+"t")
	return rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", ts, pfx+"x")
}

// ============================================================================
// Scenario A — Community Grants DAO (democratic): full lifecycle
//
//	create → 4 members → grant payout → lower quorum → kick a member →
//	transfer ownership → verify at each step.
//
// ============================================================================
func TestE2E_DemocraticGrantsDAOLifecycle(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // democratic, owner hive:someone
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	joinProjectMember(t, ct, pid, "hive:outsider")
	addTreasuryFunds(t, ct, pid, "10.000")

	// --- P1: grant 3.000 HIVE to outsider (3 of 4 vote yes) ---
	grantee0 := hiveBal(ct, "hive:outsider")
	p1 := createPollProposal(t, ct, pid, "1", "hive:outsider:3.000:hive", "")
	exec := passAndExecuteAt(t, ct, p1, lateTS, "hive:someone", "hive:someoneelse", "hive:member2")
	assert.True(t, exec.Success, "grant proposal failed to execute: %s", exec.Ret)
	assert.Equal(t, grantee0+3000, hiveBal(ct, "hive:outsider"), "grantee did not receive 3.000 HIVE")

	// --- P2: lower quorum to 30% via governance ---
	p2 := createPollProposal(t, ct, pid, "1", "", "update_quorum=30.0")
	assert.True(t, passAndExecuteAt(t, ct, p2, lateTS, "hive:someone", "hive:someoneelse", "hive:member2").Success)

	// --- P3: kick member2; they are refunded their 1.000 stake and removed ---
	m2before := hiveBal(ct, "hive:member2")
	p3 := createPollProposal(t, ct, pid, "1", "", "kick_member=hive:member2")
	assert.True(t, passAndExecuteAt(t, ct, p3, lateTS, "hive:someone", "hive:someoneelse", "hive:outsider").Success)
	assert.Equal(t, m2before+1000, hiveBal(ct, "hive:member2"), "kicked member not refunded their stake")
	// removed: a fresh join must succeed (not "already a member")
	assert.True(t, rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:member2", lateTS, "rejoin").Success)

	// --- P4: transfer ownership someone -> someoneelse (direct owner op) ---
	assert.True(t, rawCallAt(ct, "project_transfer", PayloadString(fmt.Sprintf("%d|hive:someoneelse", pid)), nil, "hive:someone", lateTS, "xfer").Success)
	// new owner can pause; old owner cannot
	assert.False(t, rawCallAt(ct, "project_pause", PayloadUint64(pid), nil, "hive:someone", lateTS, "op").Success, "old owner still had privileges")
	assert.True(t, rawCallAt(ct, "project_pause", PayloadUint64(pid), nil, "hive:someoneelse", lateTS, "np").Success, "new owner could not pause")
}

// ============================================================================
// Scenario B — Stake-weighted Treasury DAO: whale dynamics + historical weight
//
//	whale alone can pass; a minority cannot; a departed whale is refunded.
//
// ============================================================================
func TestE2E_StakeWeightedTreasuryDAO(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1")           // stake, threshold 50%, low quorum
	joinWithStake(t, ct, pid, "hive:member2", "100.000")    // whale
	joinWithStake(t, ct, pid, "hive:someoneelse", "40.000") // minority
	addTreasuryFunds(t, ct, pid, "20.000")
	// total stake = 1 (someone) + 100 + 40 = 141

	// --- P1: whale alone (100/141 = 71% > 50%) grants 5.000 to outsider -> passes ---
	o0 := hiveBal(ct, "hive:outsider")
	p1 := createPollProposal(t, ct, pid, "1", "hive:outsider:5.000:hive", "")
	assert.True(t, passAndExecuteAt(t, ct, p1, lateTS, "hive:member2").Success)
	assert.Equal(t, o0+5000, hiveBal(ct, "hive:outsider"), "whale-passed grant did not pay out")

	// --- P2: minority alone (40/141 = 28% < 50%) grants 5.000 -> fails, no payout ---
	o1 := hiveBal(ct, "hive:outsider")
	p2 := createPollProposal(t, ct, pid, "1", "hive:outsider:5.000:hive", "")
	res := passAndExecuteAt(t, ct, p2, lateTS, "hive:someoneelse")
	assertAborts(t, res, "proposal is failed", "a 28%% minority passed a >50%% threshold")
	assert.Equal(t, o1, hiveBal(ct, "hive:outsider"), "minority vote paid out")

	// --- P3: the whale leaves and is refunded its full 100.000 stake ---
	w0 := hiveBal(ct, "hive:member2")
	rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:member2", lateTS, "l1")
	// second call past the cooldown finalizes the exit
	fin := rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:member2", "2025-09-10T00:00:00", "l2")
	assert.True(t, fin.Success, "whale could not finalize leave: %s", fin.Ret)
	assert.Equal(t, w0+100000, hiveBal(ct, "hive:member2"), "departed whale not refunded full stake")
}

// ============================================================================
// Scenario C — Whitelist-gated DAO: gate enforcement + governance whitelist mgmt
// ============================================================================
func TestE2E_WhitelistGatedDAO(t *testing.T) {
	ct := SetupContractTest()
	pid := createWhitelistProject(t, ct) // democratic, whitelistOnly, owner someone
	// owner whitelists two members directly
	assert.True(t, rawCallAt(ct, "project_whitelist_add", PayloadString(fmt.Sprintf("%d|hive:someoneelse;hive:member2", pid)), nil, "hive:someone", defaultTimestamp, "wl").Success)
	// whitelisted members can join; a non-whitelisted outsider cannot
	assert.True(t, rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:someoneelse", defaultTimestamp, "j1").Success)
	assert.True(t, rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:member2", defaultTimestamp, "j2").Success)
	assert.False(t, rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:outsider", defaultTimestamp, "j3").Success, "non-whitelisted outsider joined")

	// governance whitelists the outsider (whitelist_add via a passing proposal)
	p1 := createPollProposal(t, ct, pid, "1", "", "whitelist_add=hive:outsider")
	assert.True(t, passAndExecuteAt(t, ct, p1, lateTS, "hive:someone", "hive:someoneelse", "hive:member2").Success)
	// now the outsider can join
	assert.True(t, rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:outsider", lateTS, "j4").Success, "governance-whitelisted member could not join")
}

// ============================================================================
// Scenario D — Multi-asset + pause lifecycle
//
//	fund HIVE+HBD → pause → payout blocked → unpause via toggle_pause proposal →
//	multi-asset payout executes both legs.
//
// ============================================================================
func TestE2E_MultiAssetAndPauseLifecycle(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "5.000")                                                                                                                    // HIVE
	CallContract(t, ct, "project_funds", PayloadString(fmt.Sprintf("%d|false", pid)), transferIntentWithToken("5.000", "hbd"), "hive:someone", true, bigGas) // HBD

	// pause the project
	assert.True(t, rawCallAt(ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someone", defaultTimestamp, "pause").Success)
	// a payout proposal cannot be created while paused
	pf := []string{strconv.FormatUint(pid, 10), "p", "d", "1", "", "0", "hive:someoneelse:1.000:hive", "", ""}
	assert.False(t, rawCallAt(ct, "proposal_create", PayloadString(joinPipe(pf)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "blocked").Success, "payout proposal created while paused")

	// unpause via a toggle_pause proposal (created while paused, only-meta is allowed)
	up := createPollProposal(t, ct, pid, "1", "", "toggle_pause=1")
	assert.True(t, passAndExecuteAt(t, ct, up, lateTS, "hive:someone", "hive:someoneelse").Success)

	// now a multi-asset payout: 2 HIVE to someoneelse + 2 HBD to member2
	h0, d0 := hiveBal(ct, "hive:someoneelse"), hbdBal(ct, "hive:member2")
	mp := createPollProposal(t, ct, pid, "1", "hive:someoneelse:2.000:hive;hive:member2:2.000:hbd", "")
	assert.True(t, passAndExecuteAt(t, ct, mp, "2025-09-11T00:00:00", "hive:someone", "hive:someoneelse").Success)
	assert.Equal(t, h0+2000, hiveBal(ct, "hive:someoneelse"), "HIVE payout leg missing")
	assert.Equal(t, d0+2000, hbdBal(ct, "hive:member2"), "HBD payout leg missing")
}

// ============================================================================
// Scenario E — Execution delay + poll semantics
//
//	a passed action respects the execution delay; a poll never executes.
//
// ============================================================================
func TestE2E_ExecutionDelayAndPoll(t *testing.T) {
	ct := SetupContractTest()
	// execution delay = 24h; duration min 1h
	f := []string{"dao", "desc", "0", "50.001", "50.001", "1", "24", "10", "1", "1", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", true, bigGas)
	pid := parseCreatedID(t, res.Ret, "project")
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "5.000")

	// actionable payout, created at default ts (deadline +1h, executable +25h)
	p := createPollProposal(t, ct, pid, "1", "hive:someoneelse:1.000:hive", "")
	assert.True(t, voteRaw(ct, p, "hive:someone", "1", "e1").Success)
	assert.True(t, voteRaw(ct, p, "hive:someoneelse", "1", "e2").Success)
	// tally after the 1h voting window
	rawCallAt(ct, "proposal_tally", PayloadUint64(p), nil, "hive:someone", "2025-09-03T02:00:00", "t")
	// execute BEFORE the 24h delay -> rejected
	early := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", p)), nil, "hive:someone", "2025-09-03T05:00:00", "early")
	assertAborts(t, early, "execution delay until 2025-09-04T01:00:00Z", "proposal executed before its execution delay elapsed")
	// execute AFTER the delay -> succeeds
	before := hiveBal(ct, "hive:someoneelse")
	late := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", p)), nil, "hive:someone", "2025-09-05T00:00:00", "late")
	assert.True(t, late.Success, "proposal did not execute after its delay: %s", late.Ret)
	assert.Equal(t, before+1000, hiveBal(ct, "hive:someoneelse"))

	// a poll (custom options) with a payout rider never executes
	pollFields := []string{strconv.FormatUint(pid, 10), "poll", "d", "1", "red;green;blue", "0", "hive:someoneelse:1.000:hive", "", ""}
	pollID, ok := createProposalRaw(ct, pollFields, "hive:someone", "poll")
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, pollID, "hive:someone", "0", "pv1").Success)
	assert.True(t, voteRaw(ct, pollID, "hive:someoneelse", "0", "pv2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(pollID), nil, "hive:someone", "2025-09-06T00:00:00", "pt")
	pollExec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", pollID)), nil, "hive:someone", "2025-09-07T00:00:00", "px")
	assertAborts(t, pollExec, "proposal is closed", "a poll executed its payout rider")
}

// ============================================================================
// Scenario F — Autonomous governance: remove the owner, keep governing, re-own.
// ============================================================================
func TestE2E_AutonomousGovernanceLifecycle(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	addTreasuryFunds(t, ct, pid, "10.000")

	// P1: remove_owner -> project becomes autonomous
	p1 := createPollProposal(t, ct, pid, "1", "", "remove_owner=1")
	assert.True(t, passAndExecuteAt(t, ct, p1, lateTS, "hive:someone", "hive:someoneelse", "hive:member2").Success)
	// direct owner pause now fails (no owner)
	assert.False(t, rawCallAt(ct, "project_pause", PayloadUint64(pid), nil, "hive:someone", lateTS, "op").Success, "ex-owner paused an autonomous project")

	// P2: an autonomous DAO still governs its treasury (grant to outsider)
	o0 := hiveBal(ct, "hive:outsider")
	p2 := createPollProposal(t, ct, pid, "1", "hive:outsider:2.000:hive", "")
	assert.True(t, passAndExecuteAt(t, ct, p2, "2025-09-06T00:00:00", "hive:someone", "hive:someoneelse", "hive:member2").Success)
	assert.Equal(t, o0+2000, hiveBal(ct, "hive:outsider"), "autonomous DAO grant did not pay out")

	// P3: re-own via governance (someoneelse, a member)
	p3 := createPollProposal(t, ct, pid, "1", "", "update_owner=hive:someoneelse")
	assert.True(t, passAndExecuteAt(t, ct, p3, "2025-09-07T00:00:00", "hive:someone", "hive:someoneelse", "hive:member2").Success)
	// the new owner can now pause directly
	assert.True(t, rawCallAt(ct, "project_pause", PayloadUint64(pid), nil, "hive:someoneelse", "2025-09-07T01:00:00", "np").Success, "re-owned project has no working owner")
}

// ============================================================================
// Scenario G — Proposal cost economics: owner-cancel refunds the creator; a
// creator self-cancel does not.
// ============================================================================
func TestE2E_ProposalCostAndCancelRefund(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // cost = 1.000
	joinProjectMember(t, ct, pid, "hive:someoneelse")

	// someoneelse creates a proposal (pays 1.000 cost), then the OWNER cancels it
	// -> the creator is refunded, net cost 0.
	bal0 := hiveBal(ct, "hive:someoneelse")
	fieldsA := simpleProposalFields(pid, "1")
	a, okA := createProposalRaw2(ct, fieldsA, "hive:someoneelse", "ca")
	assert.True(t, okA)
	assert.Equal(t, bal0-1000, hiveBal(ct, "hive:someoneelse"), "proposal cost was not charged")
	assert.True(t, rawCallAt(ct, "proposal_cancel", PayloadUint64(a), nil, "hive:someone", defaultTimestamp, "ownercancel").Success)
	assert.Equal(t, bal0, hiveBal(ct, "hive:someoneelse"), "owner-cancel did not refund the creator")

	// someoneelse creates another and cancels it THEMSELVES -> no refund, down 1.000.
	bal1 := hiveBal(ct, "hive:someoneelse")
	fieldsB := simpleProposalFields(pid, "1")
	b, okB := createProposalRaw2(ct, fieldsB, "hive:someoneelse", "cb")
	assert.True(t, okB)
	assert.True(t, rawCallAt(ct, "proposal_cancel", PayloadUint64(b), nil, "hive:someoneelse", defaultTimestamp, "selfcancel").Success)
	assert.Equal(t, bal1-1000, hiveBal(ct, "hive:someoneelse"), "creator self-cancel wrongly refunded the cost")
}

// createProposalRaw2 is createProposalRaw with a payment intent (for cost DAOs).
func createProposalRaw2(ct *test_utils.ContractTest, fields []string, user, nonce string) (uint64, bool) {
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), user, defaultTimestamp, nonce)
	if !res.Success {
		return 0, false
	}
	id, err := strconv.ParseUint(trimMsg(res.Ret), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
