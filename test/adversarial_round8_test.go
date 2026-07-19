package contract_test

// Round 8 — multi-project isolation, cross-entity confusion, and lifecycle edges.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// R8-1: a payout in project A's proposal cannot touch project B's treasury.
func TestBreak_MultiProjectTreasuryIsolation(t *testing.T) {
	ct := SetupContractTest()
	pidA := createDefaultProject(t, ct)
	pidB := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pidA, "hive:someoneelse")
	addTreasuryFunds(t, ct, pidB, "5.000") // fund B only
	// A proposal in project A paying 5.000 HIVE — A's treasury only has proposal-cost
	// dust (~1.000), so this can only succeed if it wrongly reaches B's 5.000.
	propID := createPollProposal(t, ct, pidA, "1", "hive:someoneelse:5.000:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, res.Success, "project A payout succeeded by draining project B's treasury")
}

// R8-2: a member of project A cannot vote on project B's proposal.
func TestBreak_MemberOfACannotVoteOnB(t *testing.T) {
	ct := SetupContractTest()
	pidA := createDefaultProject(t, ct)
	pidB := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pidA, "hive:someoneelse") // member of A only
	propB := createSimpleProposal(t, ct, pidB, "1")    // proposal in B
	res := rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", propB)), nil, "hive:someoneelse", defaultTimestamp, "v")
	assert.False(t, res.Success, "a non-member of project B voted on B's proposal")
	assert.Contains(t, res.Ret, "not a member", "wrong rejection: %s", res.Ret)
}

// R8-3: proposal IDs are globally unique across projects.
func TestBreak_ProposalIDsGloballyUnique(t *testing.T) {
	ct := SetupContractTest()
	pidA := createDefaultProject(t, ct)
	pidB := createDefaultProject(t, ct)
	a := createSimpleProposal(t, ct, pidA, "1")
	b := createSimpleProposal(t, ct, pidB, "1")
	assert.NotEqual(t, a, b, "two proposals in different projects share an ID")
}

// R8-4: the same account can be a member of two projects with independent stake;
// leaving one does not affect membership/stake in the other.
func TestBreak_MultiProjectMemberIndependent(t *testing.T) {
	ct := SetupContractTest()
	pidA := createStakeProject(t, ct)
	pidB := createStakeProject(t, ct)
	joinWithStake(t, ct, pidA, "hive:someoneelse", "10.000")
	joinWithStake(t, ct, pidB, "hive:someoneelse", "3.000")
	// leave A (two-step)
	rawCallAt(ct, "project_leave", PayloadUint64(pidA), nil, "hive:someoneelse", defaultTimestamp, "l1")
	rawCallAt(ct, "project_leave", PayloadUint64(pidA), nil, "hive:someoneelse", lateTS, "l2")
	// still a member of B: a second join must be rejected as "already a member"
	res := rawCallAt(ct, "project_join", PayloadUint64(pidB), transferIntent("3.000"), "hive:someoneelse", lateTS, "j")
	assert.False(t, res.Success, "leaving project A also removed membership from project B")
	assert.Contains(t, res.Ret, "already a member", "unexpected: %s", res.Ret)
}

// R8-5: the proposal META whitelist path enforces MaxWhitelistAddresses (50), even
// though the direct owner path is intentionally uncapped (TestOwnerWhitelistAddNoLimit).
func TestBreak_WhitelistMetaAddressCap(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	mk := func(n int) string {
		a := make([]string, n)
		for i := range a {
			a[i] = "hive:u" + strconv.Itoa(i)
		}
		return strings.Join(a, ",")
	}
	// 51 addresses via a proposal meta whitelist_add must be rejected at execution.
	fields := []string{strconv.FormatUint(pid, 10), "wl", "d", "1", "", "0", "", "whitelist_add=" + mk(51), ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "proposal creation failed")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assert.False(t, res.Success, "51-address whitelist_add via proposal meta executed (over cap)")
}

// R8-6: a proposal duration below the project's configured minimum is rejected.
func TestBreak_DurationBelowProjectMinRejected(t *testing.T) {
	ct := SetupContractTest()
	// project min duration = 5
	f := []string{"dao", "desc", "0", "50.001", "50.001", "5", "0", "10", "1", "1", "", "", "", "", "1", "", "", ""}
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	pid := parseCreatedID(t, res.Ret, "project")
	// proposal with duration 1 (< 5) must be rejected
	fields := simpleProposalFields(pid, "1")
	bad := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assert.False(t, bad.Success, "a proposal shorter than the project minimum duration was accepted")
}

// R8-7: transferring ownership to oneself succeeds and leaves the owner in place.
func TestBreak_UpdateOwnerToSelf(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // owner = hive:someone, a member
	res := rawCallAt(ct, "project_transfer", PayloadString(fmt.Sprintf("%d|hive:someone", pid)), nil, "hive:someone", defaultTimestamp, "x")
	assert.True(t, res.Success, "owner could not transfer to self: %s", res.Ret)
	// still owner: can pause
	assert.True(t, rawCallAt(ct, "project_pause", PayloadUint64(pid), nil, "hive:someone", defaultTimestamp, "p").Success)
}

// R8-8: DESIGN NOTE — a member who voted and is then kicked keeps their cast weight
// in the tally (snapshot semantics; vote receipts are not purged on kick).
func TestBreak_KickedVoterWeightRemains_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1")
	joinWithStake(t, ct, pid, "hive:someoneelse", "60.000") // whale
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:outsider:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v").Success) // whale votes yes, passes threshold
	// kick the whale via a separate proposal
	kickP := createPollProposal(t, ct, pid, "1", "", "kick_member=hive:someoneelse")
	// only the owner (stake 1) can vote on the kick now... whale still member until kick executes
	assert.True(t, voteRaw(ct, kickP, "hive:someoneelse", "1", "k").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(kickP), nil, "hive:someone", lateTS, "kt")
	// kick may or may not pass; regardless, tally the payout proposal and see the
	// whale's earlier vote still counts (snapshot).
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:outsider")
	rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	after := hiveBal(ct, "hive:outsider")
	assert.Equal(t, before+500, after, "the pre-kick vote no longer counted (snapshot semantics changed)")
}

// R8-9: DESIGN NOTE — AddFunds with toStake=true but a NON-funds asset routes that
// asset to the treasury (not stake), silently.
func TestBreak_StakeWithNonFundsAssetGoesToTreasury_DesignNote(t *testing.T) {
	ct := SetupContractTest()
	pid := createStakeProject(t, ct) // FundsAsset = HIVE
	joinWithStake(t, ct, pid, "hive:someoneelse", "5.000")
	// member asks toStake=true but sends HBD (not the funds asset)
	res := rawCallAt(ct, "project_funds", PayloadString(fmt.Sprintf("%d|true", pid)), transferIntentWithToken("2.000", "hbd"), "hive:someoneelse", defaultTimestamp, "f")
	assert.True(t, res.Success, "HBD stake-deposit to a HIVE project failed: %s", res.Ret)
	// The HBD landed in treasury; a proposal can pay it back out.
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:2.000:hbd", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v0").Success)    // owner votes (quorum needs 2)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v").Success) // 5 stake, passes threshold
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	d0 := hbdBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	assert.Equal(t, d0+2000, hbdBal(ct, "hive:someoneelse"), "HBD 'stake' deposit did not land in the treasury")
}

// R8-10: cancelling a proposal in one project does not disturb an active proposal in
// another project (independent lifecycle).
func TestBreak_CancelIsolatedAcrossProjects(t *testing.T) {
	ct := SetupContractTest()
	pidA := createDefaultProject(t, ct)
	pidB := createDefaultProject(t, ct)
	a := createSimpleProposal(t, ct, pidA, "1")
	b := createSimpleProposal(t, ct, pidB, "1")
	// cancel A's proposal
	assert.True(t, rawCallAt(ct, "proposal_cancel", PayloadUint64(a), nil, "hive:someone", defaultTimestamp, "ca").Success)
	// B's proposal is still active and votable
	res := rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", b)), nil, "hive:someone", defaultTimestamp, "vb")
	assert.True(t, res.Success, "cancelling project A's proposal disturbed project B's: %s", res.Ret)
}

// R8-11: a proposal ID that does not exist is rejected cleanly by tally/execute/vote.
func TestBreak_NonexistentProposalRejected(t *testing.T) {
	ct := SetupContractTest()
	_ = createDefaultProject(t, ct)
	assert.False(t, rawCallAt(ct, "proposals_vote", PayloadString("999999|1"), nil, "hive:someone", defaultTimestamp, "v").Success)
	assert.False(t, rawCallAt(ct, "proposal_tally", PayloadString("999999"), nil, "hive:someone", lateTS, "t").Success)
	assert.False(t, rawCallAt(ct, "proposal_execute", PayloadString("999999"), nil, "hive:someone", lateTS, "e").Success)
	assert.False(t, rawCallAt(ct, "proposal_cancel", PayloadString("999999"), nil, "hive:someone", defaultTimestamp, "c").Success)
}
