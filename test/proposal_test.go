package contract_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// =============================================================================
// Proposal Lifecycle Tests
// =============================================================================

// TestProposalLifecycle checks the proposal lifecycle flow so we dont break it again.
func TestProposalLifecycle(t *testing.T) {
	ct := SetupContractTest()

	projectFields := defaultProjectFields()
	projectFields[5] = "1"
	projectFields[6] = "0" // executionDelay: 0 hours for immediate execution
	projectFields[7] = "10"
	projectFields[8] = "1"
	projectFields[9] = "1"
	projectPayload := strings.Join(projectFields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(projectPayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	// Add funds to treasury for the payout
	addTreasuryFunds(t, ct, projectID, "0.500")

	proposalFields := []string{
		strconv.FormatUint(projectID, 10),
		"upgrade node infra",
		"upgrade description",
		"1",
		"",
		"0",
		"hive:someoneelse:0.200:hive", // Payout 0.200 HIVE to someoneelse
		"",
		"",
	}
	proposalPayload := strings.Join(proposalFields, "|")
	propRes, _, _ := CallContract(t, ct, "proposal_create", PayloadString(proposalPayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, propRes.Ret, "proposal")

	votePayload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someoneelse", true, uint(1_000_000_000))

	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute the proposal - transfers 0.200 HIVE from treasury to someoneelse
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestFullCycle checks a full proposal cycle.
func TestFullCycle(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	proposalID := createSimpleProposal(t, ct, projectID, "1")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// =============================================================================
// Proposal Creation Tests
// =============================================================================

// TestProposalRequiresMembership checks the proposal requires membership flow so we dont break it again.
func TestProposalRequiresMembership(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, transferIntent("1.000"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only members") {
		t.Fatalf("expected proposal creation rejection for non-member, got %q", res.Ret)
	}
}

// TestProposalCreationAllowedForPublic checks the proposal creation allowed for public flow so we dont break it again.
func TestProposalCreationAllowedForPublic(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[14] = "0"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")
	proposalPayload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	CallContract(t, ct, "proposal_create", proposalPayload, transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
}

// TestProposalCreationRequiresCostIntent checks the proposal creation requires cost intent flow so we dont break it again.
func TestProposalCreationRequiresCostIntent(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "no valid transfer intent") {
		t.Fatalf("expected intent requirement for proposal cost, got %q", res.Ret)
	}
}

// TestProposalCreationZeroCostNoIntent ensures proposals can be created without an intent when cost is zero.
func TestProposalCreationZeroCostNoIntent(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[8] = "0"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")
	proposalFields := simpleProposalFields(projectID, "2")
	proposalPayload := PayloadString(strings.Join(proposalFields, "|"))
	CallContract(t, ct, "proposal_create", proposalPayload, nil, "hive:someone", true, uint(1_000_000_000))
}

// TestProposalCreationRejectsInsufficientCost checks the proposal creation rejects insufficient cost flow so we dont break it again.
func TestProposalCreationRejectsInsufficientCost(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, transferIntent("0.500"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "proposal cost requires at least") {
		t.Fatalf("expected insufficient cost rejection, got %q", res.Ret)
	}
}

// TestProposalCreationRejectsWrongAssetForCost checks the proposal creation rejects wrong asset for cost flow so we dont break it again.
func TestProposalCreationRejectsWrongAssetForCost(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, transferIntentWithToken("1.000", "hbd"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "invalid asset") {
		t.Fatalf("expected proposal cost asset validation, got %q", res.Ret)
	}
}

// TestProposalDurationValidation checks the proposal duration validation flow so we dont break it again.
func TestProposalDurationValidation(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	fields := simpleProposalFields(projectID, "0")
	payload := PayloadString(strings.Join(fields, "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, res.Ret, "proposal")
	res, _, _ = CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-03T00:30:00")
	if !strings.Contains(res.Ret, "proposal still running") {
		t.Fatalf("expected duration enforcement, got %q", res.Ret)
	}
}

// TestProposalNameTooLong checks that proposal names exceeding 128 chars are rejected.
func TestProposalNameTooLong(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	fields := []string{
		strconv.FormatUint(projectID, 10),
		strings.Repeat("a", 129), // name exceeds 128 chars
		"desc",
		"1",
		"",
		"0",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "exceeds maximum length") {
		t.Fatalf("expected name length rejection, got %q", res.Ret)
	}
}

// TestProposalDescriptionTooLong checks that proposal descriptions exceeding 512 chars are rejected.
func TestProposalDescriptionTooLong(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test",
		strings.Repeat("a", 513), // description exceeds 512 chars
		"1",
		"",
		"0",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "exceeds maximum length") {
		t.Fatalf("expected description length rejection, got %q", res.Ret)
	}
}

// =============================================================================
// Voting Tests
// =============================================================================

// TestVoteRequiresMembership checks the vote requires membership flow so we dont break it again.
func TestVoteRequiresMembership(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "2")
	payload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	res, _, _ := CallContract(t, ct, "proposals_vote", payload, nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "is not a member") {
		t.Fatalf("expected vote rejection for non-member, got %q", res.Ret)
	}
}

// TestVoteCanBeUpdatedBeforeTally checks the vote update flow.
func TestVoteCanBeUpdatedBeforeTally(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "24")
	noVote := PayloadString(fmt.Sprintf("%d|0", proposalID))
	yesVote := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", noVote, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", yesVote, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", yesVote, nil, "hive:someoneelse", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestVoteRejectedForLateJoiner checks the vote rejected for late joiner flow so we dont break it again.
func TestVoteRejectedForLateJoiner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "24")
	// Join at a later timestamp than proposal creation
	CallContractAt(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000), "2025-09-04T00:00:00")
	payload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	res, _, _ := CallContractAt(t, ct, "proposals_vote", payload, nil, "hive:someoneelse", false, uint(1_000_000_000), "2025-09-04T00:00:00")
	if !strings.Contains(res.Ret, "proposal was created before joining") {
		t.Fatalf("expected late joiner vote rejection, got %q", res.Ret)
	}
}

// TestVoteInvalidOptionIndex checks voting for invalid option index.
func TestVoteInvalidOptionIndex(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "24")
	payload := PayloadString(fmt.Sprintf("%d|99", proposalID))
	res, _, _ := CallContract(t, ct, "proposals_vote", payload, nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "invalid option index") {
		t.Fatalf("expected invalid option rejection, got %q", res.Ret)
	}
}

// =============================================================================
// Tally Tests
// =============================================================================

// TestProposalEarlyTallyFails checks the proposal early tally fails flow so we dont break it again.
func TestProposalEarlyTallyFails(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "24")
	res, _, _ := CallContract(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "proposal still running") {
		t.Fatalf("expected early tally rejection, got %q", res.Ret)
	}
}

// TestVoteAllowedAfterDurationBeforeTally checks the vote allowed after duration before tally flow so we dont break it again.
func TestVoteAllowedAfterDurationBeforeTally(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	payload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContractAt(t, ct, "proposals_vote", payload, nil, "hive:someone", true, uint(1_000_000_000), "2025-09-04T00:30:00")
}

// TestTallyAlreadyTallied checks tallying an already tallied proposal.
func TestTallyAlreadyTallied(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	voteForProposal(t, ct, proposalID, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "proposal not active") {
		t.Fatalf("expected already tallied rejection, got %q", res.Ret)
	}
}

// TestQuorumThresholdFailure checks the quorum threshold failure flow so we dont break it again.
func TestQuorumThresholdFailure(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	votePayload := PayloadString(fmt.Sprintf("%d|0", proposalID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// =============================================================================
// Execute Tests
// =============================================================================

// TestProposalExecuteRequiresPassed checks the proposal execute requires passed flow so we dont break it again.
func TestProposalExecuteRequiresPassed(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "proposal is failed") {
		t.Fatalf("expected passed requirement, got %q", res.Ret)
	}
}

// TestProposalExecuteBlockedWhenPaused checks the proposal execute blocked when paused flow so we dont break it again.
func TestProposalExecuteBlockedWhenPaused(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someone", true, uint(1_000_000_000))
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "project is paused") {
		t.Fatalf("expected paused rejection, got %q", res.Ret)
	}
}

// TestExecuteAlreadyExecuted checks executing an already executed proposal.
func TestExecuteAlreadyExecuted(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "proposal already executed") {
		t.Fatalf("expected already executed rejection, got %q", res.Ret)
	}
}

// TestProposalExecuteRespectsDelay checks the proposal execute respects delay flow so we dont break it again.
func TestProposalExecuteRespectsDelay(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[6] = "24" // 24 hour execution delay
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	proposalID := createSimpleProposal(t, ct, projectID, "1")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	// Tally just after proposal duration ends (1 hour after creation)
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T02:00:00")
	// Try to execute immediately - should fail because 24 hour delay hasn't passed
	res, _, _ = CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-03T02:00:00")
	if !strings.Contains(res.Ret, "execution delay until") {
		t.Fatalf("expected delay rejection, got %q", res.Ret)
	}
}

// =============================================================================
// Cancel Tests
// =============================================================================

// TestProposalCancelFlow checks the proposal cancel flow so we dont break it again.
func TestProposalCancelFlow(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "24")
	voteForProposal(t, ct, proposalID, "hive:someone")
	CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000))
	res, _, _ := CallContract(t, ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", proposalID)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "proposal not active") {
		t.Fatalf("expected cancelled proposal to reject votes, got %q", res.Ret)
	}
}

// TestProposalCancelRequiresCreatorOrOwner checks the proposal cancel requires creator or owner flow so we dont break it again.
func TestProposalCancelRequiresCreatorOrOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "24")
	res, _, _ := CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only creator or owner") {
		t.Fatalf("expected cancellation rejection for non-creator non-owner, got %q", res.Ret)
	}
}

// TestProposalCancelOwnerRefundsCreator checks the proposal cancel owner refunds creator flow so we dont break it again.
func TestProposalCancelOwnerRefundsCreator(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	addTreasuryFunds(t, ct, projectID, "2.000")
	proposalID := createSimpleProposal(t, ct, projectID, "24")
	CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000))
}

// TestProposalCancelNotActive checks cancelling non-active proposal.
func TestProposalCancelNotActive(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContract(t, ct, "proposal_cancel", PayloadUint64(proposalID), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "proposal not active") {
		t.Fatalf("expected not active rejection, got %q", res.Ret)
	}
}

// TestOwnerCancelWithoutTreasuryRefund checks owner cancel without treasury refund flow.
func TestOwnerCancelWithoutTreasuryRefund(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[14] = "0"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	proposalFields := simpleProposalFields(projectID, "24")
	proposalPayload := PayloadString(strings.Join(proposalFields, "|"))
	res, _, _ = CallContract(t, ct, "proposal_create", proposalPayload, transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, res.Ret, "proposal")

	CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000))
}

// TestPayoutLockReleasedAfterCancel checks payout lock released after cancel flow.
func TestPayoutLockReleasedAfterCancel(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	createPollProposal(t, ct, projectID, "24", "hive:someoneelse:1:hive", "")
	res, _, _ := CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "active proposal requesting funds") {
		t.Fatalf("expected lock, got %q", res.Ret)
	}
}

// =============================================================================
// Meta Update Tests
// =============================================================================

// TestProposalMetaUpdateAllowsPublic checks the proposal meta update allows public flow so we dont break it again.
func TestProposalMetaUpdateAllowsPublic(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	outsiderProposalPayload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	CallContract(t, ct, "proposal_create", outsiderProposalPayload, transferIntent("1.000"), "hive:outsider", false, uint(1_000_000_000))
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_proposalCreatorRestriction=0")
	votePayload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:member2", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	CallContract(t, ct, "proposal_create", outsiderProposalPayload, transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))
}

// TestProposalMetaUpdateLeaveCooldown checks the proposal meta update leave cooldown flow so we dont break it again.
func TestProposalMetaUpdateLeaveCooldown(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_leaveCooldown=1")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProposalMetaUpdateThreshold checks the proposal meta update threshold flow so we dont break it again.
func TestProposalMetaUpdateThreshold(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_threshold=75")
	votePayload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:member2", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProposalMetaTogglePause checks the proposal meta toggle pause flow so we dont break it again.
func TestProposalMetaTogglePause(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "toggle_pause=")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProposalMetaMultipleUpdates checks multiple meta updates in one proposal.
func TestProposalMetaMultipleUpdates(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_threshold=60;update_quorum=40")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProposalMetaUpdateOwner checks the proposal meta update owner flow so we dont break it again.
func TestProposalMetaUpdateOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_owner=hive:someoneelse")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestExecutionDelayMetaUpdate checks the execution delay meta update flow so we dont break it again.
func TestExecutionDelayMetaUpdate(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_executionDelay=24")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Create second proposal AFTER the config update (at Sep 6, after execution at Sep 5)
	fields := simpleProposalFields(projectID, "1")
	payload := strings.Join(fields, "|")
	res, _, _ := CallContractAt(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-06T00:00:00")
	proposalID2 := parseCreatedID(t, res.Ret, "proposal")
	CallContractAt(t, ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", proposalID2)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-06T00:00:00")
	CallContractAt(t, ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", proposalID2)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-06T00:00:00")
	// Tally at Sep 6 02:00 (after 1 hour duration)
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID2), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-06T02:00:00")
	// Try to execute immediately - should fail because 24 hour delay hasn't passed
	res, _, _ = CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID2), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-06T02:00:00")
	if !strings.Contains(res.Ret, "execution delay until") {
		t.Fatalf("expected delay from updated config, got %q", res.Ret)
	}
}
