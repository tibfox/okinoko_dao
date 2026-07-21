package contract_test

// Round 14 — closes the coverage gaps the mutation audit found. Each control here
// was verified UNTESTED: breaking it in contract/ left the entire suite green.
// Every test below was then confirmed to go red under that same mutation.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"vsc-node/lib/test_utils"
)

// assertAborts checks BOTH that a call failed and WHY.
//
// The suite has ~86 negative assertions that only check Success == false. Any of
// them passes if the call fails for an unrelated reason — a wrong field count, a
// missing intent, a typo'd address — so they can silently stop testing the thing
// they name. Prefer this helper for new negative tests.
func assertAborts(t *testing.T, res test_utils.ContractTestCallResult, wantMsg, what string, args ...interface{}) {
	t.Helper()
	label := fmt.Sprintf(what, args...)
	assert.False(t, res.Success, "%s: call unexpectedly succeeded", label)
	if res.Success {
		return
	}
	assert.Contains(t, res.Ret, wantMsg,
		"%s: rejected, but for the wrong reason (got %q, want it to contain %q)", label, res.Ret, wantMsg)
}

// R14-1: re-voting must not inflate the distinct-voter count used for quorum.
//
// UNTESTED before: mutating `if prevVote == nil { prpsl.VoterCount++ }` to an
// unconditional increment left the whole suite green. With that mutation ONE
// member re-submitting the same ballot satisfies a two-voter quorum and the
// proposal pays out. No existing test re-voted and then checked quorum.
func TestBreak_RevotingDoesNotInflateQuorum(t *testing.T) {
	ct := SetupContractTest()
	// 3 members, quorum 50.001% -> ceil(1.50003) = 2 distinct voters required.
	// Threshold 1% so quorum is the only gate under test.
	f := defaultProjectFields()
	f[3] = "1.0"    // threshold
	f[4] = "50.001" // quorum
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(strings.Join(f, "|")),
		transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	pid := parseCreatedID(t, res.Ret, "project")
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	joinProjectMember(t, ct, pid, "hive:member2")
	addTreasuryFunds(t, ct, pid, "5.000")

	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:1.000:hive", "")

	// A SINGLE member votes, then votes again. That is one distinct voter.
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v2").Success)

	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, exec, "proposal is failed", "one member re-voting satisfied a two-voter quorum")
	assert.Equal(t, before, hiveBal(ct, "hive:outsider"), "payout executed on an inflated quorum")
}

// R14-2: only the creator may execute a proposal carrying inter-contract calls.
//
// UNTESTED before: removing the `caller != prpsl.Creator` abort left the suite
// green, because every round-12 ICC test executes as hive:someone, who is always
// also the creator.
func TestBreak_ICCProposalExecutableOnlyByCreator(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")

	icc := fmt.Sprintf("%s|noop|%s~x", MockID, ContractID)
	fields := []string{strconv.FormatUint(pid, 10), "icc", "d", "1", "", "0", "", "", "", "", icc}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c") // creator = someone
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")

	// A DIFFERENT member must not be able to execute it.
	other := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someoneelse", lateTS, "e1")
	assertAborts(t, other, "only proposal creator", "non-creator executing an ICC proposal")

	// The creator still can.
	own := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e2")
	assert.True(t, own.Success, "creator could not execute their own ICC proposal: %s", own.Ret)
}

// R14-3: nowUnix() must ABORT rather than fall back to wall-clock time when the
// block timestamp is missing or unparseable.
//
// UNTESTED before: replacing the abort with time.Now().Unix() left the suite green,
// because every test supplies a valid timestamp. This is the consensus-fork guard —
// a per-validator time.Now() stamped into CreatedAt/JoinedAt/stake history would
// put different values in state on every node.
func TestBreak_UnparseableBlockTimestampAborts(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	// A timestamp the contract cannot parse must fail the call outright, NOT silently
	// substitute local time.
	res := rawCallAt(ct, "project_join", PayloadString(strconv.FormatUint(pid, 10)),
		transferIntent("1.000"), "hive:someoneelse", "not-a-timestamp", "j")
	assertAborts(t, res, "block timestamp unavailable", "join with an unparseable block timestamp")
}

// R14-4: the upper bounds on execution delay and leave cooldown are enforced.
// UNTESTED before: raising MaxDurationHours to 2e9 left the suite green — only the
// LOWER bound ("duration must be at least") had coverage.
func TestBreak_DurationUpperBoundsEnforced(t *testing.T) {
	ct := SetupContractTest()
	over := "87601" // MaxDurationHours (87600) + 1

	// project_create: execution delay above the cap
	f := defaultProjectFields()
	f[6] = over
	res := rawCallAt(ct, "project_create", PayloadString(strings.Join(f, "|")),
		transferIntent("1.000"), "hive:someone", defaultTimestamp, "c1")
	assertAborts(t, res, "execution delay must not exceed", "execution delay above the cap")

	// project_create: leave cooldown above the cap
	f2 := defaultProjectFields()
	f2[7] = over
	res2 := rawCallAt(ct, "project_create", PayloadString(strings.Join(f2, "|")),
		transferIntent("1.000"), "hive:someone", defaultTimestamp, "c2")
	assertAborts(t, res2, "leave cooldown must not exceed", "leave cooldown above the cap")

	// governance outcome: update_executionDelay above the cap.
	//
	// NOTE this bound is enforced at EXECUTE time, not at creation — the proposal is
	// accepted and can pass a full vote before being rejected. That fails closed (the
	// out-of-range value is never applied) but wastes a governance cycle; validating
	// meta outcomes at creation would be the friendlier behaviour.
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	fields := []string{strconv.FormatUint(pid, 10), "raise", "d", "1", "", "0", "",
		"update_executionDelay=" + over, "", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "p")
	assert.True(t, ok, "proposal creation itself is not where this is caught")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, exec, "execution delay must not exceed", "executing an out-of-range execution delay")
}

// R14-5: a numeric overflow literal must be rejected in every float config field.
//
// Naming note, because it matters for what this test is worth: this does NOT cover
// the math.IsNaN/IsInf guard, despite being written for it. Mutation-checked —
// deleting that guard leaves this test (and the round-9 NaN/Inf tests) green.
// "1e999" is legal decimal syntax, but strconv.ParseFloat returns ErrRange for it,
// so the `err != nil` branch rejects it before the guard is consulted.
//
// The IsNaN/IsInf guard is in fact unreachable from any payload now: literal
// "NaN"/"Inf" are rejected by the decimal-syntax filter on their letters, and
// overflow literals are rejected by the range error. It is retained as
// defence-in-depth for any future caller that reaches parseFloatField differently.
// What this test genuinely pins is that overflow literals cannot become a config
// value — which is the behaviour that matters.
func TestBreak_OverflowLiteralRejected(t *testing.T) {
	ct := SetupContractTest()
	for _, tc := range []struct{ field, idx, want string }{
		{"threshold", "3", "invalid threshold"},
		{"quorum", "4", "invalid quorum"},
		{"proposal cost", "8", "invalid proposal cost"},
		{"min stake", "9", "invalid min stake"},
	} {
		f := defaultProjectFields()
		i, _ := strconv.Atoi(tc.idx)
		f[i] = "1e999" // legal decimal syntax; overflows to +Inf
		res := rawCallAt(ct, "project_create", PayloadString(strings.Join(f, "|")),
			transferIntent("1.000"), "hive:someone", defaultTimestamp, "of"+tc.idx)
		assertAborts(t, res, tc.want, "overflow literal in %s", tc.field)
	}
}

// assertAbortsAny is for negative sites whose exact message legitimately varies
// (malformed-payload fuzz loops, or a branch that is currently unreachable). It
// still enforces the property that matters most: the call was rejected BY THE
// CONTRACT, with a real abort message — not by a host-level error such as
// "contract not found", which would mean the test never reached its subject.
func assertAbortsAny(t *testing.T, res test_utils.ContractTestCallResult, what string, args ...interface{}) {
	t.Helper()
	label := fmt.Sprintf(what, args...)
	assert.False(t, res.Success, "%s: call unexpectedly succeeded", label)
	if res.Success {
		return
	}
	assert.Contains(t, res.Ret, "msg:",
		"%s: rejected by the HOST, not by the contract (got %q) — the test never reached its subject",
		label, res.Ret)
}
