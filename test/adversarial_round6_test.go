package contract_test

// Round 6 — multi-asset treasury accounting, ICC guards, and governance design notes.

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"vsc-node/lib/test_utils"
	"vsc-node/modules/db/vsc/contracts"
	ledgerDb "vsc-node/modules/db/vsc/ledger"
)

func hbdBal(ct *test_utils.ContractTest, acct string) int64 {
	return ct.GetBalance(acct, ledgerDb.AssetHbd)
}

// addTreasuryAsset donates a specific asset to a project's treasury (toStake=false).
func addTreasuryAsset(t *testing.T, ct *test_utils.ContractTest, pid uint64, amount, token, user string) {
	CallContract(t, ct, "project_funds", PayloadString(fmt.Sprintf("%d|false", pid)), transferIntentWithToken(amount, token), user, true, uint(1_000_000_000))
}

// R6-1: a payout spanning two assets moves both.
func TestBreak_MultiAssetPayoutBothTransfer(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // FundsAsset = HIVE
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")                        // HIVE
	addTreasuryAsset(t, ct, pid, "2.000", "hbd", "hive:someone") // HBD
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:1.000:hive;hive:member2:1.000:hbd", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	h0, d0 := hiveBal(ct, "hive:someoneelse"), hbdBal(ct, "hive:member2")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	assert.Equal(t, h0+1000, hiveBal(ct, "hive:someoneelse"), "HIVE leg did not transfer")
	assert.Equal(t, d0+1000, hbdBal(ct, "hive:member2"), "HBD leg did not transfer")
}

// R6-2: a payout in an asset the treasury doesn't hold aborts the whole payout and
// leaves the other asset untouched (no cross-asset drain).
func TestBreak_CrossAssetPayoutCannotDrainOther(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000") // HIVE only, no HBD
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive;hive:someoneelse:1.000:hbd", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	h0 := hiveBal(ct, "hive:someoneelse")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, res.Success, "a payout with an unfunded HBD leg executed")
	assert.Equal(t, h0, hiveBal(ct, "hive:someoneelse"), "HIVE leg leaked despite the HBD leg failing")
}

// R6-3: a non-FundsAsset (HBD) deposited to a HIVE project's treasury is later
// withdrawable via an HBD payout.
func TestBreak_NonFundsAssetDepositWithdrawable(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // FundsAsset = HIVE
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryAsset(t, ct, pid, "3.000", "hbd", "hive:someone")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:2.000:hbd", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	d0 := hbdBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	assert.Equal(t, d0+2000, hbdBal(ct, "hive:someoneelse"), "HBD deposited to a HIVE project was not withdrawable")
}

// R6-4: DESIGN NOTE — kick_member is all-or-nothing. A batch that names the owner
// aborts entirely and kicks nobody (matches TestKickMemberCannotKickOwner).
func TestBreak_KickOwnerBatchAllOrNothing_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "40.000", "1")
	joinWithStake(t, ct, pid, "hive:someoneelse", "5.000")
	joinWithStake(t, ct, pid, "hive:member2", "60.000")
	propID := createPollProposal(t, ct, pid, "1", "", "kick_member=hive:someoneelse,hive:someone")
	assert.True(t, voteRaw(ct, propID, "hive:member2", "1", "v").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, res.Success, "a kick batch including the owner unexpectedly executed")
	assert.Contains(t, res.Ret, "cannot kick project owner", "batch aborted for the wrong reason: %s", res.Ret)
	// The owner remains the owner (owner-only pause still works).
	assert.True(t, rawCallAt(ct, "project_pause", PayloadUint64(pid), nil, "hive:someone", lateTS, "p").Success, "owner lost ownership after a failed kick batch")
}

// R6-5: DESIGN NOTE — anyone (even a non-member) can donate to any project's
// treasury (AddFunds toStake=false has no membership check).
func TestBreak_AnyoneCanDonateTreasury_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	res := rawCallAt(ct, "project_funds", PayloadString(fmt.Sprintf("%d|false", pid)), transferIntent("2.000"), "hive:outsider", defaultTimestamp, "f")
	assert.True(t, res.Success, "a non-member could not donate to the treasury (behaviour change)")
}

// R6-6: DESIGN NOTE — free + democratic DAOs have no Sybil resistance: one entity
// controlling a majority of (free) accounts can drain donated treasury funds.
func TestBreak_SybilFreeDemocraticDrain_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	// fully free: stakeMin 0, cost 0, democratic
	f := []string{"dao", "desc", "0", "50.001", "50.001", "1", "0", "10", "0", "0", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(f)), nil, "hive:someone", true, uint(1_000_000_000))
	pid := parseCreatedID(t, res.Ret, "project")
	// attacker sockpuppets join for free
	for _, u := range []string{"hive:someoneelse", "hive:member2", "hive:outsider"} {
		CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(pid, 10)), nil, u, true, uint(1_000_000_000))
	}
	// a donor funds the treasury
	addTreasuryFunds(t, ct, pid, "5.000")
	// a sockpuppet proposes paying itself; the others rubber-stamp it
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:3.000:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:member2", "1", "v2").Success)
	assert.True(t, voteRaw(ct, propID, "hive:outsider", "1", "v3").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	after := hiveBal(ct, "hive:outsider")
	assert.Equal(t, before+3000, after, "Sybil drain did not go through (free-democratic exposure changed)")
}

// R6-7: adding a whitelisted address and removing it in the SAME proposal resolves
// deterministically (sorted meta order: add then remove => not whitelisted).
func TestBreak_WhitelistAddRemoveSameProposalDeterministic(t *testing.T) {
	ct := SetupContractTest()
	pid := createWhitelistProject(t, ct) // owner "hive:someone" is the sole member
	// a proposal both adds and removes someoneelse from the whitelist
	propID := createPollProposal(t, ct, pid, "1", "", "whitelist_add=hive:someoneelse;whitelist_remove=hive:someoneelse")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	// deterministic outcome: someoneelse ends NOT whitelisted -> join must fail
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:someoneelse", lateTS, "j")
	assert.False(t, res.Success, "add+remove in one proposal left the address whitelisted (nondeterministic?)")
}

// R6-8: two intents for the SAME asset in one AddFunds cannot double-draw (host
// dedups the token limit; the second draw reverts the whole tx).
func TestBreak_DuplicateSameAssetIntentReverts(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	dupIntents := []contracts.Intent{
		{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}},
		{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}},
	}
	res := rawCallAt(ct, "project_funds", PayloadUint64(pid), dupIntents, "hive:someone", defaultTimestamp, "d")
	assert.False(t, res.Success, "two same-asset intents double-drew into the treasury")
}

// R6-9: an ICC requesting more of an asset than the treasury holds aborts and
// leaves the treasury intact.
func TestBreak_ICCInsufficientTreasuryAborts(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "1.000")
	// ICC asks for 100 HIVE — far more than treasury holds.
	fields := []string{strconv.FormatUint(pid, 10), "icc", "d", "1", "", "0", "", "", "", "",
		ContractID, "noop", "{}", "hive=100.0"}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok)
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, res.Success, "an ICC over-drawing the treasury executed")
}

// R6-10: DESIGN NOTE / KNOWN LIMITATION — the ICC path debits the treasury by the
// full transfer.allow allowance up front; a callee that draws less strands the
// difference (untracked but not stealable). Documented in FINDINGS-REVIEW.md. No
// clean single-node harness reproduction (needs an under-drawing callee), so this
// is a placeholder marker asserting the ICC balance-check path exists.
func TestBreak_ICCAllowanceDebitNote(t *testing.T) {
	// Superseded: this used to be a documentation-only skip because the
	// allowance-vs-actual gap needed a real callee contract to observe. Round 12
	// added one (mockcontract/), so the behaviour is now asserted for real:
	//
	//   TestBreak_ICCUndrawnAllowanceIsStranded    — the treasury is debited the full
	//     GRANT even when the callee draws less; the remainder is stranded (safe
	//     direction: under-spend, never over-spend).
	//   TestBreak_ICCGrantsCannotOverspendTreasury — repeated grants cannot exceed
	//     the treasury (the dangerous direction).
	t.Skip("superseded by TestBreak_ICCUndrawnAllowanceIsStranded (round 12)")
}
