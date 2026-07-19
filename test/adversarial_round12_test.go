package contract_test

// Round 12 — CROSS-CONTRACT tests. These use a second registered contract
// (mockcontract/, built to artifacts/mock.wasm) to reach paths that a
// single-contract harness cannot:
//
//   - the ICC re-entrancy guard (CRITICAL): a hostile callee calls back into
//     proposal_execute and must not be able to replay the payout;
//   - ICC asset delivery: the callee must actually receive the allowance the DAO
//     debited from its treasury (the token/limit intent fix);
//   - delegation: a member calling a contract that calls the DAO must have the
//     DAO act FOR THE MEMBER (msg.sender identity — a deliberate product choice).

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"vsc-node/lib/test_utils"
	stateEngine "vsc-node/modules/state-processing"
)

//go:embed artifacts/mock.wasm
var MockWasm []byte

const MockID = "vscmockcontract"

// registerMock installs the companion contract alongside the DAO.
func registerMock(ct *test_utils.ContractTest) {
	ct.RegisterContract(MockID, ownerAddress, MockWasm)
}

// callMock invokes the MOCK contract (rawCallAt always targets the DAO).
func callMock(ct *test_utils.ContractTest, action, payload, authUser, nonce string) test_utils.ContractTestCallResult {
	res := ct.Call(stateEngine.TxVscCallContract{
		Caller: authUser,
		Self: stateEngine.TxSelf{
			TxId:                 fmt.Sprintf("mock-%s-%s-tx", action, nonce),
			BlockId:              "block1",
			Timestamp:            defaultTimestamp,
			RequiredAuths:        []string{authUser},
			RequiredPostingAuths: []string{},
		},
		ContractId: MockID,
		Action:     action,
		Payload:    PayloadString(payload),
		RcLimit:    100000,
	})
	if !res.Success && res.Ret == "" {
		res.Ret = res.ErrMsg
	}
	return res
}

// R12-1 (CRITICAL): a hostile ICC callee that re-enters proposal_execute must not
// be able to collect the payout twice. Before the checks-effects-interactions fix
// the terminal state was written AFTER the external call, so the re-entrant frame
// still observed ProposalPassed and replayed the payout on every recursion level.
func TestBreak_ICCReentrancyCannotDoublePayout(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// Fund the treasury so that EVERY level of the recursion could afford a payout
	// (depth limit is 20, payout is 2.000 -> 40.000 needed). This matters: with a
	// thin treasury the re-entrant drain runs out of funds, aborts, and the atomic
	// revert hides the bug behind delta=0. Only a treasury that can pay all 20
	// levels makes the pre-fix behaviour observable as a balance delta.
	addTreasuryFunds(t, ct, pid, "100.000")

	// payout 2.000 to outsider + an ICC that calls back into proposal_execute
	payout := "hive:outsider:2.000:hive"
	icc := fmt.Sprintf("%s|reenter|%s~0", MockID, ContractID) // proposal id 0
	fields := []string{strconv.FormatUint(pid, 10), "drain", "d", "1", "", "0", payout, "", "", "", icc}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "hostile proposal could not be created")
	assert.Equal(t, uint64(0), propID, "test assumes proposal id 0 (ICC payload references it)")

	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")

	before := hiveBal(ct, "hive:outsider")
	// creator executes (ICC proposals are creator-only)
	rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	delta := hiveBal(ct, "hive:outsider") - before

	// The payout must happen AT MOST once.
	//
	// This assertion is verified to have teeth: with the checks-effects-interactions
	// fix reverted, the mock's single re-entry measures delta=4000 here — the same
	// proposal paid the grantee TWICE. With the fix, the re-entrant frame observes
	// ProposalExecuted and aborts, which unwinds the whole transaction: delta=0.
	//
	// delta=0 is the correct, fail-closed outcome. A hostile ICC callee can deny
	// execution of the proposal that calls it, but it cannot extract a second
	// payout. Both 0 (reverted) and 2000 (paid once) are safe; 4000+ is the drain.
	assert.Less(t, delta, int64(4000), "ICC re-entrancy replayed the payout (delta=%d)", delta)
	assert.True(t, delta == 0 || delta == 2000, "unexpected payout delta %d", delta)
	t.Logf("re-entrancy contained: grantee delta=%d (drain would be 4000)", delta)
}

// R12-2: a second execute after a successful one is still rejected (the terminal
// state really is committed, not just ordered).
func TestBreak_ExecuteTwiceRejectedWithMock(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "10.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:1.000:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")

	before := hiveBal(ct, "hive:outsider")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e1").Success)
	assert.Equal(t, before+1000, hiveBal(ct, "hive:outsider"))
	second := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e2")
	assert.False(t, second.Success, "a proposal executed twice")
	assert.Equal(t, before+1000, hiveBal(ct, "hive:outsider"), "second execute paid out again")
}

// R12-3: an ICC that grants assets must actually DELIVER them — the callee draws
// its transfer.allow allowance. With the old {"to","tk","amount"} intent args the
// host silently skipped the intent, the callee's HiveDraw failed, and the whole
// execution aborted while the treasury had already been debited.
func TestBreak_ICCActuallyDeliversAssets(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "10.000")

	// ICC grants 1.000 HIVE and asks the mock to draw 1000 base units of it
	icc := fmt.Sprintf("%s|draw|%s~1000|hive=1.0", MockID, ContractID)
	fields := []string{strconv.FormatUint(pid, 10), "icc-pay", "d", "1", "", "0", "", "", "", "", icc}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "ICC proposal could not be created")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")

	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.True(t, res.Success,
		"ICC execution failed — the callee could not draw the granted allowance "+
			"(transfer.allow must use token/limit): %s", res.Ret)
}

// R12-4: an ICC whose callee draws MORE than the granted allowance must fail
// (the allowance is a real cap, not advisory).
func TestBreak_ICCCalleeCannotOverdrawAllowance(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "10.000")

	// grant 1.000 HIVE but ask the mock to draw 5.000 (5000 base units)
	icc := fmt.Sprintf("%s|draw|%s~5000|hive=1.0", MockID, ContractID)
	fields := []string{strconv.FormatUint(pid, 10), "overdraw", "d", "1", "", "0", "", "", "", "", icc}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, res.Success, "callee drew more than the granted transfer.allow limit")
}

// R12-5: DELEGATION (deliberate product behaviour). A member calling a contract
// that calls the DAO must have the DAO act FOR THE MEMBER — identity is
// msg.sender, so the member's own vote is recorded and counts at tally.
// If identity were msg.caller this would abort ("contract:... is not a member").
func TestBreak_DelegatedCallActsAsOriginalSender(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "10.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:1.000:hive", "")

	// someone votes DIRECTLY; someoneelse votes THROUGH the mock contract
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "direct").Success)
	res := callMock(ct, "delegate", fmt.Sprintf("%s~proposals_vote::%d|1", ContractID, propID), "hive:someoneelse", "d2")
	assert.True(t, res.Success, "delegated call failed — delegation is a supported feature: %s", res.Ret)

	// both votes must have counted (2 of 2 members) -> proposal passes and pays out
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.True(t, exec.Success, "proposal did not pass — the delegated vote was not counted as the member: %s", exec.Ret)
	assert.Equal(t, before+1000, hiveBal(ct, "hive:outsider"))
}

// R12-6: a benign ICC (no assets) executes cleanly end-to-end.
func TestBreak_BenignICCExecutes(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	icc := fmt.Sprintf("%s|noop|%s~x", MockID, ContractID)
	fields := []string{strconv.FormatUint(pid, 10), "benign", "d", "1", "", "0", "", "", "", "", icc}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.True(t, res.Success, "a benign ICC failed to execute: %s", res.Ret)
}

// createFreeProject builds a DAO with proposalCost=0 and stakeMin=0. Both matter for
// exact treasury accounting: the proposal cost is DEPOSITED INTO THE TREASURY on
// every proposal_create, so a cost-bearing project silently inflates the balance
// the assertions below depend on.
func createFreeProject(t *testing.T, ct *test_utils.ContractTest) uint64 {
	fields := defaultProjectFields()
	fields[8] = "0" // proposalCost
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(strings.Join(fields, "|")),
		transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

// iccDrawProposal builds+passes+executes a proposal whose only effect is an ICC
// granting `grant` HIVE to the mock, which draws `drawUnits` base units of it.
func iccDrawProposal(t *testing.T, ct *test_utils.ContractTest, pid uint64,
	grant string, drawUnits int, nonce string) test_utils.ContractTestCallResult {
	t.Helper()
	icc := fmt.Sprintf("%s|draw|%s~%d|hive=%s", MockID, ContractID, drawUnits, grant)
	fields := []string{strconv.FormatUint(pid, 10), "icc-" + nonce, "d", "1", "", "0", "", "", "", "", icc}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c"+nonce)
	assert.True(t, ok, "proposal %s could not be created", nonce)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1"+nonce).Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2"+nonce).Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t"+nonce)
	return rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e"+nonce)
}

// R12-7 (the dangerous direction): the treasury must never fund more than it holds
// across repeated ICC grants. Two proposals each granting 1.000 HIVE, both drawn in
// full, against a treasury of 1.500: the second MUST fail. If the DAO debited the
// treasury by less than it actually handed out, this is a double-spend.
func TestBreak_ICCGrantsCannotOverspendTreasury(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createFreeProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "1.500")

	first := iccDrawProposal(t, ct, pid, "1.0", 1000, "a")
	assert.True(t, first.Success, "first 1.000 grant should fit in a 1.500 treasury: %s", first.Ret)

	second := iccDrawProposal(t, ct, pid, "1.0", 1000, "b")
	assert.False(t, second.Success,
		"treasury overspent: a second 1.000 ICC grant succeeded against a 1.500 treasury")
}

// R12-8 (the safe direction): replaces a previously-skipped placeholder. When the
// callee draws LESS than the granted allowance, the DAO debits its treasury by the
// full GRANT, not by the amount actually drawn. The undrawn remainder is stranded.
//
// This is conservative (it can only under-spend, never over-spend, so it is not a
// solvency risk) but it is real: grant generously and the DAO loses the difference
// from its own accounting. Probe: after granting 1.000 from a 10.000 treasury while
// drawing only 0.400, a subsequent 9.500 payout tells us which number was debited.
func TestBreak_ICCUndrawnAllowanceIsStranded(t *testing.T) {
	ct := SetupContractTest()
	registerMock(ct)
	pid := createFreeProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "10.000")

	grant := iccDrawProposal(t, ct, pid, "1.0", 400, "s")
	assert.True(t, grant.Success, "ICC granting 1.000 / drawing 0.400 failed: %s", grant.Ret)

	// 9.500 fits only if the treasury was debited the 0.400 actually drawn.
	fields := []string{strconv.FormatUint(pid, 10), "probe", "d", "1", "", "0",
		"hive:outsider:9.500:hive", "", "", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "cp")
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "vp1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "vp2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "tp")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "ep")

	assert.False(t, res.Success,
		"treasury was debited only the amount DRAWN (0.400), not the amount GRANTED (1.000) — "+
			"if this now passes, the stranding limitation was fixed and this test should be inverted")
	t.Logf("confirmed: undrawn allowance is stranded — treasury debited the full 1.000 grant, "+
		"leaving 9.000, so a 9.500 payout is correctly refused (%s)", res.Ret)
}
