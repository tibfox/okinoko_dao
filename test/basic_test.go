package contract_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"vsc-node/lib/test_utils"
	"vsc-node/modules/db/vsc/contracts"
)

func TestCreateProject(t *testing.T) {
	ct := SetupContractTest()

	fields := []string{
		"dao",
		"desc",
		"0",
		"50.001",
		"50.001",
		"10",
		"10",
		"10",
		"1",
		"1",
		"",
		"",
		"",
		"",
	}
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

func TestProposalLifecycle(t *testing.T) {
	ct := SetupContractTest()

	projectFields := []string{
		"dao",
		"desc",
		"0",
		"50.001",
		"50.001",
		"1",
		"10",
		"10",
		"1",
		"1",
		"",
		"",
		"",
		"",
	}
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

func TestVoteDoubleSubmissionFails(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	proposalID := createSimpleProposal(t, ct, projectID, "2")
	payload := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", payload, nil, "hive:someone", true, uint(1_000_000_000))
	res, _, _ := CallContract(t, ct, "proposals_vote", payload, nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "already voted") {
		t.Fatalf("expected duplicate vote rejection, got %q", res.Ret)
	}
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
	return []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": limit, "token": "hive"}}}
}

func joinProjectMember(t *testing.T, ct *test_utils.ContractTest, projectID uint64, user string) {
	CallContract(t, ct, "project_join", PayloadUint64(projectID), transferIntent("1.000"), user, true, uint(1_000_000_000))
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

func defaultProjectFields() []string {
	return []string{
		"dao",
		"desc",
		"0",
		"50.001",
		"50.001",
		"1",
		"10",
		"10",
		"1",
		"1",
		"",
		"",
		"",
		"",
	}
}
