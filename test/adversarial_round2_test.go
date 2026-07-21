package contract_test

// Round 2 adversarial tests — timing, quorum edges, cancel/refund, polls,
// democratic math, ICC auth. Same convention: a failing test = a real bug.

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"vsc-node/lib/test_utils"
)

// makeProject builds a project with explicit voting/threshold/quorum. stakeMin is
// fixed at 1.0 (the creator deposits exactly that), matching the ".000" decimal
// form the host's intent limits expect.
func makeProject(t *testing.T, ct *test_utils.ContractTest, voting, threshold, quorum string) uint64 {
	f := []string{"dao", "desc", voting, threshold, quorum, "1", "0", "10", "1", "1", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

// createProposalRaw creates a proposal from explicit fields, returns raw result.
func createProposalRaw(ct *test_utils.ContractTest, fields []string, user, nonce string) (uint64, bool) {
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

// R2-1: empty ballots must not count toward quorum (a member who selects nothing
// is not participating in the decision).
func TestBreak_EmptyBallotDoesNotInflateQuorum(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "0", "25.000", "75.000") // democratic, 4 members
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	joinProjectMember(t, ct, pid, "hive:outsider")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")

	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v").Success) // 1 real YES (25%)
	voteRaw(ct, propID, "hive:member2", "", "e1")                         // empty
	voteRaw(ct, propID, "hive:outsider", "", "e2")                        // empty

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assertAborts(t, exec, "proposal is failed", "empty ballots met quorum for a 1-real-voter proposal")
	assert.Equal(t, before, after, "payout executed on quorum inflated by empty ballots")
}

// R2-2: DESIGN NOTE (not a hard bug) — the voting deadline is "soft": votes are
// accepted after DurationHours as long as nobody has tallied yet (asserted intended
// by TestVoteAllowedAfterDurationBeforeTally). This documents that behaviour and
// flags the last-mover-advantage caveat: a member can wait past the nominal
// deadline, observe others, and still vote until someone calls tally.
func TestBreak_VoteAfterDeadlineStillAllowed_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	propID := createSimpleProposal(t, ct, pid, "1") // 1h duration
	// lateTS is ~2 days after creation — well past the deadline, but no tally yet.
	res := rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", propID)), nil, "hive:someone", lateTS, "v")
	assert.True(t, res.Success, "soft-deadline behaviour changed: post-deadline vote was rejected")
}

// R2-3: a poll (custom options) with an attached payout must never execute.
func TestBreak_PollWithPayoutCannotExecute(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")
	// custom options (field 4) => forced poll; payout attached.
	fields := []string{strconv.FormatUint(pid, 10), "poll", "d", "1", "red;green;blue", "0", "hive:someoneelse:0.500:hive", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "poll proposal creation failed")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "0", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "0", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assertAborts(t, exec, "proposal is closed", "a poll executed its payout")
	assert.Equal(t, before, after, "poll paid out despite being advisory")
}

// R2-4: selecting the same option multiple times in one ballot must not multiply
// its weight.
func TestBreak_DuplicateChoicesNotMultiplied(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1") // stake, low quorum
	joinWithStake(t, ct, pid, "hive:someoneelse", "40.000")
	joinWithStake(t, ct, pid, "hive:member2", "60.000") // silent, total=101
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")

	// 40/101 = 39.6% < 50%. Selecting option 1 three times must stay 40, not 120.
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1,1,1", "v").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assertAborts(t, exec, "proposal is failed", "duplicate choices multiplied a sub-threshold vote over the line")
	assert.Equal(t, before, after, "duplicate-choice weight inflation paid out")
}

// R2-5: a creator cancelling their own proposal does NOT get the cost refunded
// (prevents free create/cancel spam). Documents intended behaviour.
func TestBreak_CreatorCancelKeepsCostInTreasury(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // cost = 1
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	before := hiveBal(ct, "hive:someoneelse")
	// someoneelse creates (pays 1) then cancels their own proposal.
	fields := simpleProposalFields(pid, "1")
	propID, ok := createProposalRaw(ct, fields, "hive:someoneelse", "c")
	assert.True(t, ok)
	cancel := rawCallAt(ct, "proposal_cancel", PayloadUint64(propID), nil, "hive:someoneelse", defaultTimestamp, "x")
	assert.True(t, cancel.Success)
	after := hiveBal(ct, "hive:someoneelse")
	assert.Equal(t, before-1000, after, "creator reclaimed the proposal cost by self-cancelling")
}

// R2-6: cancelling a payout proposal releases the payout lock so the target can
// leave again.
func TestBreak_ActiveProposalDoesNotLockBeneficiary(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "5", "hive:someoneelse:0.500:hive", "")
	// An ACTIVE proposal holds no payout lock — locks are taken at tally-on-pass —
	// so the named member can arm their exit while it is still being voted on.
	arm := rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", defaultTimestamp, "l0")
	assert.True(t, arm.Success, "an unapproved payout proposal froze the beneficiary: %s", arm.Ret)
	assert.Equal(t, "exit requested", arm.Ret)
	// owner cancels
	assert.True(t, rawCallAt(ct, "proposal_cancel", PayloadUint64(propID), nil, "hive:someone", defaultTimestamp, "x").Success)
	// and the exit completes once the leave cooldown has elapsed (lateTS)
	res := rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", lateTS, "l1")
	assert.True(t, res.Success, "leave blocked after cancel: %s", res.Ret)
	assert.Equal(t, "exit finished", res.Ret)
}

// R2-7: staking via AddFunds must be rejected in a democratic project.
func TestBreak_AddStakeInDemocraticRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // democratic
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	res := rawCallAt(ct, "project_funds", PayloadString(fmt.Sprintf("%d|true", pid)), transferIntent("5.000"), "hive:someoneelse", defaultTimestamp, "f")
	assertAborts(t, res, "cannot add member stake > StakeMinAmt in democratic systems", "democratic project accepted a stake increase")
}

// R2-8: democratic threshold math — a 2/3 majority passes.
func TestBreak_DemocraticMajorityPasses(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "0", "50.000", "50.000") // democratic, 3 members
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success) // 2 of 3 = 66% > 50%
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:outsider")
	assert.True(t, exec.Success, "a 2/3 democratic majority failed to pass: %s", exec.Ret)
	assert.Equal(t, before+500, after, "democratic payout did not transfer")
}

// R2-9: democratic threshold math — a lone 1/3 vote fails.
func TestBreak_DemocraticMinorityFails(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "0", "50.000", "1") // low quorum so only threshold gates
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success) // 1 of 3 = 33% < 50%
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:outsider")
	assertAborts(t, exec, "proposal is failed", "a 1/3 minority passed a >50%% threshold")
	assert.Equal(t, before, after, "minority vote paid out")
}

// R2-10: a stake-mode proposal that is voted down (NO majority) must not execute
// (second scenario for the C1 fix, in stake mode).
func TestBreak_StakeNoMajorityDoesNotExecute(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1")
	joinWithStake(t, ct, pid, "hive:someoneelse", "100.000")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "0", "v").Success) // whale votes NO
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assertAborts(t, exec, "proposal is failed", "a NO-majority stake proposal executed")
	assert.Equal(t, before, after, "NO-majority stake proposal paid out")
}

// R2-11: an ICC proposal can only be executed by its creator.
func TestBreak_ICCOnlyCreatorCanExecute(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// ICC entry is contract|function|payload — those pipes are the trailing field,
	// rejoined by the decoder. Auth (creator-only) is checked before the call runs.
	fields := []string{strconv.FormatUint(pid, 10), "icc", "d", "1", "", "0", "", "", "", "",
		ContractID, "project_pause", strconv.FormatUint(pid, 10)}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "ICC proposal could not be created")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	// someoneelse (not the creator) must not be able to execute the ICC proposal.
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someoneelse", lateTS, "e")
	assertAborts(t, res, "only proposal creator can execute proposals with inter-contract calls", "a non-creator executed an ICC proposal")
}

// R2-12: quorum must be an exact-boundary gate — one voter short must fail.
func TestBreak_QuorumExactBoundary(t *testing.T) {
	ct := SetupContractTest()
	// 4 members, quorum 75% => ceil(3) = 3 voters required.
	pid := makeProject(t, ct, "0", "1.000", "75.000")
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	joinProjectMember(t, ct, pid, "hive:outsider")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:0.500:hive", "")
	// Only 2 of 4 vote — one short of the 3-voter quorum.
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:outsider")
	assertAborts(t, exec, "proposal is failed", "quorum met with one voter short of the requirement")
	assert.Equal(t, before, after, "sub-quorum proposal paid out")
}
