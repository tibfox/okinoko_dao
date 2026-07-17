package contract_test

// adversarial_test.go — attempts to BREAK the Okinoko DAO contract.
//
// Each test asserts the behaviour a correct contract SHOULD exhibit. A failing
// test therefore documents a real logic bug (not a broken test). Tests are
// grouped:
//   BREAK_*  — probes a suspected logic bug; failure = bug confirmed.
//   GUARD_*  — verifies the hardening added in this branch actually fires.
//   BOUND_*  — boundary / robustness checks that should hold.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"vsc-node/lib/test_utils"
	"vsc-node/modules/db/vsc/contracts"
	ledgerDb "vsc-node/modules/db/vsc/ledger"
	stateEngine "vsc-node/modules/state-processing"

	"github.com/stretchr/testify/assert"
)

const lateTS = "2025-09-05T00:00:00" // well past a 1h proposal deadline

// rawCallAt invokes the contract WITHOUT the success/failure assertion baked
// into CallContract, so a test can inspect the raw outcome itself. The result's
// Ret is normalized to carry the abort message on failure (the host now returns
// it in ErrMsg), so existing `res.Ret` substring assertions keep working.
func rawCallAt(ct *test_utils.ContractTest, action string, payload []byte, intents []contracts.Intent, authUser, timestamp, nonce string) test_utils.ContractTestCallResult {
	if timestamp == "" {
		timestamp = defaultTimestamp
	}
	result := ct.Call(stateEngine.TxVscCallContract{
		Caller: authUser,
		Self: stateEngine.TxSelf{
			TxId:                 fmt.Sprintf("%s-%s-%s-tx", action, authUser, nonce),
			BlockId:              "block1",
			Index:                0,
			OpIndex:              0,
			Timestamp:            timestamp,
			RequiredAuths:        []string{authUser},
			RequiredPostingAuths: []string{},
		},
		ContractId: ContractID,
		Action:     action,
		Payload:    payload,
		RcLimit:    100000,
		Intents:    intents,
	})
	if !result.Success && result.Ret == "" {
		result.Ret = result.ErrMsg
	}
	return result
}

// joinWithStake joins `user` into a stake-voting project with a custom deposit.
func joinWithStake(t *testing.T, ct *test_utils.ContractTest, projectID uint64, user, amount string) {
	CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent(amount), user, true, uint(1_000_000_000))
}

// createStakeProject spins up a stake-weighted DAO (voting mode "1").
func createStakeProject(t *testing.T, ct *test_utils.ContractTest) uint64 {
	return createProjectWithVoting(t, ct, "1")
}

// voteRaw casts a vote for arbitrary choice string and returns raw success.
func voteRaw(ct *test_utils.ContractTest, proposalID uint64, user, choices, nonce string) test_utils.ContractTestCallResult {
	return rawCallAt(ct, "proposals_vote", []byte(strconv.Quote(fmt.Sprintf("%d|%s", proposalID, choices))), nil, user, defaultTimestamp, nonce)
}

// hiveBal is a convenience wrapper.
func hiveBal(ct *test_utils.ContractTest, acct string) int64 {
	return ct.GetBalance(acct, ledgerDb.AssetHive)
}

// ============================================================================
// GROUP 1 — Voting / tally correctness
// ============================================================================

// BREAK 1: A proposal that members vote DOWN ("no" = option 0) must not pay out.
// The execute path never checks which option won, so a "no" landslide still runs
// the attached payout. CRITICAL.
func TestBreak_NoOptionWinningStillPaysOut(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // democratic, threshold/quorum 50.001%
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")

	// Both members vote NO (option index 0).
	assert.True(t, voteRaw(ct, propID, "hive:someone", "0", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "0", "v2").Success)

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")

	assert.False(t, exec.Success, "a down-voted (NO) proposal must not be executable")
	assert.Equal(t, before, after, "treasury paid out even though NO won the vote")
}

// BREAK 2: Quorum must count distinct voters, not per-option participations.
// A single member selecting multiple options should not satisfy a 2-voter quorum.
func TestBreak_QuorumInflationViaMultiSelect(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)                 // creator hive:someone stake 1
	joinWithStake(t, ct, pid, "hive:member2", "100.000")   // whale
	joinWithStake(t, ct, pid, "hive:someoneelse", "1.000") // 3 members total
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:member2:0.500:hive", "")

	// ONLY the whale votes, but selects BOTH options -> voterCount inflated to 2.
	assert.True(t, voteRaw(ct, propID, "hive:member2", "0,1", "v").Success)

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:member2")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:member2")

	assert.False(t, exec.Success, "quorum (2 of 3) met by a SINGLE multi-selecting voter")
	assert.Equal(t, before, after, "payout executed on inflated quorum")
}

// BREAK 3: Free-membership DAOs should still be governable. Members have stake 0,
// and VoteProposal aborts on weight==0, making the whole DAO unusable.
func TestBreak_FreeMembershipDaoCanVote(t *testing.T) {
	ct := SetupContractTest()
	pid := createFreeMembershipProject(t, ct)
	// free membership => no intent needed
	CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(pid, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000))
	propID := createSimpleProposal(t, ct, pid, "1")

	res := voteRaw(ct, propID, "hive:someone", "1", "v")
	assert.True(t, res.Success, "member of a free DAO cannot vote (governance is dead): %s", res.Ret)
}

// BREAK 4: Negative payout amounts must be rejected (ICC path rejects them; the
// payout path does not). A negative amount corrupts treasury accounting.
func TestBreak_NegativePayoutRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	addTreasuryFunds(t, ct, pid, "2.000")
	fields := []string{
		strconv.FormatUint(pid, 10), "evil", "drain", "1", "", "0",
		"hive:someoneelse:-100:hive", "", "",
	}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assert.False(t, res.Success, "proposal with a NEGATIVE payout amount was accepted")
}

// BREAK 5: A proposal with zero votes must fail and never execute.
func TestBreak_ZeroVotesFailsAndCannotExecute(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, exec.Success, "proposal with zero votes must not be executable")
}

// BREAK 6: Re-voting the SAME option must not accumulate weight. Isolated from the
// "no-wins-executes" bug: here YES is the winner, but a single voter's stake is
// below threshold, so the payout may only fire if re-voting doubled their weight.
func TestBreak_ReVoteSameOptionDoesNotDoubleWeight(t *testing.T) {
	ct := SetupContractTest()
	// stake voting, threshold 50%, LOW quorum so one voter satisfies it.
	fields := []string{"dao", "desc", "1", "50.000", "1", "1", "0", "10", "1", "1", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	pid := parseCreatedID(t, res.Ret, "project")
	joinWithStake(t, ct, pid, "hive:someoneelse", "40.000") // voter, 40 stake
	joinWithStake(t, ct, pid, "hive:member2", "60.000")     // silent, inflates total to 101
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")

	// someoneelse (40 of 101 = 39.6% < 50%) votes YES twice.
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "a").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "b").Success)

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assert.False(t, exec.Success, "re-voting doubled a sub-threshold voter over the line")
	assert.Equal(t, before, after, "re-vote accumulated weight and paid out")
}

// BREAK 7: An empty-choice vote must not count toward quorum.
func TestBreak_EmptyChoiceVoteDoesNotCountQuorum(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")

	// One real YES vote, plus an empty ballot. Quorum needs 2 of 2 members.
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	voteRaw(ct, propID, "hive:someoneelse", "", "v2") // empty choices

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assert.False(t, exec.Success, "empty ballot counted toward quorum")
	assert.Equal(t, before, after, "payout executed with only one real voter")
}

// BREAK 8: Voting for an out-of-range option index must abort.
func TestBreak_VoteInvalidOptionIndex(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	propID := createSimpleProposal(t, ct, pid, "1")
	res := voteRaw(ct, propID, "hive:someone", "999", "v")
	assert.False(t, res.Success, "vote for nonexistent option index was accepted")
}

// BREAK 9: A member who already left cannot vote.
func TestBreak_VoteAfterLeaveRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createSimpleProposal(t, ct, pid, "1")
	// leave requires two calls past cooldown (10h). Use late timestamps.
	CallContractAt(t, ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", true, uint(1_000_000_000), defaultTimestamp)
	CallContractAt(t, ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", true, uint(1_000_000_000), lateTS)
	res := rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", propID)), nil, "hive:someoneelse", lateTS, "v")
	assert.False(t, res.Success, "ex-member was allowed to vote")
}

// ============================================================================
// GROUP 2 — Config / meta bounds and timing overflow
// ============================================================================

// BREAK 10: A proposal duration near uint64 max overflows the deadline math and
// could make a proposal tallyable immediately.
func TestBreak_ProposalDurationOverflow(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := simpleProposalFields(pid, "18446744073709551615") // max uint64 hours
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	if !res.Success {
		return // rejecting the absurd duration is acceptable
	}
	propID, perr := strconv.ParseUint(trimMsg(res.Ret), 10, 64)
	assert.NoError(t, perr)
	// Deadline should be far in the future; an immediate tally must be rejected.
	tally := rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", defaultTimestamp, "t")
	assert.False(t, tally.Success, "duration overflow let a proposal tally immediately")
}

// BREAK 11: Setting leave-cooldown to an absurd value via governance must not be
// able to permanently trap members' stake. Either the update is bounded, or the
// member can still eventually exit.
func TestBreak_LeaveCooldownOverflowTrapsStake(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// Pass a meta proposal raising leave cooldown to max uint64.
	propID := createPollProposal(t, ct, pid, "1", "", "update_leaveCooldown=18446744073709551615")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	execRes := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	if !execRes.Success {
		return // rejecting the update is acceptable
	}
	// Cooldown*3600 overflows int64; check a member can still exit some finite time later.
	rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", lateTS, "l1")
	res := rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", "2099-01-01T00:00:00", "l2")
	assert.True(t, res.Success, "leave-cooldown overflow permanently trapped a member's stake")
}

// ============================================================================
// GROUP 3 — Membership / treasury / accounting
// ============================================================================

// BREAK 12: AddFunds should respect pause like Join/Leave do.
func TestBreak_AddFundsWhilePausedBlocked(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)
	CallContract(t, ct, "project_pause", PayloadUint64(pid), nil, "hive:someone", true, uint(1_000_000_000))
	res := rawCallAt(ct, "project_funds", PayloadString(fmt.Sprintf("%d|true", pid)), transferIntent("5.000"), "hive:someone", defaultTimestamp, "f")
	assert.False(t, res.Success, "AddFunds succeeded while project was paused (Join/Leave are blocked)")
}

// BREAK 13: A member who votes then leaves (fully refunded) should not still swing
// a passed payout with capital they no longer have locked.
func TestBreak_VoteThenLeaveWeightRetained(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)
	joinWithStake(t, ct, pid, "hive:member2", "100.000") // whale swings the vote
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:0.500:hive", "")

	assert.True(t, voteRaw(ct, propID, "hive:member2", "1", "v").Success) // whale votes YES
	// Whale leaves and is refunded BEFORE tally.
	rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:member2", defaultTimestamp, "l1")
	rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:member2", lateTS, "l2")

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:outsider")
	assert.Equal(t, before, after, "a departed+refunded voter's weight still passed the payout")
}

// BREAK 14: Multi-asset payout must be atomic. If the 2nd asset is short, the 1st
// must be rolled back (no partial payout).
func TestBreak_MultiAssetPayoutAtomicRollback(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000") // hive only, no hbd
	// Payout: 0.5 hive (ok) + 5 hbd (treasury has 0 hbd -> must fail whole tx).
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive;hive:someoneelse:5.000:hbd", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")

	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assert.False(t, exec.Success, "payout with an unfunded 2nd asset should abort")
	assert.Equal(t, before, after, "first asset paid out despite the whole payout aborting")
}

// BREAK 15: Kicking a non-member must be a silent no-op, not corrupt member count.
func TestBreak_KickNonMemberNoop(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "1", "", "kick_member=hive:ghost")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.True(t, exec.Success, "kicking a non-member should be a harmless no-op: %s", exec.Ret)
}

// ============================================================================
// GROUP 4 — Hardening guards added in this branch (should PASS)
// ============================================================================

// longAddr returns a syntactically-plausible address that exceeds MaxAddressLength
// (128). Unlike delimiter chars, an over-length address survives payload parsing
// and actually reaches validateAddress, so it is a sound probe for the guard.
func longAddr() string { return "hive:" + strings.Repeat("a", 200) }

// GUARD: an over-length whitelist address is rejected by validateAddress.
func TestGuard_WhitelistOverlongAddressRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createWhitelistProject(t, ct)
	res := rawCallAt(ct, "project_whitelist_add", PayloadString(fmt.Sprintf("%d|%s", pid, longAddr())), nil, "hive:someone", defaultTimestamp, "w")
	assert.False(t, res.Success, "whitelist accepted an over-length address")
}

// GUARD: an over-length payout address is rejected at proposal creation.
func TestGuard_PayoutOverlongAddressRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := []string{strconv.FormatUint(pid, 10), "p", "d", "1", "", "0", longAddr() + ":1.000:hive", "", ""}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assert.False(t, res.Success, "payout accepted an over-length address")
}

// GUARD: an internal space (a byte <= 0x20 that survives payload splitting) in an
// address is rejected by validateAddress.
func TestGuard_WhitelistSpaceAddressRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createWhitelistProject(t, ct)
	res := rawCallAt(ct, "project_whitelist_add", PayloadString(fmt.Sprintf("%d|hive:al ice", pid)), nil, "hive:someone", defaultTimestamp, "w")
	assert.False(t, res.Success, "whitelist accepted an address with an internal space")
}

// GUARD: transferring ownership to a malformed address is rejected.
func TestGuard_TransferOwnershipBadAddressRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	res := rawCallAt(ct, "project_transfer", PayloadString(fmt.Sprintf("%d|hive:a;b", pid)), nil, "hive:someone", defaultTimestamp, "x")
	assert.False(t, res.Success, "ownership transfer accepted an address containing ';'")
}

// GUARD: an astronomically large deposit is rejected (FloatToAmount range guard),
// not silently wrapped into a negative stake.
func TestGuard_OverflowDepositRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntentWithToken("100000000000000000000", "hive"), "hive:member2", defaultTimestamp, "j")
	assert.False(t, res.Success, "an overflowing deposit was accepted")
}

// GUARD: two members whose addresses differ only in length keep independent stake
// histories (would have collided under the old key layout).
func TestGuard_StakeHistoryNoCollisionByAddressLength(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)
	// hive:someone (creator) vs hive:someoneelse (superstring) — differing lengths.
	joinWithStake(t, ct, pid, "hive:someoneelse", "7.000")
	// Increase someoneelse's stake several times to grow the history keyspace.
	CallContract(t, ct, "project_funds", PayloadString(fmt.Sprintf("%d|true", pid)), transferIntent("3.000"), "hive:someoneelse", true, uint(1_000_000_000))
	// A proposal snapshots stake; both members must vote with their own weights.
	propID := createPollProposal(t, ct, pid, "1", "", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success, "stake-history key collision blocked a member's vote")
}

// ============================================================================
// GROUP 5 — Boundary / robustness
// ============================================================================

// BOUND: tallying twice must reject the second attempt.
func TestBound_DoubleTallyRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createSimpleProposal(t, ct, pid, "1")
	voteRaw(ct, propID, "hive:someone", "1", "v1")
	voteRaw(ct, propID, "hive:someoneelse", "1", "v2")
	first := rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t1")
	assert.True(t, first.Success)
	second := rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t2")
	assert.False(t, second.Success, "a proposal was tallied twice")
}

// BOUND: tallying before the deadline must reject.
func TestBound_TallyBeforeDeadlineRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	propID := createSimpleProposal(t, ct, pid, "1")
	res := rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", defaultTimestamp, "t")
	assert.False(t, res.Success, "proposal tallied before its deadline")
}

// BOUND: executing a still-active (untallied) proposal must reject.
func TestBound_ExecuteWithoutTallyRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, res.Success, "an un-tallied proposal was executed")
}

// BOUND: a non-owner cannot pause the project.
func TestBound_NonOwnerCannotPause(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	res := rawCallAt(ct, "project_pause", PayloadUint64(pid), nil, "hive:someoneelse", defaultTimestamp, "p")
	assert.False(t, res.Success, "a non-owner paused the project")
}

// BOUND: joining twice must reject the second attempt.
func TestBound_DoubleJoinRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:someoneelse", defaultTimestamp, "j2")
	assert.False(t, res.Success, "a member joined the same project twice")
}

// BOUND: cancelling an already-tallied proposal must reject.
func TestBound_CancelAfterTallyRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	propID := createSimpleProposal(t, ct, pid, "1")
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_cancel", PayloadUint64(propID), nil, "hive:someone", lateTS, "c")
	assert.False(t, res.Success, "a tallied proposal was cancelled")
}

// BOUND: a non-member cannot create a proposal in a members-only project.
func TestBound_NonMemberCannotCreateProposal(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // default ProposalsMembersOnly = true
	fields := simpleProposalFields(pid, "1")
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:outsider", defaultTimestamp, "c")
	assert.False(t, res.Success, "a non-member created a proposal in a members-only DAO")
}

// BOUND: a member with active payout lock cannot leave (griefing-adjacent but
// documents the intended lock).
func TestBound_PayoutTargetCannotLeaveDuringVote(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// proposal names someoneelse as payout target; lock is set at creation.
	createPollProposal(t, ct, pid, "5", "hive:someoneelse:0.500:hive", "")
	res := rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", defaultTimestamp, "l")
	assert.False(t, res.Success, "payout-target left while a payout was pending")
}

// BOUND: owner cannot leave without transferring ownership first.
func TestBound_OwnerCannotLeave(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	res := rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someone", defaultTimestamp, "l")
	assert.False(t, res.Success, "owner left without transferring ownership")
}

// BOUND: proposal cost must actually be charged (intent below cost rejected).
func TestBound_ProposalCostEnforced(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // cost = 1
	fields := simpleProposalFields(pid, "1")
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("0.500"), "hive:someone", defaultTimestamp, "c")
	assert.False(t, res.Success, "proposal created with an intent below the required cost")
}

// BOUND: stake-voting DAO must reject proposal creation when total stake is zero
// is unreachable (creator always stakes), but a democratic exact-stake join must
// reject an over-payment.
func TestBound_DemocraticJoinRejectsOverpay(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // democratic, stakeMin 1
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("5.000"), "hive:someoneelse", defaultTimestamp, "j")
	assert.False(t, res.Success, "democratic DAO accepted a deposit != stakeMin")
}

// BOUND: whitelist gating does not waive the stake deposit.
func TestBound_WhitelistStillRequiresStake(t *testing.T) {
	ct := SetupContractTest()
	pid := createWhitelistProject(t, ct) // democratic, stakeMin 1, whitelistOnly
	CallContract(t, ct, "project_whitelist_add", PayloadString(fmt.Sprintf("%d|hive:someoneelse", pid)), nil, "hive:someone", true, uint(1_000_000_000))
	// Join with NO intent should fail even though whitelisted.
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), nil, "hive:someoneelse", defaultTimestamp, "j")
	assert.False(t, res.Success, "whitelisted member bypassed the stake deposit")
}

// ---------------------------------------------------------------------------
// small local helpers
// ---------------------------------------------------------------------------

func joinPipe(fields []string) string {
	out := fields[0]
	for _, f := range fields[1:] {
		out += "|" + f
	}
	return out
}

func trimMsg(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	if len(s) >= 4 && s[:4] == "msg:" {
		s = s[4:]
	}
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 {
		last := s[len(s)-1]
		if last == ' ' || last == '\n' || last == '\t' {
			s = s[:len(s)-1]
		} else {
			break
		}
	}
	return s
}
