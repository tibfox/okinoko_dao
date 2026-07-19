package contract_test

// Round 3 adversarial tests — ICC reachability, ownership lifecycle, whitelist/NFT
// gating, cancel edges, historical stake weight, and malformed-input robustness.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// R3-1: after the decoder fix, an ICC proposal PARSES (the whole trailing field is
// consumed). With no such target contract registered it now fails at the existence
// check ("not found") rather than at parsing ("invalid ICC entry format").
func TestBreak_ICCReachableAfterDecoderFix(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := []string{strconv.FormatUint(pid, 10), "icc", "d", "1", "", "0", "", "", "meta", "",
		"contract:dex", "swap", "{}", "hive=1.0"}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	// Before the decoder fix this aborted with "invalid ICC entry format" because
	// only parts[10] was read, truncating the entry. Now the whole ICC field parses
	// and the proposal is created (the test harness treats every contract id as
	// existing, so the later existence check passes).
	assert.True(t, res.Success, "ICC proposal still unparseable after decoder fix: %s", res.Ret)
	assert.NotContains(t, res.Ret, "invalid ICC entry format", "ICC entry still fails to parse")
}

// R3-2: a genuinely malformed ICC entry (too few components) is still rejected.
func TestBreak_ICCMalformedRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := []string{strconv.FormatUint(pid, 10), "icc", "d", "1", "", "0", "", "", "meta", "", "onlycontract"}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assert.False(t, res.Success, "a 1-component ICC entry was accepted")
}

// R3-3: a proposal that sets ownership to a non-member must fail at execution.
func TestBreak_UpdateOwnerToNonMemberRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "1", "", "update_owner=hive:ghost")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, res.Success, "ownership was transferred to a non-member via proposal")
}

// R3-4: after a project becomes autonomous (remove_owner), owner-only ops must fail.
func TestBreak_RemoveOwnerDisablesOwnerOps(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "1", "", "remove_owner=1")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	// former owner can no longer pause
	res := rawCallAt(ct, "project_pause", PayloadUint64(pid), nil, "hive:someone", lateTS, "p")
	assert.False(t, res.Success, "former owner retained privileges after remove_owner")
}

// R3-5: transferring ownership to a non-member must be rejected.
func TestBreak_TransferOwnershipToNonMemberRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	res := rawCallAt(ct, "project_transfer", PayloadString(fmt.Sprintf("%d|hive:member2", pid)), nil, "hive:someone", defaultTimestamp, "x")
	assert.False(t, res.Success, "ownership transferred to a non-member")
}

// R3-6: a whitelist entry is consumed on join; re-joining after leaving requires a
// fresh whitelist approval.
func TestBreak_WhitelistConsumedRequiresReAdd(t *testing.T) {
	ct := SetupContractTest()
	pid := createWhitelistProject(t, ct)
	CallContract(t, ct, "project_whitelist_add", PayloadString(fmt.Sprintf("%d|hive:someoneelse", pid)), nil, "hive:someone", true, uint(1_000_000_000))
	joinProjectMember(t, ct, pid, "hive:someoneelse") // consumes the entry
	// leave (two-step, past cooldown)
	rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", defaultTimestamp, "l1")
	rawCallAt(ct, "project_leave", PayloadUint64(pid), nil, "hive:someoneelse", lateTS, "l2")
	// rejoin without re-whitelisting must fail
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:someoneelse", lateTS, "j")
	assert.False(t, res.Success, "rejoined a whitelist-only project without a fresh approval")
}

// R3-7: an NFT-gated project rejects joins from non-holders.
func TestBreak_NFTGateBlocksJoin(t *testing.T) {
	ct := SetupContractTest()
	pid := createNftGatedProject(t, ct)
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:someoneelse", defaultTimestamp, "j")
	assert.False(t, res.Success, "NFT gate did not block a non-holder")
}

// R3-8: an active proposal cannot be cancelled twice.
func TestBreak_DoubleCancelRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	propID := createSimpleProposal(t, ct, pid, "1")
	assert.True(t, rawCallAt(ct, "proposal_cancel", PayloadUint64(propID), nil, "hive:someone", defaultTimestamp, "c1").Success)
	res := rawCallAt(ct, "proposal_cancel", PayloadUint64(propID), nil, "hive:someone", defaultTimestamp, "c2")
	assert.False(t, res.Success, "a proposal was cancelled twice")
}

// R3-9: an executed proposal cannot be cancelled.
func TestBreak_CancelExecutedProposalRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	res := rawCallAt(ct, "proposal_cancel", PayloadUint64(propID), nil, "hive:someone", lateTS, "c")
	assert.False(t, res.Success, "an executed proposal was cancelled")
}

// R3-10: with execution delay 0, a passed proposal is executable immediately after
// tally (same timestamp).
func TestBreak_ExecutionDelayZeroAllowsImmediateExecute(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // execDelay = 0
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assert.True(t, exec.Success, "execDelay 0 proposal was not executable right after tally: %s", exec.Ret)
	assert.Equal(t, before+500, after)
}

// R3-11: stake added AFTER a proposal is created must not raise the voter's weight
// on that proposal (historical-stake snapshot).
func TestBreak_StakeIncreaseAfterProposalNotCounted(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1") // stake, low quorum
	joinWithStake(t, ct, pid, "hive:someoneelse", "40.000")
	joinWithStake(t, ct, pid, "hive:member2", "60.000") // total 101
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	// someoneelse has 40/101 = 39.6% at creation. Top up stake in a LATER block
	// (later timestamp than the proposal), then vote — historical weight must stay 40.
	const midTS = "2025-09-03T00:30:00" // after 00:00 creation, before the 1h deadline
	CallContractAt(t, ct, "project_funds", PayloadString(fmt.Sprintf("%d|true", pid)), transferIntent("100.000"), "hive:someoneelse", true, uint(1_000_000_000), midTS)
	assert.True(t, rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", propID)), nil, "hive:someoneelse", midTS, "v").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	exec := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:someoneelse")
	assert.False(t, exec.Success, "post-creation stake top-up boosted voting weight over threshold")
	assert.Equal(t, before, after, "stake-topup vote inflation paid out")
}

// R3-12: kicking a member via proposal refunds their stake and removes them.
func TestBreak_KickViaProposalRefundsStake(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1")
	joinWithStake(t, ct, pid, "hive:someoneelse", "5.000")
	joinWithStake(t, ct, pid, "hive:member2", "60.000") // provides the passing weight
	propID := createPollProposal(t, ct, pid, "1", "", "kick_member=hive:someoneelse")
	assert.True(t, voteRaw(ct, propID, "hive:member2", "1", "v").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	after := hiveBal(ct, "hive:someoneelse")
	assert.Equal(t, before+5000, after, "kicked member was not refunded their 5.000 stake")
	// kicked member is no longer a member: joining again should be allowed (not "already a member")
	rejoin := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("5.000"), "hive:someoneelse", lateTS, "j")
	assert.True(t, rejoin.Success, "kicked member was not actually removed")
}

// R3-13: DESIGN NOTE — a majority can pay the treasury to itself. This is intended
// governance (the treasury is majority-controlled); documents that the guardrails
// are quorum/threshold, not payout-target restrictions.
func TestBreak_MajoritySelfPayoutDrains_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1")
	joinWithStake(t, ct, pid, "hive:member2", "100.000")
	addTreasuryFunds(t, ct, pid, "3.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:member2:1.000:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:member2", "1", "v").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:member2")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	after := hiveBal(ct, "hive:member2")
	assert.Equal(t, before+1000, after, "majority self-payout did not transfer")
}

// R3-14: malformed / hostile payloads must abort cleanly (never crash the harness).
func TestBreak_MalformedPayloadsAbortCleanly(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	cases := []struct {
		action  string
		payload string
	}{
		{"project_join", "not-a-number"},
		{"project_join", ""},
		{"proposals_vote", fmt.Sprintf("%d|", pid)},          // no choices
		{"proposals_vote", "abc|1"},                          // bad proposal id
		{"proposal_create", strconv.FormatUint(pid, 10)},     // missing required fields
		{"proposal_tally", "99999"},                          // nonexistent proposal
		{"proposal_execute", "not-a-number"},                 // bad id
		{"project_transfer", strconv.FormatUint(pid, 10)},    // missing new owner
		{"project_funds", strconv.FormatUint(pid, 10)},       // missing toStake flag
		{"project_whitelist_add", strconv.FormatUint(pid, 10)}, // no addresses
	}
	for i, c := range cases {
		res := rawCallAt(ct, c.action, PayloadString(c.payload), nil, "hive:someone", defaultTimestamp, "m"+strconv.Itoa(i))
		assert.False(t, res.Success, "%s accepted malformed payload %q", c.action, c.payload)
		assert.NotContains(t, strings.ToLower(res.Ret), "panic", "%s panicked on %q", c.action, c.payload)
	}
}
