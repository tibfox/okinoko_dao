package contract_test

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"vsc-node/lib/test_utils"
	ledgerDb "vsc-node/modules/db/vsc/ledger"
)

// =============================================================================
// Milestone Commitment Tests
//
// A commitment splits "fund this developer" into two votes: `commit_funds`
// reserves the money, and a later `release_commitment` (milestone met) or
// `cancel_commitment` (milestone missed) settles it. The point of the first vote
// is the reservation, so most of what is worth testing here is what OTHER
// proposals can no longer do while a commitment is pending.
// =============================================================================

const (
	kProjectTreasuryPrefix  byte = 0x07
	kProjectCommittedPrefix byte = 0x08
	kCommitmentPrefix       byte = 0x30
)

// stateKeyProjectAsset rebuilds the contract's projectTreasuryKey/
// projectCommittedKey layout: prefix, little-endian project id, asset name.
func stateKeyProjectAsset(prefix byte, projectID uint64, asset string) string {
	buf := make([]byte, 0, 1+8+len(asset))
	buf = append(buf, prefix)
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], projectID)
	buf = append(buf, n[:]...)
	buf = append(buf, asset...)
	return string(buf)
}

// stateKeyCommitment rebuilds commitmentKey: prefix + little-endian proposal id.
func stateKeyCommitment(proposalID uint64) string {
	buf := make([]byte, 9)
	buf[0] = kCommitmentPrefix
	binary.LittleEndian.PutUint64(buf[1:], proposalID)
	return string(buf)
}

// readScaledBalance reads one of the contract's scaled int64 balance keys.
// A missing key is zero, matching getTreasuryBalance/getCommittedBalance.
func readScaledBalance(t *testing.T, ct *test_utils.ContractTest, prefix byte, projectID uint64, asset string) int64 {
	t.Helper()
	raw := ct.StateGet(ContractID, stateKeyProjectAsset(prefix, projectID, asset))
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	val, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("unparseable balance %q: %v", raw, err)
	}
	return val
}

func treasuryOf(t *testing.T, ct *test_utils.ContractTest, projectID uint64, asset string) int64 {
	t.Helper()
	return readScaledBalance(t, ct, kProjectTreasuryPrefix, projectID, asset)
}

func committedOf(t *testing.T, ct *test_utils.ContractTest, projectID uint64, asset string) int64 {
	t.Helper()
	return readScaledBalance(t, ct, kProjectCommittedPrefix, projectID, asset)
}

func commitmentRecord(ct *test_utils.ContractTest, proposalID uint64) string {
	return ct.StateGet(ContractID, stateKeyCommitment(proposalID))
}

// passProposal drives a proposal through the whole lifecycle: every voter
// approves, then it is tallied and executed after the voting window closes.
func passProposal(t *testing.T, ct *test_utils.ContractTest, proposalID uint64, voters ...string) {
	t.Helper()
	voteForProposal(t, ct, proposalID, voters...)
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), afterVoting)
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), afterVoting)
}

// passProposalExpectExecuteFailure votes a proposal through and tallies it, but
// expects the EXECUTE step to abort. Used for guards that can only fire once the
// membership has already approved the action.
func passProposalExpectExecuteFailure(t *testing.T, ct *test_utils.ContractTest, proposalID uint64, voters ...string) string {
	t.Helper()
	voteForProposal(t, ct, proposalID, voters...)
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), afterVoting)
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), afterVoting)
	return res.Ret
}

// afterVoting is any timestamp past the 1-hour voting window the helpers use.
const afterVoting = "2025-09-05T00:00:00"

// setupCommitmentProject builds a two-member project with a funded treasury and
// returns the project id. The treasury holds `funding` on top of whatever the
// creation/join/proposal fees already contributed.
func setupCommitmentProject(t *testing.T, funding string) (*test_utils.ContractTest, uint64) {
	t.Helper()
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	addTreasuryFunds(t, ct, projectID, funding)
	return ct, projectID
}

// commitProposal creates and passes a commit_funds proposal, returning its id —
// which is also the commitment's id.
func commitProposal(t *testing.T, ct *test_utils.ContractTest, projectID uint64, entries string) uint64 {
	t.Helper()
	id := createPollProposal(t, ct, projectID, "1", "", "commit_funds="+entries)
	passProposal(t, ct, id, "hive:someone", "hive:someoneelse")
	return id
}

// createProposalAs creates a meta-only proposal on behalf of a specific member.
// createPollProposal always authors as hive:someone, which cannot create
// proposals in a project that member has not joined.
func createProposalAs(t *testing.T, ct *test_utils.ContractTest, projectID uint64, creator string, meta string) uint64 {
	t.Helper()
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"milestone",
		"settle a commitment",
		"1",
		"",
		"0",
		"",
		meta,
		"",
	}
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(strings.Join(fields, "|")), transferIntent("1.000"), creator, true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "proposal")
}

// =============================================================================
// Happy path
// =============================================================================

// TestCommitmentReservesButDoesNotPay is the core of the first vote: after
// commit_funds executes the money has left the treasury and is sitting in the
// committed balance, and the developer has been paid nothing.
func TestCommitmentReservesButDoesNotPay(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")

	treasuryBefore := treasuryOf(t, ct, projectID, "hive")
	devBefore := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive)

	commitID := commitProposal(t, ct, projectID, "hive:someoneelse:4.000:hive")

	if got := committedOf(t, ct, projectID, "hive"); got != 4000 {
		t.Fatalf("expected 4.000 hive committed, got %d", got)
	}
	// The proposal itself cost 1.000 HIVE, which lands in the treasury, so compare
	// against the drop caused by the commitment rather than an absolute figure.
	treasuryAfter := treasuryOf(t, ct, projectID, "hive")
	if treasuryAfter >= treasuryBefore {
		t.Fatalf("treasury did not fall after commit: %d -> %d", treasuryBefore, treasuryAfter)
	}
	if devAfter := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive); devAfter > devBefore {
		t.Fatalf("beneficiary was paid at COMMIT time: %d -> %d", devBefore, devAfter)
	}
	if rec := commitmentRecord(ct, commitID); !strings.Contains(rec, "|1|") {
		t.Fatalf("expected pending commitment record, got %q", rec)
	}
}

// TestCommitmentReleasePaysBeneficiary walks the full two-vote flow and asserts
// the developer is paid exactly the committed amount, on the SECOND vote.
func TestCommitmentReleasePaysBeneficiary(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	commitID := commitProposal(t, ct, projectID, "hive:someoneelse:4.000:hive")

	devBefore := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive)

	releaseID := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("release_commitment=%d", commitID))
	passProposal(t, ct, releaseID, "hive:someone", "hive:someoneelse")

	devAfter := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive)
	// The release proposal costs the creator 1.000 HIVE, but the creator here is
	// hive:someone, not the beneficiary — so the beneficiary's delta is the payout
	// alone.
	if devAfter-devBefore != 4000 {
		t.Fatalf("expected beneficiary +4.000 hive, got %d", devAfter-devBefore)
	}
	if got := committedOf(t, ct, projectID, "hive"); got != 0 {
		t.Fatalf("expected committed balance drained, got %d", got)
	}
	if rec := commitmentRecord(ct, commitID); !strings.Contains(rec, "|2|") {
		t.Fatalf("expected released commitment record, got %q", rec)
	}
}

// TestCommitmentCancelReturnsFundsToTreasury proves the missed-milestone path
// puts the money back where other proposals can spend it again.
func TestCommitmentCancelReturnsFundsToTreasury(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	commitID := commitProposal(t, ct, projectID, "hive:someoneelse:4.000:hive")

	reservedTreasury := treasuryOf(t, ct, projectID, "hive")
	devBefore := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive)

	cancelID := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("cancel_commitment=%d", commitID))
	passProposal(t, ct, cancelID, "hive:someone", "hive:someoneelse")

	if got := committedOf(t, ct, projectID, "hive"); got != 0 {
		t.Fatalf("expected committed balance drained, got %d", got)
	}
	// +4.000 returned, +1.000 from the cancel proposal's own cost.
	if got := treasuryOf(t, ct, projectID, "hive"); got != reservedTreasury+4000+1000 {
		t.Fatalf("expected treasury %d, got %d", reservedTreasury+4000+1000, got)
	}
	if devAfter := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive); devAfter != devBefore {
		t.Fatalf("cancelled commitment still paid the beneficiary: %d -> %d", devBefore, devAfter)
	}
}

// TestCommitmentMultipleEntriesAndAssets covers a commitment spanning several
// beneficiaries and both supported assets in one action.
func TestCommitmentMultipleEntriesAndAssets(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	addTreasuryFunds(t, ct, projectID, "10.000")
	// Fund the HBD side of the treasury too.
	CallContract(t, ct, "project_funds", PayloadString(fmt.Sprintf("%d|false", projectID)), transferIntentWithToken("6.000", "hbd"), "hive:someone", true, uint(1_000_000_000))

	commitID := commitProposal(t, ct, projectID, "hive:someoneelse:2.000:hive,hive:member2:1.500:hive,hive:someoneelse:3.000:hbd")

	if got := committedOf(t, ct, projectID, "hive"); got != 3500 {
		t.Fatalf("expected 3.500 hive committed, got %d", got)
	}
	if got := committedOf(t, ct, projectID, "hbd"); got != 3000 {
		t.Fatalf("expected 3.000 hbd committed, got %d", got)
	}

	devHiveBefore := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive)
	devHbdBefore := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHbd)
	otherBefore := ct.GetBalance("hive:member2", ledgerDb.AssetHive)

	releaseID := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("release_commitment=%d", commitID))
	passProposal(t, ct, releaseID, "hive:someone", "hive:someoneelse")

	if got := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive) - devHiveBefore; got != 2000 {
		t.Fatalf("expected +2.000 hive, got %d", got)
	}
	if got := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHbd) - devHbdBefore; got != 3000 {
		t.Fatalf("expected +3.000 hbd, got %d", got)
	}
	if got := ct.GetBalance("hive:member2", ledgerDb.AssetHive) - otherBefore; got != 1500 {
		t.Fatalf("expected +1.500 hive for second beneficiary, got %d", got)
	}
	if got := committedOf(t, ct, projectID, "hive"); got != 0 {
		t.Fatalf("hive committed balance not drained: %d", got)
	}
	if got := committedOf(t, ct, projectID, "hbd"); got != 0 {
		t.Fatalf("hbd committed balance not drained: %d", got)
	}
}

// =============================================================================
// The reservation itself
// =============================================================================

// TestCommittedFundsCannotBeSpentByAnotherPayout is the whole point of the
// feature. The same payout that succeeds against an uncommitted treasury must
// fail once a commitment has reserved the money — otherwise the first vote
// secures nothing and the developer's funding is just a promise racing every
// other proposal to the treasury.
func TestCommittedFundsCannotBeSpentByAnotherPayout(t *testing.T) {
	// Control: without a commitment, a 6.000 payout goes through.
	ctControl, controlProject := setupCommitmentProject(t, "10.000")
	controlPayout := createPollProposal(t, ctControl, controlProject, "1", "hive:member2:6.000:hive", "")
	passProposal(t, ctControl, controlPayout, "hive:someone", "hive:someoneelse")

	// Same treasury, but 8.000 is now committed to a milestone.
	ct, projectID := setupCommitmentProject(t, "10.000")
	commitProposal(t, ct, projectID, "hive:someoneelse:8.000:hive")

	raidID := createPollProposal(t, ct, projectID, "1", "hive:member2:6.000:hive", "")
	ret := passProposalExpectExecuteFailure(t, ct, raidID, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "insufficient") {
		t.Fatalf("expected insufficient-funds abort, got %q", ret)
	}
	if got := committedOf(t, ct, projectID, "hive"); got != 8000 {
		t.Fatalf("commitment was disturbed by the failed payout: %d", got)
	}
}

// TestCommittedFundsCannotBeSpentByAnotherCommitment closes the same hole
// against a second commit_funds rather than a direct payout.
func TestCommittedFundsCannotBeSpentByAnotherCommitment(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	commitProposal(t, ct, projectID, "hive:someoneelse:8.000:hive")

	secondID := createPollProposal(t, ct, projectID, "1", "", "commit_funds=hive:member2:6.000:hive")
	ret := passProposalExpectExecuteFailure(t, ct, secondID, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "insufficient") {
		t.Fatalf("expected insufficient-funds abort, got %q", ret)
	}
	if got := committedOf(t, ct, projectID, "hive"); got != 8000 {
		t.Fatalf("first commitment was disturbed: %d", got)
	}
}

// TestCommitInsufficientTreasuryFails rejects a commitment larger than the
// treasury outright, rather than recording a reservation the DAO cannot back.
func TestCommitInsufficientTreasuryFails(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "2.000")
	id := createPollProposal(t, ct, projectID, "1", "", "commit_funds=hive:someoneelse:500.000:hive")
	ret := passProposalExpectExecuteFailure(t, ct, id, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "insufficient") {
		t.Fatalf("expected insufficient-funds abort, got %q", ret)
	}
	if got := committedOf(t, ct, projectID, "hive"); got != 0 {
		t.Fatalf("failed commit still reserved %d", got)
	}
	if rec := commitmentRecord(ct, id); rec != "" {
		t.Fatalf("failed commit still wrote a record: %q", rec)
	}
}

// TestCommitPartialFailureReservesNothing checks the abort is atomic across a
// multi-entry commitment: the first entry fits, the second does not, and neither
// may end up reserved.
func TestCommitPartialFailureReservesNothing(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	before := treasuryOf(t, ct, projectID, "hive")

	id := createPollProposal(t, ct, projectID, "1", "", "commit_funds=hive:someoneelse:5.000:hive,hive:member2:500.000:hive")
	ret := passProposalExpectExecuteFailure(t, ct, id, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "insufficient") {
		t.Fatalf("expected insufficient-funds abort, got %q", ret)
	}
	if got := committedOf(t, ct, projectID, "hive"); got != 0 {
		t.Fatalf("partial commit leaked a reservation: %d", got)
	}
	// +1.000 for the failed proposal's own cost, which is charged at CREATION and
	// therefore survives the execute-time abort.
	if got := treasuryOf(t, ct, projectID, "hive"); got != before+1000 {
		t.Fatalf("treasury changed unexpectedly: %d -> %d", before, got)
	}
}

// =============================================================================
// Settlement guards
// =============================================================================

// TestReleaseUnknownCommitmentFails rejects a milestone vote pointing at a
// proposal that never committed anything.
func TestReleaseUnknownCommitmentFails(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	id := createPollProposal(t, ct, projectID, "1", "", "release_commitment=987654")
	ret := passProposalExpectExecuteFailure(t, ct, id, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "no commitment created by proposal") {
		t.Fatalf("expected unknown-commitment abort, got %q", ret)
	}
}

// TestDoubleReleaseFails is the double-spend guard: a released commitment has
// already drained its reservation, so releasing it again would pay out of
// whatever else the contract holds.
func TestDoubleReleaseFails(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	commitID := commitProposal(t, ct, projectID, "hive:someoneelse:4.000:hive")

	firstRelease := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("release_commitment=%d", commitID))
	passProposal(t, ct, firstRelease, "hive:someone", "hive:someoneelse")

	devBefore := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive)
	secondRelease := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("release_commitment=%d", commitID))
	ret := passProposalExpectExecuteFailure(t, ct, secondRelease, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "already released") {
		t.Fatalf("expected already-released abort, got %q", ret)
	}
	if devAfter := ct.GetBalance("hive:someoneelse", ledgerDb.AssetHive); devAfter != devBefore {
		t.Fatalf("second release paid again: %d -> %d", devBefore, devAfter)
	}
}

// TestReleaseAfterCancelFails stops a cancelled commitment from being revived —
// its funds are back in the treasury and may already have been spent.
func TestReleaseAfterCancelFails(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	commitID := commitProposal(t, ct, projectID, "hive:someoneelse:4.000:hive")

	cancelID := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("cancel_commitment=%d", commitID))
	passProposal(t, ct, cancelID, "hive:someone", "hive:someoneelse")

	releaseID := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("release_commitment=%d", commitID))
	ret := passProposalExpectExecuteFailure(t, ct, releaseID, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "already cancelled") {
		t.Fatalf("expected already-cancelled abort, got %q", ret)
	}
}

// TestCancelAfterReleaseFails is the mirror: a released commitment cannot be
// "returned" to the treasury, which would mint funds that were already paid out.
func TestCancelAfterReleaseFails(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	commitID := commitProposal(t, ct, projectID, "hive:someoneelse:4.000:hive")

	releaseID := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("release_commitment=%d", commitID))
	passProposal(t, ct, releaseID, "hive:someone", "hive:someoneelse")

	treasuryBefore := treasuryOf(t, ct, projectID, "hive")
	cancelID := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("cancel_commitment=%d", commitID))
	ret := passProposalExpectExecuteFailure(t, ct, cancelID, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "already released") {
		t.Fatalf("expected already-released abort, got %q", ret)
	}
	// The failed cancel's own 1.000 cost is charged at creation and survives.
	if got := treasuryOf(t, ct, projectID, "hive"); got != treasuryBefore+1000 {
		t.Fatalf("cancel-after-release credited the treasury: %d -> %d", treasuryBefore, got)
	}
}

// TestCrossProjectReleaseIsRejected is the isolation guard. Commitment ids come
// from the single global proposal counter, so without the project check a
// throwaway one-member DAO could vote to release funds another DAO committed.
func TestCrossProjectReleaseIsRejected(t *testing.T) {
	ct := SetupContractTest()

	victimProject := createDefaultProject(t, ct)
	joinProjectMember(t, ct, victimProject, "hive:someoneelse")
	addTreasuryFunds(t, ct, victimProject, "10.000")
	commitID := commitProposal(t, ct, victimProject, "hive:someoneelse:6.000:hive")

	// A second project, created by someone else, votes to release the FIRST
	// project's commitment.
	attackerFields := defaultProjectFields()
	attackerFields[0] = "raider"
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(strings.Join(attackerFields, "|")), transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))
	attackerProject := parseCreatedID(t, res.Ret, "project")

	raidID := createProposalAs(t, ct, attackerProject, "hive:outsider", fmt.Sprintf("release_commitment=%d", commitID))
	voteForProposal(t, ct, raidID, "hive:outsider")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(raidID), nil, "hive:outsider", true, uint(1_000_000_000), afterVoting)
	execRes, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", raidID)), nil, "hive:outsider", false, uint(1_000_000_000), afterVoting)
	if !strings.Contains(execRes.Ret, "belongs to project") {
		t.Fatalf("expected cross-project abort, got %q", execRes.Ret)
	}
	if got := committedOf(t, ct, victimProject, "hive"); got != 6000 {
		t.Fatalf("victim commitment was disturbed: %d", got)
	}
}

// TestSelfSettlementIsRejected blocks the shape that would make the second vote
// optional: one proposal that both commits and releases, which meta-key sort
// order would otherwise execute back to back as a plain payout.
func TestSelfSettlementIsRejected(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")

	// The proposal's own id is needed inside its own meta, so create it first with
	// a placeholder to learn the id... which is impossible. Instead, target the id
	// the proposal is about to receive: proposal ids are sequential, so create a
	// throwaway to read the counter, then aim one past it.
	probe := createSimpleProposal(t, ct, projectID, "1")
	selfID := probe + 1

	id := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("commit_funds=hive:someoneelse:2.000:hive;release_commitment=%d", selfID))
	if id != selfID {
		t.Skipf("proposal id was %d, expected %d — sequential id assumption no longer holds", id, selfID)
	}
	ret := passProposalExpectExecuteFailure(t, ct, id, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "cannot settle the commitment created by the same proposal") {
		t.Fatalf("expected self-settlement abort, got %q", ret)
	}
	if got := committedOf(t, ct, projectID, "hive"); got != 0 {
		t.Fatalf("self-settling proposal leaked a reservation: %d", got)
	}
}

// =============================================================================
// Creation-time validation
//
// A malformed commitment must be rejected when the proposal is CREATED, not
// when it executes. A proposal that only fails to parse after it has passed is
// one the DAO approved and can never enact.
// =============================================================================

func assertProposalCreateRejected(t *testing.T, ct *test_utils.ContractTest, projectID uint64, meta string, wantMsg string) {
	t.Helper()
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"bad commitment",
		"should not be creatable",
		"1",
		"",
		"0",
		"",
		meta,
		"",
	}
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(strings.Join(fields, "|")), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, wantMsg) {
		t.Fatalf("expected %q in rejection, got %q", wantMsg, res.Ret)
	}
}

// TestCommitMalformedEntryRejectedAtCreation covers a missing asset field.
func TestCommitMalformedEntryRejectedAtCreation(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	assertProposalCreateRejected(t, ct, projectID, "commit_funds=hive:someoneelse:4.000", "payout entry missing asset")
}

// TestCommitZeroAmountRejectedAtCreation keeps a no-op reservation out of state.
func TestCommitZeroAmountRejectedAtCreation(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	assertProposalCreateRejected(t, ct, projectID, "commit_funds=hive:someoneelse:0:hive", "payout amount must be positive")
}

// TestCommitNegativeAmountRejectedAtCreation stops a reservation that would
// CREDIT the treasury on release.
func TestCommitNegativeAmountRejectedAtCreation(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	assertProposalCreateRejected(t, ct, projectID, "commit_funds=hive:someoneelse:-4.000:hive", "payout amount must be positive")
}

// TestCommitUnsupportedAssetRejectedAtCreation rejects assets the treasury and
// the transfer path cannot handle.
func TestCommitUnsupportedAssetRejectedAtCreation(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	assertProposalCreateRejected(t, ct, projectID, "commit_funds=hive:someoneelse:4.000:doge", "is not supported")
}

// TestCommitEmptyValueRejectedAtCreation rejects a commitment with no entries.
func TestCommitEmptyValueRejectedAtCreation(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	assertProposalCreateRejected(t, ct, projectID, "commit_funds=", "commit_funds requires at least one")
}

// TestCommitTooManyEntriesRejectedAtCreation enforces MaxCommitmentEntries, the
// bound on how much work one execute can be made to do.
func TestCommitTooManyEntriesRejectedAtCreation(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	entries := make([]string, 0, 51)
	for i := 0; i < 51; i++ {
		entries = append(entries, fmt.Sprintf("hive:user%d:0.001:hive", i))
	}
	assertProposalCreateRejected(t, ct, projectID, "commit_funds="+strings.Join(entries, ","), "cannot exceed 50 entries")
}

// TestReleaseNonNumericTargetRejectedAtCreation rejects a milestone vote whose
// target is not a proposal id.
func TestReleaseNonNumericTargetRejectedAtCreation(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	assertProposalCreateRejected(t, ct, projectID, "release_commitment=milestone-one", "release_commitment requires the id")
}

// TestCancelNonNumericTargetRejectedAtCreation mirrors the release check.
func TestCancelNonNumericTargetRejectedAtCreation(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	assertProposalCreateRejected(t, ct, projectID, "cancel_commitment=", "cancel_commitment requires the id")
}

// =============================================================================
// Interaction with existing rules
// =============================================================================

// TestCommitmentHoldsBeneficiaryPayoutLock checks a pending commitment keeps its
// beneficiary in the project, the same way a passed payout does — and that
// settling it lets them go.
func TestCommitmentHoldsBeneficiaryPayoutLock(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")
	commitID := commitProposal(t, ct, projectID, "hive:someoneelse:4.000:hive")

	// A pending commitment blocks the beneficiary's removal via kick_member,
	// which is the lock's observable effect.
	kickID := createPollProposal(t, ct, projectID, "1", "", "kick_member=hive:someoneelse")
	ret := passProposalExpectExecuteFailure(t, ct, kickID, "hive:someone", "hive:someoneelse")
	if !strings.Contains(ret, "active payout pending") {
		t.Fatalf("expected payout-lock abort, got %q", ret)
	}

	// Cancelling the commitment releases the lock.
	cancelID := createPollProposal(t, ct, projectID, "1", "", fmt.Sprintf("cancel_commitment=%d", commitID))
	passProposal(t, ct, cancelID, "hive:someone", "hive:someoneelse")

	kickAgain := createPollProposal(t, ct, projectID, "1", "", "kick_member=hive:someoneelse")
	passProposal(t, ct, kickAgain, "hive:someone", "hive:someoneelse")
}

// TestCommitmentRejectedWhilePaused confirms commitment actions do not inherit
// the pause exemption. outcomeIsPauseSafe only lets a lone pause/ownership meta
// key through, and a commitment moves funds.
func TestCommitmentRejectedWhilePaused(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")

	// The proposal has to be created and voted BEFORE the freeze: proposal_create
	// is itself blocked while paused. This is also the realistic shape of the
	// guard — a commitment the DAO approved before an emergency pause must not
	// move funds during it.
	id := createPollProposal(t, ct, projectID, "1", "", "commit_funds=hive:someoneelse:4.000:hive")
	voteForProposal(t, ct, id, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(id), nil, "hive:someone", true, uint(1_000_000_000), afterVoting)

	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|1", projectID)), nil, "hive:someone", true, uint(1_000_000_000))

	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", id)), nil, "hive:someone", false, uint(1_000_000_000), afterVoting)
	ret := res.Ret
	if !strings.Contains(ret, "paused") {
		t.Fatalf("expected paused abort, got %q", ret)
	}
	if got := committedOf(t, ct, projectID, "hive"); got != 0 {
		t.Fatalf("commit executed while paused: %d", got)
	}
}

// TestRejectedCommitProposalReservesNothing confirms a commitment the membership
// VOTED DOWN never reaches state.
func TestRejectedCommitProposalReservesNothing(t *testing.T) {
	ct, projectID := setupCommitmentProject(t, "10.000")

	id := createPollProposal(t, ct, projectID, "1", "", "commit_funds=hive:someoneelse:4.000:hive")
	// Vote option 0 ("no") from both members.
	noPayload := PayloadString(fmt.Sprintf("%d|0", id))
	CallContract(t, ct, "proposals_vote", noPayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", noPayload, nil, "hive:someoneelse", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(id), nil, "hive:someone", true, uint(1_000_000_000), afterVoting)
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", id)), nil, "hive:someone", false, uint(1_000_000_000), afterVoting)

	if got := committedOf(t, ct, projectID, "hive"); got != 0 {
		t.Fatalf("rejected proposal reserved %d", got)
	}
	if rec := commitmentRecord(ct, id); rec != "" {
		t.Fatalf("rejected proposal wrote a commitment record: %q", rec)
	}
}
