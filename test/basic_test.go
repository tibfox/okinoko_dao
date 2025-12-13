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

// TestProposalLifecycle checks the proposal lifecycle flow so we dont break it again.
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

// TestAddFundsToTreasury checks the add funds to treasury flow so we dont break it again.
func TestAddFundsToTreasury(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.500"), "hive:someone", true, uint(1_000_000_000))
}

// TestAddStakeFundsFailsInDemocracy checks the add stake funds fails in democracy flow so we dont break it again.
func TestAddStakeFundsFailsInDemocracy(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "0")
	payload := fmt.Sprintf("%d|true", projectID)
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.500"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "cannot add member stake") {
		t.Fatalf("expected democratic stake rejection, got %q", res.Ret)
	}
}

// TestAddStakeFundsSucceedsInStakeSystem checks the add stake funds succeeds in stake system flow so we dont break it again.
func TestAddStakeFundsSucceedsInStakeSystem(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	payload := fmt.Sprintf("%d|true", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.750"), "hive:someone", true, uint(1_000_000_000))
}

// TestAddFundsRejectsWrongAsset checks the add funds rejects wrong asset flow so we dont break it again.
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

// TestProjectJoinRejectsWrongAsset checks the project join rejects wrong asset flow so we dont break it again.
func TestProjectJoinRejectsWrongAsset(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", payload, transferIntentWithToken("1.000", "hbd"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "invalid asset") {
		t.Fatalf("expected invalid asset rejection, got %q", res.Ret)
	}
}

// TestAddStakeFundsRequiresMembership checks the add stake funds requires membership flow so we dont break it again.
func TestAddStakeFundsRequiresMembership(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	payload := fmt.Sprintf("%d|true", projectID)
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.500"), "hive:outsider", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "is not a member") {
		t.Fatalf("expected membership check, got %q", res.Ret)
	}
}

// TestVoteCanBeUpdatedBeforeTally checks the vote can be updated before tally flow so we dont break it again.
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

// TestProjectJoinSuccess checks the project join success flow so we dont break it again.
func TestProjectJoinSuccess(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	fmt.Printf("Joining project %d\n", projectID)
	CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
}

// TestProjectJoinRequiresIntent checks the project join requires intent flow so we dont break it again.
func TestProjectJoinRequiresIntent(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	res, _, _ := CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "no valid transfer intent") {
		t.Fatalf("expected join intent check, got %q", res.Ret)
	}
}

// TestWhitelistDoesNotBypassNFT ensures entries do not override nft requirements.
func TestWhitelistDoesNotBypassNFT(t *testing.T) {
	ct := SetupContractTest()
	projectID := createNftGatedProject(t, ct)
	whitelistPayload := fmt.Sprintf("%d|%s", projectID, "hive:someoneelse")
	CallContract(t, ct, "project_whitelist_add", PayloadString(whitelistPayload), nil, "hive:someone", true, uint(1_000_000_000))
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", payload, transferIntent("1.000"), "hive:someoneelse", false, uint(1_000_000_000))
	if !containsNFTGateMessage(res.Ret) {
		t.Fatalf("expected nft ownership rejection despite whitelist, got %q", res.Ret)
	}
}

// TestWhitelistRemovalBlocksJoin verifies entries are required whenever whitelist enforcement is on.
func TestWhitelistRemovalBlocksJoin(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[len(fields)-1] = "1" // whitelist enforcement
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	addPayload := fmt.Sprintf("%d|%s;%s", projectID, "hive:outsider", "hive:member2")
	CallContract(t, ct, "project_whitelist_add", PayloadString(addPayload), nil, "hive:someone", true, uint(1_000_000_000))
	removePayload := fmt.Sprintf("%d|%s", projectID, "hive:outsider")
	CallContract(t, ct, "project_whitelist_remove", PayloadString(removePayload), nil, "hive:someone", true, uint(1_000_000_000))

	projPayload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ = CallContract(t, ct, "project_join", projPayload, transferIntent("1.000"), "hive:outsider", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "whitelist approval required") {
		t.Fatalf("expected whitelist removal to block outsider, got %q", res.Ret)
	}
	CallContract(t, ct, "project_join", projPayload, transferIntent("1.000"), "hive:member2", true, uint(1_000_000_000))
}

// TestWhitelistManagedByProposal confirms dao proposals can add/remove pending members.
func TestWhitelistManagedByProposal(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[len(fields)-1] = "1"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	addMeta := "whitelist_add=hive:someoneelse,hive:outsider"
	addProposal := createPollProposal(t, ct, projectID, "1", "", addMeta)
	votePayload := PayloadString(fmt.Sprintf("%d|1", addProposal))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(addProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(addProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T01:00:00")

	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))
	CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))

	removeMeta := "whitelist_remove=hive:someoneelse"
	removeProposal := createPollProposal(t, ct, projectID, "1", "", removeMeta)
	removeVote := PayloadString(fmt.Sprintf("%d|1", removeProposal))
	CallContract(t, ct, "proposals_vote", removeVote, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", removeVote, nil, "hive:outsider", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(removeProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(removeProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T01:00:00")

	res, _, _ = CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "whitelist approval required") {
		t.Fatalf("expected removal proposal to clear whitelist entry, got %q", res.Ret)
	}
}

// TestWhitelistDuplicateEntriesIgnoreMembers ensures duplicates and existing members are ignored gracefully.
func TestWhitelistDuplicateEntriesIgnoreMembers(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[len(fields)-1] = "1"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	dupPayload := fmt.Sprintf("%d|%s;%s;%s", projectID, "hive:someone", "hive:outsider", "hive:outsider")
	CallContract(t, ct, "project_whitelist_add", PayloadString(dupPayload), nil, "hive:someone", true, uint(1_000_000_000))

	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))
	CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))
}

// TestWhitelistRemoveMemberNoEffect confirms removing a member address does nothing harmful.
func TestWhitelistRemoveMemberNoEffect(t *testing.T) {
	ct := SetupContractTest()
	projectID := createWhitelistProject(t, ct)
	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))

	addPayload := fmt.Sprintf("%d|%s", projectID, "hive:outsider")
	CallContract(t, ct, "project_whitelist_add", PayloadString(addPayload), nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))

	removePayload := fmt.Sprintf("%d|%s", projectID, "hive:outsider")
	CallContract(t, ct, "project_whitelist_remove", PayloadString(removePayload), nil, "hive:someone", true, uint(1_000_000_000))

	res, _, _ := CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:outsider", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "already a member") {
		t.Fatalf("expected member removal attempt to have no effect, got %q", res.Ret)
	}
}

// TestWhitelistRemoveUnknownNoop ensures removing a non-existent entry leaves approvals unchanged.
func TestWhitelistRemoveUnknownNoop(t *testing.T) {
	ct := SetupContractTest()
	projectID := createWhitelistProject(t, ct)

	removePayload := fmt.Sprintf("%d|%s", projectID, "hive:outsider")
	CallContract(t, ct, "project_whitelist_remove", PayloadString(removePayload), nil, "hive:someone", true, uint(1_000_000_000))

	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:outsider", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "whitelist approval required") {
		t.Fatalf("expected removal noop to keep whitelist enforcing, got %q", res.Ret)
	}
}

// TestWhitelistOwnerEmptyPayload checks owner calls must include addresses.
func TestWhitelistOwnerEmptyPayload(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	badPayload := fmt.Sprintf("%d|", projectID)

	res, _, _ := CallContract(t, ct, "project_whitelist_add", PayloadString(badPayload), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "whitelist payload requires addresses") {
		t.Fatalf("expected add payload validation, got %q", res.Ret)
	}

	res, _, _ = CallContract(t, ct, "project_whitelist_remove", PayloadString(badPayload), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "whitelist payload requires addresses") {
		t.Fatalf("expected remove payload validation, got %q", res.Ret)
	}
}

// TestWhitelistProposalRemoveMissingAddress ensures proposals removing unknown addresses are harmless.
func TestWhitelistProposalRemoveMissingAddress(t *testing.T) {
	ct := SetupContractTest()
	projectID := createWhitelistProject(t, ct)

	propID := createPollProposal(t, ct, projectID, "1", "", "whitelist_remove=hive:outsider")
	voteForProposal(t, ct, propID, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T01:00:00")

	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:outsider", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "whitelist approval required") {
		t.Fatalf("expected removal of missing entry to keep whitelist blocking, got %q", res.Ret)
	}
}

// TestWhitelistProposalUnknownKeyIgnored verifies unrelated metadata keys are ignored.
func TestWhitelistProposalUnknownKeyIgnored(t *testing.T) {
	ct := SetupContractTest()
	projectID := createWhitelistProject(t, ct)

	meta := "custom_flag=foo;whitelist_add=hive:outsider"
	propID := createPollProposal(t, ct, projectID, "1", "", meta)
	voteForProposal(t, ct, propID, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T01:00:00")

	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))
	CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))
}

// TestWhitelistOwnerOnlyAccess ensures only the project owner can mutate the whitelist directly.
func TestWhitelistOwnerOnlyAccess(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := fmt.Sprintf("%d|%s", projectID, "hive:outsider")

	res, _, _ := CallContract(t, ct, "project_whitelist_add", PayloadString(payload), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only owner can update whitelist") {
		t.Fatalf("expected add to be restricted to owner, got %q", res.Ret)
	}

	res, _, _ = CallContract(t, ct, "project_whitelist_remove", PayloadString(payload), nil, "hive:member2", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only owner can update whitelist") {
		t.Fatalf("expected remove to be restricted to owner, got %q", res.Ret)
	}
}

// TestWhitelistProposalInvalidMetadata verifies proposals must supply valid whitelist metadata.
func TestWhitelistProposalInvalidMetadata(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[len(fields)-1] = "1"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	propID := createPollProposal(t, ct, projectID, "1", "", "whitelist_add=")
	voteForProposal(t, ct, propID, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ = CallContractAt(t, ct, "proposal_execute", PayloadUint64(propID), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T01:00:00")
	if !strings.Contains(res.Ret, "whitelist_add metadata requires addresses") {
		t.Fatalf("expected metadata validation failure, got %q", res.Ret)
	}
}

// TestWhitelistToggleInvalidFlag ensures proposals reject unknown whitelist toggle values.
func TestWhitelistToggleInvalidFlag(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	propID := createPollProposal(t, ct, projectID, "1", "", "update_whitelistOnly=maybe")
	voteForProposal(t, ct, propID, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadUint64(propID), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T01:00:00")
	if !strings.Contains(res.Ret, "invalid whitelist flag") {
		t.Fatalf("expected invalid flag rejection, got %q", res.Ret)
	}
}

// TestWhitelistAndNFTEnforced ensures both checks run when whitelist enforcement is enabled.
func TestWhitelistAndNFTEnforced(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[10] = "contract:mocknft"
	fields[11] = "owns"
	fields[12] = "1"
	fields[len(fields)-1] = "1"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))
	failRes, _, _ := CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:someoneelse", false, uint(1_000_000_000))
	if !containsNFTGateMessage(failRes.Ret) {
		t.Fatalf("expected nft gating on initial join, got %q", failRes.Ret)
	}

	whitelistPayload := fmt.Sprintf("%d|%s", projectID, "hive:someoneelse")
	CallContract(t, ct, "project_whitelist_add", PayloadString(whitelistPayload), nil, "hive:someone", true, uint(1_000_000_000))
	nftRes, _, _ := CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:someoneelse", false, uint(1_000_000_000))
	if !containsNFTGateMessage(nftRes.Ret) {
		t.Fatalf("expected nft rejection after whitelist, got %q", nftRes.Ret)
	}
}

// TestWhitelistOnlyProjectRequiresApproval ensures projects can force manual approvals without NFTs.
func TestWhitelistOnlyProjectRequiresApproval(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[len(fields)-1] = "1" // whitelist only flag
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))
	failRes, _, _ := CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(failRes.Ret, "whitelist") {
		t.Fatalf("expected whitelist enforced, got %q", failRes.Ret)
	}

	whitelistPayload := fmt.Sprintf("%d|%s", projectID, "hive:someoneelse")
	CallContract(t, ct, "project_whitelist_add", PayloadString(whitelistPayload), nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
}

// TestWhitelistOnlyToggleViaProposal verifies DAO proposals can toggle whitelist enforcement.
func TestWhitelistOnlyToggleViaProposal(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	enableProposal := createPollProposal(t, ct, projectID, "1", "", "update_whitelistOnly=true")
	voteForProposal(t, ct, enableProposal, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(enableProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(enableProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T01:00:00")

	projectPayload := PayloadString(strconv.FormatUint(projectID, 10))
	failRes, _, _ := CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:member2", false, uint(1_000_000_000))
	if !strings.Contains(failRes.Ret, "whitelist") {
		t.Fatalf("expected whitelist gating after toggle, got %q", failRes.Ret)
	}

	CallContract(t, ct, "project_whitelist_add", PayloadString(fmt.Sprintf("%d|%s", projectID, "hive:member2")), nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:member2", true, uint(1_000_000_000))

	disableProposal := createPollProposal(t, ct, projectID, "1", "", "update_whitelistOnly=false")
	voteForProposal(t, ct, disableProposal, "hive:someone", "hive:member2")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(disableProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(disableProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T01:00:00")

	CallContract(t, ct, "project_join", projectPayload, transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))
}

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

// TestProposalEarlyTallyFails checks the proposal early tally fails flow so we dont break it again.
func TestProposalEarlyTallyFails(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "2")
	res, _, _ := CallContract(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "still running") {
		t.Fatalf("expected tally to fail with running proposal, got %q", res.Ret)
	}
}

// TestVoteAllowedAfterDurationBeforeTally ensures votes can be cast after the duration until tally occurs.
func TestVoteAllowedAfterDurationBeforeTally(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createSimpleProposal(t, ct, projectID, "1")
	payload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContractAt(t, ct, "proposals_vote", payload, nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProjectLeaveFlow checks the project leave flow so we dont break it again.
func TestProjectLeaveFlow(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	idStr := strconv.FormatUint(projectID, 10)
	CallContract(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", true, uint(1_000_000_000))
	CallContractAt(t, ct, "project_leave", PayloadString(idStr), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProjectLeaveCooldown checks the project leave cooldown flow so we dont break it again.
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

// TestVoteRejectedForLateJoiner checks the vote rejected for late joiner flow so we dont break it again.
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

// TestProjectLeaveBlockedDuringPayoutProposal checks the project leave blocked during payout proposal flow so we dont break it again.
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

// TestProposalMetaUpdateLeaveCooldown checks the proposal meta update leave cooldown flow so we dont break it again.
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

// TestProposalMetaUpdateThreshold checks the proposal meta update threshold flow so we dont break it again.
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

// TestProposalMetaTogglePause checks the proposal meta toggle pause flow so we dont break it again.
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

// TestProposalExecuteTransfersFunds2Members checks the payout split across two members so we dont break it again.
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

// TestProposalExecuteTransfersFunds3Members checks the payout split across three members so we dont break it again.
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

// TestProposalExecuteRequiresPassed checks the proposal execute requires passed flow so we dont break it again.
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

// TestProposalExecuteInsufficientFunds checks the proposal execute insufficient funds flow so we dont break it again.
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

// TestProposalExecuteBlockedWhenPaused checks the proposal execute blocked when paused flow so we dont break it again.
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

// TestProposalCancelFlow checks the proposal cancel flow so we dont break it again.
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

// TestProposalCancelRequiresCreatorOrOwner checks the proposal cancel requires creator or owner flow so we dont break it again.
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

// TestProposalCancelOwnerRefundsCreator checks the proposal cancel owner refunds creator flow so we dont break it again.
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

// TestProposalExecuteRespectsDelay checks the proposal execute respects delay flow so we dont break it again.
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

// TestProposalMetaUpdateOwner checks the proposal meta update owner flow so we dont break it again.
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

// TestFullCycle simulates a simple cycle from project to executed proposal.
func TestFullCycle(t *testing.T) {
	ct := SetupContractTest()
	CallContract(t, ct, "project_create", PayloadString("test|desc|0|50.001|50.001|1|0|10|1|1|||||1|"), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "project_join", PayloadString("0"), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
	addTreasuryFunds(t, ct, 0, "3.000")
	CallContract(t, ct, "proposal_create", PayloadString("0|prpsl|desc|1||0|hive:tibfox:3||"), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", PayloadString("0|1"), nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", PayloadString("0|1"), nil, "hive:someoneelse", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(0), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString("0"), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000), "2025-09-05T01:00:00")
}

////////////////////////////////////////////////////////////////////////////////
// Pending scenarios (rename to Test* once ready to execute)
////////////////////////////////////////////////////////////////////////////////

// PendingDemocraticJoinExactStake documents the expectation that democratic joins require exact stake.
func TestDemocraticJoinExactStake(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "0")
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", payload, transferIntent("0.750"), "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "democratic projects require exactly") {
		t.Fatalf("expected exact stake requirement, got %q", res.Ret)
	}
	CallContract(t, ct, "project_join", payload, transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
}

// PendingStakeJoinMinimumEnforced ensures stake systems reject underfunded joins.
func TestStakeJoinMinimumEnforced(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	payload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ := CallContract(t, ct, "project_join", payload, transferIntent("0.500"), "hive:member2", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "stake too low") {
		t.Fatalf("expected stake minimum enforcement, got %q", res.Ret)
	}
	CallContract(t, ct, "project_join", payload, transferIntent("1.000"), "hive:member2", true, uint(1_000_000_000))
}

// PendingProposalDurationValidation covers the "duration shorter than config" branch.
func TestProposalDurationValidation(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[5] = "5"
	projectPayload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(projectPayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")
	proposalFields := simpleProposalFields(projectID, "1")
	proposalPayload := PayloadString(strings.Join(proposalFields, "|"))
	res, _, _ = CallContract(t, ct, "proposal_create", proposalPayload, transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "Duration must be higher or equal") {
		t.Fatalf("expected minimum duration check, got %q", res.Ret)
	}
}

// PendingTogglePauseProposalWhilePaused verifies toggle pause proposals work while the project is paused.
func TestTogglePauseProposalWhilePaused(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someone", true, uint(1_000_000_000))
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"resume",
		"toggle pause",
		"1",
		"",
		"0",
		"",
		"toggle_pause=0",
		"",
	}
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(strings.Join(fields, "|")), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, res.Ret, "proposal")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// PendingProjectFundsBlockedWhilePaused ensures pause blocks treasury operations.
func TestProjectFundsAllowedWhilePaused(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", projectID)), nil, "hive:someone", true, uint(1_000_000_000))
	payload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.250"), "hive:someone", true, uint(1_000_000_000))
}

// PendingTransferRequiresMemberTarget asserts transfer fails when new owner is not a member.
func TestTransferRequiresMemberTarget(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := PayloadString(fmt.Sprintf("%d|%s", projectID, "hive:outsider"))
	res, _, _ := CallContract(t, ct, "project_transfer", payload, nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "new owner must be a member") {
		t.Fatalf("expected member-only transfer target, got %q", res.Ret)
	}
}

// PendingQuorumThresholdFailure demonstrates tally failure when quorum/threshold aren't met.
func TestQuorumThresholdFailure(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	proposalID := createPollProposal(t, ct, projectID, "1", "", "")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContract(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "proposal is failed") {
		t.Fatalf("expected execution rejection due to failed proposal, got %q", res.Ret)
	}
}

// PendingExecutionDelayMetaUpdate confirms update_executionDelay meta takes effect.
func TestExecutionDelayMetaUpdate(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	delayProposal := createPollProposal(t, ct, projectID, "1", "", "update_executionDelay=12")
	voteForProposal(t, ct, delayProposal, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(delayProposal), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T02:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", delayProposal)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T02:00:00")
	target := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:0.100", "")
	voteForProposal(t, ct, target, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(target), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T01:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", target)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-03T05:00:00")
	if !strings.Contains(res.Ret, "execution delay") {
		t.Fatalf("expected extended execution delay, got %q", res.Ret)
	}
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", target)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T20:00:00")
}

// PendingOwnerCancelWithoutTreasuryRefund shows owner cancellations skip refunds when treasury is empty.
func TestOwnerCancelWithoutTreasuryRefund(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	// Drain treasury via payout.
	addTreasuryFunds(t, ct, projectID, "1.000")
	spend := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:1.000", "")
	voteForProposal(t, ct, spend, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(spend), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T02:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", spend)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T02:00:00")
	// Create another proposal, then drain its cost deposit before cancelling.
	target := createPollProposal(t, ct, projectID, "1", "", "")
	drain := createPollProposal(t, ct, projectID, "1", "hive:someone:1.000", "")
	voteForProposal(t, ct, drain, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(drain), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T04:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", drain)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-03T04:00:00")
	pre := ct.GetBalance("hive:someone", ledgerDb.AssetHive)
	CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", target)), nil, "hive:someone", true, uint(1_000_000_000))
	post := ct.GetBalance("hive:someone", ledgerDb.AssetHive)
	if post != pre {
		t.Fatalf("expected no refund when treasury empty")
	}
}

// PendingPayoutLockReleasedAfterCancel ensures payout locks unblock leaves when the proposal is cancelled.
func TestPayoutLockReleasedAfterCancel(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	addTreasuryFunds(t, ct, projectID, "0.500")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:0.500", "")
	res, _, _ := CallContract(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "active proposal requesting funds") {
		t.Fatalf("expected payout lock to block leave, got %q", res.Ret)
	}
	CallContract(t, ct, "proposal_cancel", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000))
}

// PendingVoteInvalidOptionIndex covers invalid vote payloads for multi-option proposals.
func TestVoteInvalidOptionIndex(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	fields := simpleProposalFields(projectID, "1")
	fields[4] = "yes;no;maybe"
	proposalPayload := PayloadString(strings.Join(fields, "|"))
	res, _, _ := CallContract(t, ct, "proposal_create", proposalPayload, transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, res.Ret, "proposal")
	badVote := PayloadString(fmt.Sprintf("%d|5", proposalID))
	res2, _, _ := CallContract(t, ct, "proposals_vote", badVote, nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res2.Ret, "invalid option index") {
		t.Fatalf("expected invalid option rejection, got %q", res2.Ret)
	}
}

// PendingMembershipNFTFlow outlines the expected NFT validation scenarios (requires future harness support).
func PendingMembershipNFTFlow(t *testing.T) {
	t.Skip("NFT contract interactions are not yet available in this harness; rename once environment supports it.")
}

// parseCreatedID reads the `msg:<id>` responses so the tests can reuse the same helper everywhere.
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

// transferIntent crafts a simple hive transfer.allow intent used by most tests.
func transferIntent(limit string) []contracts.Intent {
	return transferIntentWithToken(limit, "hive")
}

// transferIntentWithToken allows tests to swap the token for negative scenarios.
func transferIntentWithToken(limit string, token string) []contracts.Intent {
	return []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": limit, "token": token}}}
}

// containsNFTGateMessage returns true if the error indicates NFT gating blocked access.
func containsNFTGateMessage(msg string) bool {
	return strings.Contains(msg, "membership nft not owned") || strings.Contains(msg, "contract contract:mocknft does not exist")
}

// joinProjectMember wraps the repeated join call to keep tests terse.
func joinProjectMember(t *testing.T, ct *test_utils.ContractTest, projectID uint64, user string) {
	CallContract(t, ct, "project_join", PayloadString(strconv.FormatUint(projectID, 10)), transferIntent("1.000"), user, true, uint(1_000_000_000))
}

// voteForProposal reuses the same payload to submit yes votes from multiple members.
func voteForProposal(t *testing.T, ct *test_utils.ContractTest, proposalID uint64, voters ...string) {
	payload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	for _, voter := range voters {
		CallContract(t, ct, "proposals_vote", payload, nil, voter, true, uint(1_000_000_000))
	}
}

// createDefaultProject uses the default field template and returns the new project id.
func createDefaultProject(t *testing.T, ct *test_utils.ContractTest) uint64 {
	payload := strings.Join(defaultProjectFields(), "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

// createProjectWithVoting tweaks the default fields with a custom voting mode before deploying.
func createProjectWithVoting(t *testing.T, ct *test_utils.ContractTest, voting string) uint64 {
	fields := defaultProjectFields()
	fields[2] = voting
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

// createWhitelistProject builds a project with whitelist enforcement enabled.
func createWhitelistProject(t *testing.T, ct *test_utils.ContractTest) uint64 {
	fields := defaultProjectFields()
	fields[len(fields)-1] = "1"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

// createNftGatedProject configures membership contract requirements for whitelist tests.
func createNftGatedProject(t *testing.T, ct *test_utils.ContractTest) uint64 {
	fields := defaultProjectFields()
	fields[10] = "contract:mocknft"
	fields[11] = "owns"
	fields[12] = "1"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

// createSimpleProposal assembles a minimal non-payout proposal for helper cases.
func createSimpleProposal(t *testing.T, ct *test_utils.ContractTest, projectID uint64, duration string) uint64 {
	payload := strings.Join(simpleProposalFields(projectID, duration), "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "proposal")
}

// simpleProposalFields returns the base pipe-delimited fields used by helper builders.
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

// createPollProposal spawns a poll-style proposal optionally including payouts/meta updates.
func createPollProposal(t *testing.T, ct *test_utils.ContractTest, projectID uint64, duration string, payouts string, meta string) uint64 {
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"payout",
		"treasury distribution",
		duration,
		"",
		"0",
		payouts,
		meta,
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "proposal")
}

// addTreasuryFunds injects some hive into a project so payout tests can execute.
func addTreasuryFunds(t *testing.T, ct *test_utils.ContractTest, projectID uint64, amount string) {
	payload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent(amount), "hive:someone", true, uint(1_000_000_000))
}

// defaultProjectFields returns the canonical test fixture for quick DAO deployments.
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
		"",
		"",
	}
}
