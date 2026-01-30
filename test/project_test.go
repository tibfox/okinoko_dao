package contract_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"vsc-node/modules/db/vsc/contracts"
)

// =============================================================================
// Contract Initialization Tests
// =============================================================================

// TestContractInitPublic verifies that contract_init works with public mode
func TestContractInitPublic(t *testing.T) {
	ct := SetupContractTestUninitialized()
	res, _, _ := CallContract(t, ct, "contract_init", PayloadString("public"), nil, ownerAddress, true, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "public") {
		t.Fatalf("expected public init message, got %q", res.Ret)
	}
}

// TestContractInitOwnerOnly verifies that contract_init works with owner-only mode
func TestContractInitOwnerOnly(t *testing.T) {
	ct := SetupContractTestUninitialized()
	res, _, _ := CallContract(t, ct, "contract_init", PayloadString("owner-only"), nil, ownerAddress, true, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "owner-only") {
		t.Fatalf("expected owner-only init message, got %q", res.Ret)
	}
}

// TestContractInitOnlyOnce verifies that contract_init can only be called once
func TestContractInitOnlyOnce(t *testing.T) {
	ct := SetupContractTest() // Already initialized
	res, _, _ := CallContract(t, ct, "contract_init", PayloadString("public"), nil, ownerAddress, false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "already initialized") {
		t.Fatalf("expected already initialized error, got %q", res.Ret)
	}
}

// TestUninitializedContractRejects verifies that functions fail before init
func TestUninitializedContractRejects(t *testing.T) {
	ct := SetupContractTestUninitialized()
	fields := defaultProjectFields()
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "not initialized") {
		t.Fatalf("expected not initialized error, got %q", res.Ret)
	}
}

// TestOwnerOnlyProjectCreation verifies that only owner can create projects in owner-only mode
func TestOwnerOnlyProjectCreation(t *testing.T) {
	ct := SetupContractTestOwnerOnly()
	fields := defaultProjectFields()
	payload := strings.Join(fields, "|")

	// Non-owner should fail
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only contract owner") {
		t.Fatalf("expected owner-only error, got %q", res.Ret)
	}

	// Owner should succeed
	CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), ownerAddress, true, uint(1_000_000_000))
}

// =============================================================================
// Project Creation Tests
// =============================================================================

// TestCreateProject checks the create project flow so we dont break it again.
func TestCreateProject(t *testing.T) {
	ct := SetupContractTest()

	fields := defaultProjectFields()
	fields[5] = "10"
	fields[6] = "10"
	fields[7] = "10"
	payload := strings.Join(fields, "|")
	CallContract(t, ct, "project_create", PayloadString(payload), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}}}, "hive:someone", true, uint(1_000_000_000))
	joinProjectMember(t, ct, uint64(0), "hive:someoneelse")
	proposalFields := []string{
		"0",                       // project id
		"prpsl",                   // name
		"desc",                    // description
		"24",                      // duration
		"",                        // options -> default yes/no
		"0",                       // is poll
		"hive:someoneelse:1:hive", // payouts
		"",                        // outcome meta
		"asd",                     // metadata
	}
	proposalPayload := strings.Join(proposalFields, "|")

	CallContract(t, ct, "proposal_create", PayloadString(proposalPayload), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}}}, "hive:someone", true, uint(1_000_000_000))
}

// TestProjectCreateRequiresIntent checks the project create requires intent flow so we dont break it again.
func TestProjectCreateRequiresIntent(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "no valid transfer intent") {
		t.Fatalf("expected missing intent rejection, got %q", res.Ret)
	}
}

// TestProjectNameTooLong checks that project names exceeding 128 chars are rejected.
func TestProjectNameTooLong(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[0] = strings.Repeat("a", 129) // name exceeds 128 chars
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "exceeds maximum length") {
		t.Fatalf("expected name length rejection, got %q", res.Ret)
	}
}

// TestProjectDescriptionTooLong checks that project descriptions exceeding 512 chars are rejected.
func TestProjectDescriptionTooLong(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[1] = strings.Repeat("a", 513) // description exceeds 512 chars
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "exceeds maximum length") {
		t.Fatalf("expected description length rejection, got %q", res.Ret)
	}
}

// =============================================================================
// Project Join/Leave Tests
// =============================================================================

// TestProjectJoinSuccess checks the project join success flow so we dont break it again.
func TestProjectJoinSuccess(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	CallContract(t, ct, "project_join", payload, transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
}

// TestProjectJoinRequiresIntent checks the project join requires intent flow so we dont break it again.
func TestProjectJoinRequiresIntent(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", payload, nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "no valid transfer intent") {
		t.Fatalf("expected missing intent rejection, got %q", res.Ret)
	}
}

// TestProjectJoinRejectsWrongAsset ensures joining with wrong asset type fails.
func TestProjectJoinRejectsWrongAsset(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", payload, transferIntentWithToken("1.000", "hbd"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "invalid asset") {
		t.Fatalf("expected wrong asset rejection, got %q", res.Ret)
	}
}

// TestJoinProjectAlreadyMember checks joining when already a member.
func TestJoinProjectAlreadyMember(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", payload, transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "already a member") {
		t.Fatalf("expected already member rejection, got %q", res.Ret)
	}
}

// TestProjectLeaveFlow checks the project leave flow so we dont break it again.
func TestProjectLeaveFlow(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-06T00:00:00")
}

// TestProjectLeaveCooldown checks the project leave cooldown flow so we dont break it again.
func TestProjectLeaveCooldown(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", false, uint(1_000_000_000), "2025-09-05T01:00:00")
	if !strings.Contains(res.Ret, "cooldown not passed") {
		t.Fatalf("expected cooldown rejection, got %q", res.Ret)
	}
}

// TestLeaveProjectNotMember checks leaving when not a member.
func TestLeaveProjectNotMember(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	res, _, _ := CallContract(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "is not a member") {
		t.Fatalf("expected not member rejection, got %q", res.Ret)
	}
}

// TestLeaveProjectOwnerMustTransferFirst checks owner cannot leave without transfer.
func TestLeaveProjectOwnerMustTransferFirst(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	res, _, _ := CallContract(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "owner must transfer") {
		t.Fatalf("expected owner transfer requirement, got %q", res.Ret)
	}
}

// TestProjectLeaveBlockedDuringPayoutProposal checks the project leave blocked during payout proposal flow so we dont break it again.
func TestProjectLeaveBlockedDuringPayoutProposal(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	createPollProposal(t, ct, projectID, "24", "hive:someoneelse:1:hive", "")
	res, _, _ := CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "active proposal requesting funds") {
		t.Fatalf("expected payout lock block, got %q", res.Ret)
	}
}

// TestMemberCanLeaveAfterVoting ensures members can leave after voting on proposals.
func TestMemberCanLeaveAfterVoting(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	voteForProposal(t, ct, proposalID, "hive:someoneelse")
	CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// =============================================================================
// Project Transfer Ownership Tests
// =============================================================================

// TestProjectTransferOwnership checks the project transfer ownership flow so we dont break it again.
func TestProjectTransferOwnership(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	payload := PayloadString(fmt.Sprintf("%d|%s", projectID, "hive:someoneelse"))
	CallContract(t, ct, "project_transfer", payload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someoneelse", true, uint(1_000_000_000))
}

// TestProjectTransferRequiresOwner checks the project transfer requires owner flow so we dont break it again.
func TestProjectTransferRequiresOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	payload := PayloadString(fmt.Sprintf("%d|%s", projectID, "hive:member2"))
	res, _, _ := CallContract(t, ct, "project_transfer", payload, nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only owner") {
		t.Fatalf("expected transfer rejection for non-owner, got %q", res.Ret)
	}
}

// TestTransferOwnershipNotOwner checks non-owner cannot transfer.
func TestTransferOwnershipNotOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	payload := PayloadString(fmt.Sprintf("%d|hive:member2", projectID))
	res, _, _ := CallContract(t, ct, "project_transfer", payload, nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only owner") {
		t.Fatalf("expected not owner rejection, got %q", res.Ret)
	}
}

// TestTransferRequiresMemberTarget checks the transfer requires member target flow so we dont break it again.
func TestTransferRequiresMemberTarget(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(fmt.Sprintf("%d|hive:someoneelse", projectID))
	res, _, _ := CallContract(t, ct, "project_transfer", payload, nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "new owner must be a member") {
		t.Fatalf("expected member requirement for transfer, got %q", res.Ret)
	}
}

// =============================================================================
// Project Pause Tests
// =============================================================================

// TestProjectPauseRequiresOwner checks the project pause requires owner flow so we dont break it again.
func TestProjectPauseRequiresOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	res, _, _ := CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only owner") {
		t.Fatalf("expected pause rejection for non-owner, got %q", res.Ret)
	}
}

// TestProjectPauseBlocksProposals checks the project pause blocks proposals flow so we dont break it again.
func TestProjectPauseBlocksProposals(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someone", true, uint(1_000_000_000))
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|")), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "project is paused") {
		t.Fatalf("expected paused project to reject proposal creation, got %q", res.Ret)
	}
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|false", projectID)), nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposal_create", PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|")), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestTogglePauseProposalWhilePaused checks toggle pause while paused flow so we dont break it again.
func TestTogglePauseProposalWhilePaused(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	proposalID := createPollProposal(t, ct, projectID, "1", "", "toggle_pause=")
	votePayload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:member2", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someone", true, uint(1_000_000_000))

	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProjectFundsAllowedWhilePaused checks adding funds while paused.
func TestProjectFundsAllowedWhilePaused(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someone", true, uint(1_000_000_000))
	addTreasuryFunds(t, ct, projectID, "1.000")
}

// =============================================================================
// Stake/Voting System Tests
// =============================================================================

// TestDemocraticJoinExactStake checks the democratic join exact stake flow so we dont break it again.
func TestDemocraticJoinExactStake(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[2] = "0"
	fields[9] = "2"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("2.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")
	CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("2.000"), "hive:someoneelse", true, uint(1_000_000_000))
}

// TestStakeJoinMinimumEnforced checks the stake join minimum enforced flow so we dont break it again.
func TestStakeJoinMinimumEnforced(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[2] = "1"
	fields[9] = "2"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("2.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")
	res, _, _ = CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("1.500"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "stake too low") {
		t.Fatalf("expected minimum stake rejection, got %q", res.Ret)
	}
}

// TestAddStakeFundsFailsInDemocracy ensures stake funding fails in democratic projects.
func TestAddStakeFundsFailsInDemocracy(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := fmt.Sprintf("%d|true", projectID)
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "cannot add member stake") {
		t.Fatalf("expected stake addition rejection, got %q", res.Ret)
	}
}

// TestAddStakeFundsSucceedsInStakeSystem ensures stake funding works in stake-weighted projects.
func TestAddStakeFundsSucceedsInStakeSystem(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	payload := fmt.Sprintf("%d|true", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestAddStakeFundsRequiresMembership ensures stake funding requires membership.
func TestAddStakeFundsRequiresMembership(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	payload := fmt.Sprintf("%d|true", projectID)
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "not a member") {
		t.Fatalf("expected membership requirement, got %q", res.Ret)
	}
}

// =============================================================================
// Kick Member Tests
// =============================================================================

// TestKickMemberSuccess checks the kick member via proposal flow.
func TestKickMemberSuccess(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	// Create proposal to kick someoneelse
	proposalID := createPollProposal(t, ct, projectID, "1", "", "kick_member=hive:someoneelse")

	// Vote yes (owner + member2)
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:member2")

	// Tally and execute
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Verify kicked member cannot vote anymore
	newProposalID := createSimpleProposal(t, ct, projectID, "1")
	res, _, _ := CallContract(t, ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", newProposalID)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "not a member") {
		t.Fatalf("expected kicked member to be rejected, got %q", res.Ret)
	}
}

// TestKickMemberCannotKickOwner ensures the owner cannot be kicked.
func TestKickMemberCannotKickOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	// Create proposal to kick the owner
	proposalID := createPollProposal(t, ct, projectID, "1", "", "kick_member=hive:someone")

	// Vote yes
	voteForProposal(t, ct, proposalID, "hive:someoneelse", "hive:member2")

	// Tally
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute should fail
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "cannot kick project owner") {
		t.Fatalf("expected owner kick rejection, got %q", res.Ret)
	}
}

// TestKickMemberWithActivePayout ensures members with active payouts cannot be kicked.
func TestKickMemberWithActivePayout(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	// Create a payout proposal targeting someoneelse (creates payout lock)
	createPollProposal(t, ct, projectID, "24", "hive:someoneelse:1:hive", "")

	// Create proposal to kick someoneelse
	kickProposalID := createPollProposal(t, ct, projectID, "1", "", "kick_member=hive:someoneelse")

	// Vote yes
	voteForProposal(t, ct, kickProposalID, "hive:someone", "hive:member2")

	// Tally
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(kickProposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute should fail due to active payout
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadUint64(kickProposalID), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "active payout pending") {
		t.Fatalf("expected payout lock rejection, got %q", res.Ret)
	}
}
