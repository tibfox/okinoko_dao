package contract_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// =============================================================================
// Whitelist Tests
// =============================================================================

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

// TestWhitelistProposalUnknownKeyIgnored ensures unknown meta keys are silently skipped.
func TestWhitelistProposalUnknownKeyIgnored(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	propID := createPollProposal(t, ct, projectID, "1", "", "unknown_key=value")
	voteForProposal(t, ct, propID, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T01:00:00")
}

// TestWhitelistOwnerOnlyAccess ensures owner-only whitelist operations.
func TestWhitelistOwnerOnlyAccess(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	addPayload := fmt.Sprintf("%d|hive:outsider", projectID)
	res, _, _ := CallContract(t, ct, "project_whitelist_add", PayloadString(addPayload), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only owner") {
		t.Fatalf("expected owner-only rejection for add, got %q", res.Ret)
	}

	res, _, _ = CallContract(t, ct, "project_whitelist_remove", PayloadString(addPayload), nil, "hive:someoneelse", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "only owner") {
		t.Fatalf("expected owner-only rejection for remove, got %q", res.Ret)
	}
}

// TestWhitelistProposalInvalidMetadata ensures malformed metadata is rejected during execution.
func TestWhitelistProposalInvalidMetadata(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	propID := createPollProposal(t, ct, projectID, "1", "", "whitelist_add=")
	voteForProposal(t, ct, propID, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadUint64(propID), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T01:00:00")
	if !strings.Contains(res.Ret, "whitelist_add metadata requires addresses") {
		t.Fatalf("expected empty whitelist rejection, got %q", res.Ret)
	}
}

// TestWhitelistToggleInvalidFlag ensures invalid flag values are rejected.
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

// TestWhitelistAndNFTEnforced ensures whitelist + NFT enforcement combination works.
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

	// NFT gating is checked first - mock contract doesn't exist, so it fails there
	projPayload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ = CallContract(t, ct, "project_join", projPayload, transferIntent("1.000"), "hive:outsider", false, uint(1_000_000_000))
	if !containsNFTGateMessage(res.Ret) {
		t.Fatalf("expected nft check (contract doesn't exist), got %q", res.Ret)
	}
}

// TestWhitelistOnlyProjectRequiresApproval ensures whitelist-only projects require approval.
func TestWhitelistOnlyProjectRequiresApproval(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[len(fields)-1] = "1"
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	projPayload := PayloadString(strconv.FormatUint(projectID, 10))
	res, _, _ = CallContract(t, ct, "project_join", projPayload, transferIntent("1.000"), "hive:outsider", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "whitelist approval required") {
		t.Fatalf("expected whitelist requirement, got %q", res.Ret)
	}

	addPayload := fmt.Sprintf("%d|hive:outsider", projectID)
	CallContract(t, ct, "project_whitelist_add", PayloadString(addPayload), nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "project_join", projPayload, transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))
}

// TestWhitelistOnlyToggleViaProposal ensures whitelist toggle via proposal works.
func TestWhitelistOnlyToggleViaProposal(t *testing.T) {
	ct := SetupContractTest()
	projectID := createWhitelistProject(t, ct)
	// Whitelist members first since project has whitelist enforcement
	addPayload := fmt.Sprintf("%d|hive:someoneelse;hive:member2", projectID)
	CallContract(t, ct, "project_whitelist_add", PayloadString(addPayload), nil, "hive:someone", true, uint(1_000_000_000))
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	propID := createPollProposal(t, ct, projectID, "1", "", "update_whitelistOnly=0")
	votePayload := PayloadString(fmt.Sprintf("%d|1", propID))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", votePayload, nil, "hive:member2", true, uint(1_000_000_000))
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(propID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T01:00:00")

	projPayload := PayloadString(strconv.FormatUint(projectID, 10))
	CallContract(t, ct, "project_join", projPayload, transferIntent("1.000"), "hive:outsider", true, uint(1_000_000_000))
}
