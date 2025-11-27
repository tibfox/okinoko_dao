package contract_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"vsc-node/lib/test_utils"
	"vsc-node/modules/db/vsc/contracts"
	ledgerDb "vsc-node/modules/db/vsc/ledger"
)

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
		"0",                  // project id
		"prpsl",              // name
		"desc",               // description
		"24",                 // duration
		"",                   // options -> default yes/no
		"0",                  // is poll
		"hive:someoneelse:1", // payouts
		"",                   // outcome meta
		"asd",                // metadata
	}
	proposalPayload := strings.Join(proposalFields, "|")

	CallContract(t, ct, "proposal_create", PayloadString(proposalPayload), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}}}, "hive:someone", true, uint(1_000_000_000))
}

func TestProjectCreateRequiresIntent(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "no valid transfer intent") {
		t.Fatalf("expected missing intent rejection, got %q", res.Ret)
	}
}

func TestProposalLifecycle(t *testing.T) {
	ct := SetupContractTest()

	projectFields := defaultProjectFields()
	projectFields[5] = "1"
	projectFields[6] = "10"
	projectFields[7] = "10"
	projectFields[8] = "1"
	projectFields[9] = "1"
	projectPayload := strings.Join(projectFields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(projectPayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	proposalFields := []string{
		strconv.FormatUint(projectID, 10),
		"upgrade node infra",
		"upgrade description",
		"1",
		"",
		"0",
		"",
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
}

func TestAddFundsToTreasury(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.500"), "hive:someone", true, uint(1_000_000_000))
}

func TestAddStakeFundsFailsInDemocracy(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "0")
	payload := fmt.Sprintf("%d|true", projectID)
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.500"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "cannot add member stake") {
		t.Fatalf("expected democratic stake rejection, got %q", res.Ret)
	}
}

func TestAddStakeFundsSucceedsInStakeSystem(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	payload := fmt.Sprintf("%d|true", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.750"), "hive:someone", true, uint(1_000_000_000))
}

func TestAddFundsRejectsWrongAsset(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := fmt.Sprintf("%d|false", projectID)
	intents := []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "0.250", "token": "hbd"}}}
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(payload), intents, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "invalid asset") {
		t.Fatalf("expected invalid asset rejection, got %q", res.Ret)
	}
}

func TestProjectJoinRejectsWrongAsset(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", payload, transferIntentWithToken("1.000", "hbd"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "invalid asset") {
		t.Fatalf("expected invalid asset rejection, got %q", res.Ret)
	}
}

func TestAddStakeFundsRequiresMembership(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	payload := fmt.Sprintf("%d|true", projectID)
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.500"), "hive:outsider", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "is not a member") {
		t.Fatalf("expected membership check, got %q", res.Ret)
	}
}

func TestVoteCanBeUpdatedBeforeTally(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "")
	voteYes := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", voteYes, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", voteYes, nil, "hive:someoneelse", true, uint(1_000_000_000))
	voteNo := PayloadString(fmt.Sprintf("%d|0", proposalID))
	CallContract(t, ct, "proposals_vote", voteNo, nil, "hive:someone", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "proposal is") {
		t.Fatalf("expected proposal execution to fail due to updated vote, got %q", res.Ret)
	}
}

func TestProjectJoinSuccess(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	fmt.Printf("Joining project %d\n", projectID)
	CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
}

func TestProjectJoinRequiresIntent(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	res, _, _ := CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "no valid transfer intent") {
		t.Fatalf("expected join intent check, got %q", res.Ret)
	}
}

func TestProjectTransferOwnership(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	payload := PayloadString(fmt.Sprintf("%d|%s", projectID, "hive:someoneelse"))
	CallContract(t, ct, "project_transfer", payload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someoneelse", true, uint(1_000_000_000))
}

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

func TestProjectPauseRequiresOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	res, _, _ := CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only owner") {
		t.Fatalf("expected pause rejection for non-owner, got %q", res.Ret)
	}
}

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

func TestProposalRequiresMembership(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, transferIntent("1.000"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only members") {
		t.Fatalf("expected proposal creation rejection for non-member, got %q", res.Ret)
	}
}

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

func TestProposalCreationRequiresCostIntent(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "no valid transfer intent") {
		t.Fatalf("expected intent requirement for proposal cost, got %q", res.Ret)
	}
}

func TestProposalCreationRejectsInsufficientCost(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, transferIntent("0.500"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "proposal cost requires at least") {
		t.Fatalf("expected insufficient cost rejection, got %q", res.Ret)
	}
}

func TestProposalCreationRejectsWrongAssetForCost(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", payload, transferIntentWithToken("1.000", "hbd"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "invalid asset") {
		t.Fatalf("expected proposal cost asset validation, got %q", res.Ret)
	}
}

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

func TestProposalEarlyTallyFails(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "2")
	res, _, _ := CallContract(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "still running") {
		t.Fatalf("expected tally to fail with running proposal, got %q", res.Ret)
	}
}

func TestProjectLeaveFlow(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	idStr := strconv.FormatUint(projectID, 10)
	CallContract(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", true, uint(1_000_000_000))
	CallContractAt(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

func TestProjectLeaveCooldown(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	idStr := strconv.FormatUint(projectID, 10)
	CallContract(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", true, uint(1_000_000_000))
	res, _, _ := CallContract(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "cooldown not passed") {
		t.Fatalf("expected cooldown check, got %q", res.Ret)
	}
}

func TestVoteRejectedForLateJoiner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	payload := PayloadString(strings.Join(simpleProposalFields(projectID, "24"), "|"))
	res, _, _ := CallContractAt(t, ct, "proposal_create", payload, transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-03T00:00:00")
	proposalID := parseCreatedID(t, res.Ret, "proposal")
	CallContractAt(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000), "2025-09-04T00:00:00")
	CallContractAt(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("1.000"), "hive:member2", true, uint(1_000_000_000), "2025-09-04T00:00:00")
	payload = PayloadString(fmt.Sprintf("%d|1", proposalID))
	res, _, _ = CallContract(t, ct, "proposals_vote", payload, nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "created before joining") {
		t.Fatalf("expected vote rejection for late joiner, got %q", res.Ret)
	}
}

func TestProjectLeaveBlockedDuringPayoutProposal(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	addTreasuryFunds(t, ct, projectID, "0.500")
	proposalID := createPollProposal(t, ct, projectID, "24", "hive:someoneelse:0.500", "")
	voteForProposal(t, ct, proposalID, "hive:someone")
	idStr := strconv.FormatUint(projectID, 10)
	res, _, _ := CallContract(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "active proposal requesting funds") {
		t.Fatalf("expected leave rejection due to active payout, got %q", res.Ret)
	}
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContract(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", true, uint(1_000_000_000))
}

func TestProposalMetaUpdateLeaveCooldown(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_leaveCooldown=0")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:member2")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	idStr := strconv.FormatUint(projectID, 10)
	CallContract(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", true, uint(1_000_000_000))
	CallContract(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", true, uint(1_000_000_000))
}

func TestProposalMetaUpdateThreshold(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:member2")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_threshold=90")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:member2")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	limitedProposal := createSimpleProposal(t, ct, projectID, "1")
	votePayload := PayloadString(fmt.Sprintf("%d|1", limitedProposal))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(limitedProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", limitedProposal)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T02:00:00")
	if !strings.Contains(res.Ret, "proposal is") {
		t.Fatalf("expected execution rejection due to high threshold, got %q", res.Ret)
	}
}

func TestProposalMetaTogglePause(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "toggle_pause=1")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:member2")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(strings.Join(simpleProposalFields(projectID, "2"), "|")), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "project is paused") {
		t.Fatalf("expected paused project to reject proposal creation, got %q", res.Ret)
	}
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|false", projectID)), nil, "hive:someone", true, uint(1_000_000_000))
}
func TestProposalExecuteTransfersFunds2Members(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	addTreasuryFunds(t, ct, projectID, "0.600")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:0.500", "")
	votePayload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someoneelse", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

func TestProposalExecuteTransfersFunds3Members(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")
	addTreasuryFunds(t, ct, projectID, "0.600")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:0.500", "")
	votePayload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:member2", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

func TestProposalExecuteRequiresPassed(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	addTreasuryFunds(t, ct, projectID, "0.400")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:0.200", "")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "proposal is") {
		t.Fatalf("expected execute rejection for active proposal, got %q", res.Ret)
	}
}

func TestProposalExecuteInsufficientFunds(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")
	addTreasuryFunds(t, ct, projectID, "0.200")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:2.000", "")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:member2")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "insufficient funds") {
		t.Fatalf("expected insufficient funds rejection, got %q", res.Ret)
	}
}

func TestProposalExecuteBlockedWhenPaused(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	addTreasuryFunds(t, ct, projectID, "0.400")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:0.200", "")
	votePayload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someone", true, uint(1_000_000_000))
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "project is paused") {
		t.Fatalf("expected paused project rejection, got %q", res.Ret)
	}
}

func TestProposalCancelFlow(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:0.200", "")
	res, _, _ := CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "cancelled") {
		t.Fatalf("expected cancellation, got %q", res.Ret)
	}
	_, _, _ = CallContract(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000))
}

func TestProposalCancelRequiresCreatorOrOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "")
	res, _, _ := CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only creator or owner") {
		t.Fatalf("expected unauthorized cancel rejection, got %q", res.Ret)
	}
}

func TestProposalCancelOwnerRefundsCreator(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	CallContract(t, ct, "project_transfer", PayloadString(fmt.Sprintf("%d|%s", projectID, "hive:someoneelse")), nil, "hive:someone", true, uint(1_000_000_000))
	proposalID := createPollProposal(t, ct, projectID, "1", "", "")
	pre := ct.GetBalance("hive:someone", ledgerDb.AssetHive)
	CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someoneelse", true, uint(1_000_000_000))
	post := ct.GetBalance("hive:someone", ledgerDb.AssetHive)
	if post <= pre {
		t.Fatalf("expected proposal cost refund to creator")
	}
}

func TestProposalExecuteRespectsDelay(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[6] = "10"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T02:00:00")
	res2, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-03T03:00:00")
	if !strings.Contains(res2.Ret, "execution delay") {
		t.Fatalf("expected execution delay enforcement, got %q", res2.Ret)
	}
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T12:00:00")
}

func TestProposalMetaUpdateOwner(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "update_owner=hive:someoneelse")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someoneelse", true, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "paused") {
		t.Fatalf("new owner should control pause, got %q", res.Ret)
	}
}

func parseCreatedID(t *testing.T, ret string, entity string) uint64 {
	cleaned := strings.TrimSpace(ret)
	cleaned = strings.TrimPrefix(cleaned, "msg:")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		t.Fatalf("empty return when parsing %s id", entity)
	}
	id, err := strconv.ParseUint(cleaned, 10, 64)
	if err != nil {
		t.Fatalf("failed to parse %s id from %q: %v", entity, cleaned, err)
	}
	return id
}

func transferIntent(limit string) []contracts.Intent {
	return transferIntentWithToken(limit, "hive")
}

func transferIntentWithToken(limit string, token string) []contracts.Intent {
	return []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": limit, "token": token}}}
}

func joinProjectMember(t *testing.T, ct *test_utils.ContractTest, projectID uint64, user string) {
	CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("1.000"), user, true, uint(1_000_000_000))
}

func voteForProposal(t *testing.T, ct *test_utils.ContractTest, proposalID uint64, voters ...string) {
	payload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	for _, voter := range voters {
		CallContract(t, ct, "proposals_vote", payload, nil, voter, true, uint(1_000_000_000))
	}
}

func createDefaultProject(t *testing.T, ct *test_utils.ContractTest) uint64 {
	payload := strings.Join(defaultProjectFields(), "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

func createProjectWithVoting(t *testing.T, ct *test_utils.ContractTest, voting string) uint64 {
	fields := defaultProjectFields()
	fields[2] = voting
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

func createSimpleProposal(t *testing.T, ct *test_utils.ContractTest, projectID uint64, duration string) uint64 {
	payload := strings.Join(simpleProposalFields(projectID, duration), "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "proposal")
}

func simpleProposalFields(projectID uint64, duration string) []string {
	return []string{
		strconv.FormatUint(projectID, 10),
		"maintenance",
		"upgrade nodes",
		duration,
		"",
		"0",
		"",
		"",
		"",
	}
}

func createPollProposal(t *testing.T, ct *test_utils.ContractTest, projectID uint64, duration string, payouts string, meta string) uint64 {
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"payout",
		"treasury distribution",
		duration,
		"",
		"1",
		payouts,
		meta,
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "proposal")
}

func addTreasuryFunds(t *testing.T, ct *test_utils.ContractTest, projectID uint64, amount string) {
	payload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent(amount), "hive:someone", true, uint(1_000_000_000))
}

func defaultProjectFields() []string {
	return []string{
		"dao",
		"desc",
		"0",
		"50.001",
		"50.001",
		"1",
		"0",
		"10",
		"1",
		"1",
		"",
		"",
		"",
		"",
		"1",
		"",
	}
}
