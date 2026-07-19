package contract_test

// Round 5 — numeric precision, rounding, and float-comparison edges.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// R5-1: a positive proposal cost that rounds below chain precision (0.0004 -> 0)
// must be rejected at project creation, not silently become a free proposal.
func TestBreak_SubMilliunitCostRejected(t *testing.T) {
	ct := SetupContractTest()
	f := []string{"dao", "desc", "0", "50.001", "50.001", "1", "0", "10", "0.0004", "1", "", "", "", "", "1", "", "", ""}
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "proposal cost is below the minimum representable amount", "a sub-milliunit proposal cost (0.0004) was accepted -> free proposals")
}

// R5-2: a sub-milliunit min-stake (rounds to 0) must be rejected.
func TestBreak_SubMilliunitStakeMinRejected(t *testing.T) {
	ct := SetupContractTest()
	f := []string{"dao", "desc", "1", "50.001", "50.001", "1", "0", "10", "1", "0.0004", "", "", "", "", "1", "", "", ""}
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "min stake is below the minimum representable amount", "a sub-milliunit min stake (0.0004) was accepted")
}

// R5-3: raising proposal cost to a sub-milliunit value via governance is rejected.
func TestBreak_SubMilliunitCostViaMetaRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "1", "", "update_proposalCost=0.0004")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, res, "proposal cost is below the minimum representable amount", "sub-milliunit proposal cost accepted via meta update")
}

// R5-4: a negative intent limit must be rejected (must not reach HiveDraw).
func TestBreak_NegativeIntentLimitRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntentWithToken("-5.000", "hive"), "hive:member2", defaultTimestamp, "j")
	assertAborts(t, res, "invalid intent limit", "a negative intent limit was accepted")
}

// R5-5: a NaN intent limit must be rejected.
func TestBreak_NaNIntentLimitRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntentWithToken("NaN", "hive"), "hive:member2", defaultTimestamp, "j")
	assertAborts(t, res, "invalid intent limit", "a NaN intent limit was accepted")
}

// R5-6: a zero intent limit must be rejected.
func TestBreak_ZeroIntentLimitRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntentWithToken("0", "hive"), "hive:member2", defaultTimestamp, "j")
	assertAborts(t, res, "invalid intent limit", "a zero intent limit was accepted")
}

// R5-7: an astronomically large deposit aborts gracefully (out of range) rather
// than wrapping negative / trapping (FloatToAmount 2^63 guard).
func TestBreak_HugeDepositGracefulReject(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct)
	// ~2^63 scaled: token value near 9.223e15 -> *1000 near 2^63.
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntentWithToken("9223372036854776", "hive"), "hive:member2", defaultTimestamp, "j")
	assertAborts(t, res, "amount out of range", "an out-of-range deposit was accepted")
}

// R5-8: fractional amounts (within milliunit precision) survive a payout exactly.
func TestBreak_FractionalPayoutExact(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "5.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:1.234:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	after := hiveBal(ct, "hive:someoneelse")
	assert.Equal(t, before+1234, after, "1.234 HIVE payout lost precision (expected +1234 milliunits)")
}

// R5-9: with the default 50.001% threshold, an exact 50/50 tie FAILS (defaults are
// a strict-majority configuration).
func TestBreak_DefaultThresholdRejectsExactTie(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // threshold 50.001, democratic
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	// one YES, one NO -> 50/50 -> 50% < 50.001% -> must fail
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "0", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assertAborts(t, exec, "proposal is failed", "an exact 50/50 tie passed the 50.001%% threshold")
	assert.Equal(t, before, after)
}

// R5-10: DESIGN NOTE — the threshold comparison is inclusive (>=), so a project
// configured with exactly 50% threshold passes at exactly 50% support. Operators
// wanting a strict majority must set >50 (the default is 50.001).
//
// This deliberately does NOT use a 50/50 tie to demonstrate it any more. A tie is
// now resolved to the LOWEST option index, so on the default [no, yes] ballot an
// exactly-split vote resolves to "no" — see TestBreak_TieResolvesToRejection.
// Here 3 of 4 members vote (2 yes, 1 no) so "yes" wins outright with exactly 50%
// of the 4-member denominator, isolating the inclusive-threshold rule.
func TestBreak_ExactThresholdInclusive_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	// threshold 50.0 exactly, low quorum so threshold is the gate.
	f := []string{"dao", "desc", "0", "50.0", "1", "1", "0", "10", "1", "1", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	pid := parseCreatedID(t, res.Ret, "project")
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	joinProjectMember(t, ct, pid, "hive:outsider")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)  // yes
	assert.True(t, voteRaw(ct, propID, "hive:member2", "1", "v2").Success)  // yes
	assert.True(t, voteRaw(ct, propID, "hive:outsider", "0", "v3").Success) // no
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	// 2 yes of a 4-member denominator == exactly 50%, which passes at threshold 50.0.
	assert.True(t, exec.Success, "inclusive-threshold behaviour changed (50%% no longer passes at 50.0): %s", exec.Ret)
}

// R5-10b: a dead-even vote must NOT approve. The tally scan resolves ties to the
// lowest option index, which is "no" on the default ballot.
//
// Previously the scan used >= so the HIGHEST index won ties, and ApproveOptionIndex
// is the higher index — meaning an exactly-split vote counted as approval. The
// 50.001% default masks it, but a project configuring a round 50% threshold would
// approve proposals half its membership voted against.
func TestBreak_TieResolvesToRejection(t *testing.T) {
	ct := SetupContractTest()
	// threshold exactly 50.0 so a 50/50 split clears the threshold gate and the
	// outcome is decided purely by the tie-break.
	f := []string{"dao", "desc", "0", "50.0", "1", "1", "0", "10", "1", "1", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	pid := parseCreatedID(t, res.Ret, "project")
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)     // yes
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "0", "v2").Success) // no -> 50/50
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")

	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, exec, "proposal is failed", "a dead-even vote was treated as approval")
	assert.Equal(t, before, hiveBal(ct, "hive:someoneelse"), "tie paid out the treasury")
}

// R5-11: quorum with a fractional percent rounds up correctly (3 members, 33.334%
// -> ceil(1.00002) = 2 voters required).
func TestBreak_QuorumFractionalPercentCeil(t *testing.T) {
	ct := SetupContractTest()
	f := []string{"dao", "desc", "0", "1.0", "33.334", "1", "0", "10", "1", "1", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	pid := parseCreatedID(t, res.Ret, "project")
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:0.500:hive", "")
	// only 1 of 3 votes; quorum needs ceil(3*0.33334)=2 -> must fail
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:outsider")
	assertAborts(t, exec, "proposal is failed", "1 of 3 voters met a 33.334%% (2-voter) quorum")
	assert.Equal(t, before, after)
}
