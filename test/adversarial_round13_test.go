package contract_test

// Round 13 — regressions for the findings of the five-agent independent review.
// Every test here was confirmed to FAIL against the code as it stood before its
// corresponding fix (see the commit message for the observed pre-fix values).

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"vsc-node/lib/test_utils"
)

// freeMembershipProject builds a DAO with proposalCost=0 AND stakeMin=0, i.e. one
// where every member legitimately holds Stake == 0.
func freeMembershipProject(t *testing.T, ct *test_utils.ContractTest) uint64 {
	t.Helper()
	f := defaultProjectFields()
	f[8] = "0" // proposalCost
	f[9] = "0" // stakeMin
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(strings.Join(f, "|")),
		transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

// joinFree adds a member to a zero-stake project (no transfer intent needed).
func joinFree(ct *test_utils.ContractTest, pid uint64, user, nonce string) test_utils.ContractTestCallResult {
	return rawCallAt(ct, "project_join", PayloadString(strconv.FormatUint(pid, 10)), nil, user, defaultTimestamp, nonce)
}

// R13-1 (CRITICAL): members who join in the SAME BLOCK as proposal creation, but
// after it, must not be able to vote.
//
// nowUnix() returns the block timestamp, identical for every tx in a block, so the
// old `member.JoinedAt > prpsl.CreatedAt` guard let them through while the tally
// denominators (MemberCountSnapshot/StakeSnapshot) still held pre-join values.
// Pre-fix this drained 50.000 HIVE with ZERO honest members voting: 5 sybils voting
// against a denominator of 4 yields 125%, clearing any threshold.
func TestBreak_SameBlockJoinCannotVote(t *testing.T) {
	ct := SetupContractTest()
	pid := freeMembershipProject(t, ct)
	for i, m := range []string{"hive:hon2", "hive:hon3", "hive:hon4"} {
		assert.True(t, joinFree(ct, pid, m, fmt.Sprintf("h%d", i)).Success)
	}
	addTreasuryFunds(t, ct, pid, "50.000")

	// 4 honest members exist; the snapshot is taken here.
	fields := []string{strconv.FormatUint(pid, 10), "drain", "d", "1", "", "0",
		"hive:atk1:50.000:hive", "", "", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok)
	before := hiveBal(ct, "hive:atk1")

	sybils := []string{"hive:atk1", "hive:atk2", "hive:atk3", "hive:atk4", "hive:atk5"}
	for i, m := range sybils {
		// Joining still succeeds — it is only VOTING that must be refused.
		assert.True(t, joinFree(ct, pid, m, fmt.Sprintf("s%d", i)).Success)
	}
	for i, m := range sybils {
		res := voteRaw(ct, propID, m, "1", fmt.Sprintf("v%d", i))
		assert.False(t, res.Success, "same-block joiner %s was allowed to vote", m)
		assert.Contains(t, res.Ret, "created before joining",
			"rejected for the wrong reason: %s", res.Ret)
	}
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.Equal(t, before, hiveBal(ct, "hive:atk1"), "sybil-only vote still moved treasury funds")
}

// R13-1b: the legitimate mirror — members who joined BEFORE the proposal (in the
// same block, sharing its timestamp) must still be able to vote. This is why the
// fix cannot simply be `>=`: it would disenfranchise everyone in the snapshot.
func TestBreak_SameBlockJoinBeforeProposalCanStillVote(t *testing.T) {
	ct := SetupContractTest()
	pid := freeMembershipProject(t, ct)
	assert.True(t, joinFree(ct, pid, "hive:hon2", "h1").Success)

	fields := []string{strconv.FormatUint(pid, 10), "poll", "d", "1", "", "0", "", "", "", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok)

	// Both joined before creation and share its block timestamp.
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success,
		"founding member was wrongly excluded from voting")
	assert.True(t, voteRaw(ct, propID, "hive:hon2", "1", "v2").Success,
		"member who joined before the proposal was wrongly excluded from voting")
}

// R13-2: a zero-stake member must be able to leave. sdk.HiveTransfer rejects a
// zero-value transfer ("amount must be positive"), so refunding unconditionally
// locked every member of a free-membership DAO in permanently.
func TestBreak_ZeroStakeMemberCanLeave(t *testing.T) {
	ct := SetupContractTest()
	pid := freeMembershipProject(t, ct)
	assert.True(t, joinFree(ct, pid, "hive:zed", "j").Success)
	p := PayloadString(strconv.FormatUint(pid, 10))

	arm := rawCallAt(ct, "project_leave", p, nil, "hive:zed", defaultTimestamp, "l1")
	assert.True(t, arm.Success)
	assert.Equal(t, "exit requested", arm.Ret)

	done := rawCallAt(ct, "project_leave", p, nil, "hive:zed", lateTS, "l2")
	assert.True(t, done.Success, "zero-stake member could not leave: %s", done.Ret)
	assert.Equal(t, "exit finished", done.Ret)
}

// R13-3: a passed proposal carrying kick_member against a ZERO-STAKE member must
// execute. Pre-fix the zero-value refund aborted the whole ExecuteProposal, so the
// proposal stayed ProposalPassed forever and any payout riding on it was stranded.
func TestBreak_KickZeroStakeMemberExecutes(t *testing.T) {
	ct := SetupContractTest()
	pid := freeMembershipProject(t, ct)
	assert.True(t, joinFree(ct, pid, "hive:someoneelse", "j1").Success)
	assert.True(t, joinFree(ct, pid, "hive:member2", "j2").Success)
	addTreasuryFunds(t, ct, pid, "10.000")

	fields := []string{strconv.FormatUint(pid, 10), "kick", "d", "1", "", "0",
		"hive:outsider:5.000:hive", "kick_member=hive:member2", "", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")

	before := hiveBal(ct, "hive:outsider")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.True(t, res.Success, "kicking a zero-stake member bricked execution: %s", res.Ret)
	assert.Equal(t, before+5000, hiveBal(ct, "hive:outsider"), "payout riding on the kick was stranded")
}

// R13-4 (HIGH): the project record must not be written back from a snapshot taken
// before the ICC's external call.
//
// The attacker passes a proposal carrying a payout (so the finance record is
// written) plus an ICC that re-enters the DAO as themselves and completes a
// project_leave. Pre-fix the trailing saveProjectFinance restored the stale
// StakeTotal/MemberCount over the nested frame's work: the attacker got their stake
// REFUNDED while the books still counted it as staked, permanently inflating the
// denominator every later proposal is measured against.
//
// Observable discriminator: after the attack the only remaining member holds 100%
// of the real stake, so their proposal must pass. With an inflated StakeTotal it
// measures 50% and fails the 50.001% threshold.
func TestBreak_ProjectRecordNotClobberedByICC(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createProjectWithVoting(t, ct, "1") // stake-weighted
	joinProjectMember(t, ct, pid, "hive:member2")
	addTreasuryFunds(t, ct, pid, "10.000")

	// member2 passes a proposal that pays out AND re-enters as themselves to leave.
	icc := fmt.Sprintf("%s|delegate|%s~project_leave::%d", MockID, ContractID, pid)
	fields := []string{strconv.FormatUint(pid, 10), "exit-via-icc", "d", "1", "", "0",
		"hive:outsider:0.001:hive", "", "", "", icc}
	propID, ok := createProposalRaw(ct, fields, "hive:member2", "c")
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:member2", "1", "v2").Success)
	// Arm the exit AFTER voting (casting a vote re-arms the leave cooldown).
	assert.True(t, rawCallAt(ct, "project_leave", PayloadString(strconv.FormatUint(pid, 10)), nil,
		"hive:member2", defaultTimestamp, "arm").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:member2", lateTS, "t")
	rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:member2", lateTS, "e")

	// Whatever the ICC did, the books must agree with it. Probe with a proposal from
	// the remaining member: it passes only if StakeTotal reflects reality.
	probe := []string{strconv.FormatUint(pid, 10), "probe", "d", "1", "", "0",
		"hive:outsider:0.001:hive", "", "", "", ""}
	probeID, ok := createProposalRaw(ct, probe, "hive:someone", "c2")
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, probeID, "hive:someone", "1", "pv").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(probeID), nil, "hive:someone", lateTS, "t2")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", probeID)), nil, "hive:someone", lateTS, "e2")
	assert.True(t, res.Success,
		"governance is stuck: StakeTotal/MemberCount were clobbered by the stale "+
			"post-ICC write, inflating the tally denominator (%s)", res.Ret)
}

// R13-5: an address that is not a routable hive:/did: identity must be refused.
// Pre-fix "notanaddress" was accepted and irreversibly PAID: the ledger credits a
// literal account of that name.
func TestBreak_MalformedPayoutAddressRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	addTreasuryFunds(t, ct, pid, "10.000")
	for _, bad := range []string{"notanaddress", "0", ":::", "hive:", "did:"} {
		fields := []string{strconv.FormatUint(pid, 10), "pay", "d", "1", "", "0",
			bad + ":1.000:hive", "", "", "", ""}
		_, ok := createProposalRaw(ct, fields, "hive:someone", "c"+bad)
		assert.False(t, ok, "payout to malformed address %q was accepted", bad)
	}
}

// R13-6: a missing entity id must abort, not silently mean entity 0. Pre-fix "|1"
// cast a real ballot on proposal 0.
func TestBreak_EmptyEntityIDRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	propID := createPollProposal(t, ct, pid, "1", "", "")
	assert.Equal(t, uint64(0), propID, "test assumes the target is proposal 0")

	vote := rawCallAt(ct, "proposals_vote", PayloadString("|1"), nil, "hive:someone", defaultTimestamp, "v")
	assert.False(t, vote.Success, "empty proposal id silently voted on proposal 0")

	funds := rawCallAt(ct, "project_funds", PayloadString("|false"), transferIntent("1.000"),
		"hive:someone", defaultTimestamp, "f")
	assert.False(t, funds.Success, "empty project id silently deposited into project 0")
}

// R13-7: numeric fields must reject Go literal syntax that stores a value other
// than the text shown. Pre-fix "1_0" was stored as 10 and "0x1p+6" as 64.
func TestBreak_NumericLiteralSyntaxRejected(t *testing.T) {
	ct := SetupContractTest()
	for _, bad := range []string{"1_0", "0x1p+6", "0b11"} {
		f := defaultProjectFields()
		f[3] = bad // threshold
		res := rawCallAt(ct, "project_create", PayloadString(strings.Join(f, "|")),
			transferIntent("1.000"), "hive:someone", defaultTimestamp, "c"+bad)
		assert.False(t, res.Success, "threshold %q was accepted", bad)
	}
}

// R13-8: a proposal's voting period is capped at MaxProposalDurationHours. The
// voting period is exactly how long one member can freeze another's stake via a
// payout lock, so the old 10-year MaxDurationHours was far too permissive.
func TestBreak_ProposalDurationCapped(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := []string{strconv.FormatUint(pid, 10), "long", "d", "87600", "", "0", "", "", "", "", ""}
	_, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.False(t, ok, "a 10-year proposal duration was accepted")

	within := []string{strconv.FormatUint(pid, 10), "ok", "d", "2160", "", "0", "", "", "", "", ""}
	_, ok = createProposalRaw(ct, within, "hive:someone", "c2")
	assert.True(t, ok, "a proposal at exactly the cap was rejected")
}

// R13-9: closes a false-pass found by the mutation audit.
//
// TestBreak_CancelRefundsAmountPaidNotCurrentCost is titled for this rule but
// creates the proposal at cost 0, so the `prpsl.CostPaid > 0` guard short-circuits
// and the refund AMOUNT is never evaluated — mutating it survived the whole suite.
// This covers the dangerous direction: created at 1.000, governance raises the cost
// to 5.000, owner cancels. The refund must be the 1.000 actually paid; refunding
// the CURRENT cost would mint 4.000 out of the treasury.
func TestBreak_CancelRefundsOriginalCostAfterRaise(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // proposalCost = 1.000
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "20.000")

	// someoneelse creates a proposal, paying the CURRENT cost of 1.000.
	victimFields := []string{strconv.FormatUint(pid, 10), "mine", "d", "1", "", "0", "", "", "", "", ""}
	target, ok := createProposalRaw(ct, victimFields, "hive:someoneelse", "t")
	assert.True(t, ok)

	// Governance raises the proposal cost to 5.000.
	raise := createPollProposal(t, ct, pid, "1", "", "update_proposalCost=5.0")
	assert.True(t, voteRaw(ct, raise, "hive:someone", "1", "r1").Success)
	assert.True(t, voteRaw(ct, raise, "hive:someoneelse", "1", "r2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(raise), nil, "hive:someone", lateTS, "rt")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", raise)),
		nil, "hive:someone", lateTS, "rx").Success)

	// Owner cancels: refund must be exactly what was paid, not the new cost.
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_cancel", PayloadUint64(target), nil, "hive:someone", lateTS, "c").Success)
	assert.Equal(t, before+1000, hiveBal(ct, "hive:someoneelse"),
		"cancel refunded the CURRENT proposal cost (5.000) instead of the 1.000 actually paid")
}

// R13-10: the previously-unbounded list fields are now capped. Each was verified
// to be accepted before the cap (200 ICC entries and 1000 ballot choices both
// persisted into state for a single fee).
func TestBreak_ListFieldsAreBounded(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")

	// >MaxICCCalls inter-contract calls
	var icc []string
	for i := 0; i < 25; i++ {
		icc = append(icc, fmt.Sprintf("%s|noop|x", MockID))
	}
	fields := []string{strconv.FormatUint(pid, 10), "many-icc", "d", "1", "", "0", "", "", "", "",
		strings.Join(icc, ";")}
	_, ok := createProposalRaw(ct, fields, "hive:someone", "icc")
	assert.False(t, ok, "an unbounded ICC list was accepted")

	// >MaxProposalOptions ballot choices
	propID := createPollProposal(t, ct, pid, "1", "", "")
	var choices []string
	for i := 0; i < 1000; i++ {
		choices = append(choices, "1")
	}
	res := voteRaw(ct, propID, "hive:someone", strings.Join(choices, ","), "v")
	assert.False(t, res.Success, "a 1000-choice ballot was accepted")

	// oversize outcome-meta blob
	big := []string{strconv.FormatUint(pid, 10), "big-meta", "d", "1", "", "0", "",
		"update_url=" + strings.Repeat("x", 9000), "", "", ""}
	_, ok = createProposalRaw(ct, big, "hive:someone", "meta")
	assert.False(t, ok, "an oversize outcome-meta blob was accepted")
}
