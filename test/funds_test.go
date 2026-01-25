package contract_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"vsc-node/modules/db/vsc/contracts"
)

// =============================================================================
// Treasury Funds Tests
// =============================================================================

// TestAddFundsToTreasury checks the add funds to treasury flow so we dont break it again.
func TestAddFundsToTreasury(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestAddFundsZeroAmount checks adding zero funds.
func TestAddFundsZeroAmount(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := fmt.Sprintf("%d|false", projectID)
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("0.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "invalid amount") {
		t.Fatalf("expected zero amount rejection, got %q", res.Ret)
	}
}

// TestAddFundsMultipleAssets checks adding multiple asset types to treasury.
func TestAddFundsMultipleAssets(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	payload := fmt.Sprintf("%d|false", projectID)

	// Add HIVE
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))

	// Add HBD
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntentWithToken("1.000", "hbd"), "hive:someone", true, uint(1_000_000_000))
}

// TestAddFundsMultipleAssetsWithStake checks adding stake funds with multiple assets in stake project.
func TestAddFundsMultipleAssetsWithStake(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")

	// Add stake (HIVE)
	stakePayload := fmt.Sprintf("%d|true", projectID)
	CallContract(t, ct, "project_funds", PayloadString(stakePayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))

	// Add treasury funds (HIVE)
	treasuryPayload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(treasuryPayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))

	// Add treasury funds (HBD)
	CallContract(t, ct, "project_funds", PayloadString(treasuryPayload), transferIntentWithToken("1.000", "hbd"), "hive:someone", true, uint(1_000_000_000))
}

// TestAddFundsMultipleAssetsNoStakeAsset checks adding non-stake asset falls back to treasury in stake project.
func TestAddFundsMultipleAssetsNoStakeAsset(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")

	// Add HBD to treasury (project uses HIVE for stake)
	treasuryPayload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(treasuryPayload), transferIntentWithToken("1.000", "hbd"), "hive:someone", true, uint(1_000_000_000))

	// Try to add HBD as stake - should gracefully fall back to treasury (stake must match project asset)
	stakePayload := fmt.Sprintf("%d|true", projectID)
	res, _, _ := CallContract(t, ct, "project_funds", PayloadString(stakePayload), transferIntentWithToken("1.000", "hbd"), "hive:someone", true, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "funds added to treasury") {
		t.Fatalf("expected treasury fallback, got %q", res.Ret)
	}
}

// =============================================================================
// Payout Tests
// =============================================================================

// TestProposalExecuteTransfersFunds2Members checks the proposal execute transfers funds flow so we dont break it again.
func TestProposalExecuteTransfersFunds2Members(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	addTreasuryFunds(t, ct, projectID, "2.000")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:0.500:hive", "")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProposalExecuteTransfersFunds3Members checks the proposal execute transfers funds with 3 members flow so we dont break it again.
func TestProposalExecuteTransfersFunds3Members(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")
	addTreasuryFunds(t, ct, projectID, "2.000")
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:member2:0.500:hive", "")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse", "hive:member2")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestProposalExecuteInsufficientFunds checks the proposal execute insufficient funds flow so we dont break it again.
func TestProposalExecuteInsufficientFunds(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	// Request payout larger than total treasury balance (project creation + join + proposal costs = ~3 HIVE)
	proposalID := createPollProposal(t, ct, projectID, "1", "hive:someoneelse:10.000:hive", "")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "insufficient") {
		t.Fatalf("expected insufficient funds rejection, got %q", res.Ret)
	}
}

// TestDemocraticHBDPayoutExactAmount tests HBD payout with exact amount.
func TestDemocraticHBDPayoutExactAmount(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Add HBD to treasury
	treasuryPayload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(treasuryPayload), transferIntentWithToken("5.000", "hbd"), "hive:someone", true, uint(1_000_000_000))

	// Create proposal with HBD payout
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"hbd payout",
		"distribute hbd",
		"1",
		"",
		"0",
		"hive:someoneelse:2.500:hbd",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, res.Ret, "proposal")

	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestDemocraticPayoutThenMembersLeave tests payout followed by member leaving.
func TestDemocraticPayoutThenMembersLeave(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[5] = "1"
	fields[6] = "0"
	fields[7] = "1" // short cooldown
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	joinProjectMember(t, ct, projectID, "hive:someoneelse")
	joinProjectMember(t, ct, projectID, "hive:member2")

	// Add funds
	addTreasuryFunds(t, ct, projectID, "5.000")

	// Create payout proposal
	propFields := []string{
		strconv.FormatUint(projectID, 10),
		"payout",
		"distribute",
		"1",
		"",
		"0",
		"hive:someoneelse:1.000:hive",
		"",
		"",
	}
	propPayload := strings.Join(propFields, "|")
	res, _, _ = CallContract(t, ct, "proposal_create", PayloadString(propPayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, res.Ret, "proposal")

	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse", "hive:member2")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Now member can leave
	CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-06T00:00:00")
	CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-07T00:00:00")
}

// =============================================================================
// Multi-Asset Payout Tests
// =============================================================================

// TestMultiAssetPayoutSameReceiver tests multiple assets paid to the same receiver.
func TestMultiAssetPayoutSameReceiver(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Add both HIVE and HBD to treasury
	addTreasuryFunds(t, ct, projectID, "5.000")
	treasuryPayload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(treasuryPayload), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "5.000", "token": "hbd"}}}, "hive:someone", true, uint(1_000_000_000))

	// Create proposal with multiple assets to same receiver
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"multi asset payout",
		"pay both hive and hbd",
		"1",
		"",
		"0",
		"hive:someoneelse:1.000:hive;hive:someoneelse:2.000:hbd",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, res.Ret, "proposal")

	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}
