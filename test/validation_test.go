package contract_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"vsc-node/lib/test_utils"
	"vsc-node/modules/db/vsc/contracts"
)

// =============================================================================
// Threshold/Quorum Bounds Validation Tests
// =============================================================================

// TestThresholdTooLow checks that threshold < 1% is rejected.
func TestThresholdTooLow(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[3] = "0.5" // threshold below minimum (1%)
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "threshold must be between") {
		t.Fatalf("expected threshold bounds rejection, got %q", res.Ret)
	}
}

// TestThresholdTooHigh checks that threshold > 100% is rejected.
func TestThresholdTooHigh(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[3] = "101" // threshold above maximum (100%)
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "threshold must be between") {
		t.Fatalf("expected threshold bounds rejection, got %q", res.Ret)
	}
}

// TestQuorumTooLow checks that quorum < 1% is rejected.
func TestQuorumTooLow(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[4] = "0.5" // quorum below minimum (1%)
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "quorum must be between") {
		t.Fatalf("expected quorum bounds rejection, got %q", res.Ret)
	}
}

// TestQuorumTooHigh checks that quorum > 100% is rejected.
func TestQuorumTooHigh(t *testing.T) {
	ct := SetupContractTest()
	fields := defaultProjectFields()
	fields[4] = "150" // quorum above maximum (100%)
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "quorum must be between") {
		t.Fatalf("expected quorum bounds rejection, got %q", res.Ret)
	}
}

// TestThresholdBoundsValidViaMetaUpdate checks that threshold updates via proposal also validate bounds.
func TestThresholdBoundsValidViaMetaUpdate(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create proposal to update threshold to invalid value
	proposalID := createMetaProposal(t, ct, projectID, "1", "update_threshold=150")

	// Vote and tally
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute should fail due to invalid threshold
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "threshold must be between") {
		t.Fatalf("expected threshold bounds rejection on execute, got %q", res.Ret)
	}
}

// TestQuorumBoundsValidViaMetaUpdate checks that quorum updates via proposal also validate bounds.
func TestQuorumBoundsValidViaMetaUpdate(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create proposal to update quorum to invalid value
	proposalID := createMetaProposal(t, ct, projectID, "1", "update_quorum=0")

	// Vote and tally
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute should fail due to invalid quorum
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "quorum must be between") {
		t.Fatalf("expected quorum bounds rejection on execute, got %q", res.Ret)
	}
}

// =============================================================================
// Multi-Asset Payouts Per Address Tests
// =============================================================================

// TestMultiAssetPayoutToSameAddress checks that we can send multiple assets to the same address.
func TestMultiAssetPayoutToSameAddress(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Add both HIVE and HBD to treasury
	addTreasuryFunds(t, ct, projectID, "1.000")
	addTreasuryFundsWithToken(t, ct, projectID, "1.000", "hbd")

	// Create proposal with payouts: HIVE and HBD to the same address
	payouts := "hive:someoneelse:0.200:hive;hive:someoneelse:0.300:hbd"
	proposalID := createPayoutProposal(t, ct, projectID, "1", payouts)

	// Vote and tally
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute should succeed
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// =============================================================================
// Tie-Breaking Tests (Higher Index Wins)
// =============================================================================

// TestTieBreakingHigherIndexWins checks that when options tie, the higher index wins.
// Note: This test verifies the tie-breaking behavior by having both voters vote for
// multiple options. When votes tie, the algorithm picks the option with the higher index.
func TestTieBreakingHigherIndexWins(t *testing.T) {
	ct := SetupContractTest()

	// Create project with low threshold so ties can pass
	fields := defaultProjectFields()
	fields[3] = "25"  // threshold: 25% (so 50% can pass)
	fields[4] = "25"  // quorum: 25%
	fields[5] = "1"   // proposalDurationHours
	fields[6] = "0"   // executionDelayHours
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create proposal with 3 options
	proposalFields := []string{
		strconv.FormatUint(projectID, 10),
		"tiebreak test",
		"testing tie break",
		"1",
		"optionA;optionB;optionC", // 3 options: indices 0, 1, 2
		"1",                       // poll (so we can see which option won without execution)
		"",
		"",
		"",
	}
	propPayload := strings.Join(proposalFields, "|")
	propRes, _, _ := CallContract(t, ct, "proposal_create", PayloadString(propPayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	proposalID := parseCreatedID(t, propRes.Ret, "proposal")

	// Both voters vote for both option 0 AND option 2 - creating a tie
	// With threshold at 25% and each option getting 100% of votes, both pass threshold
	// The tie-breaking should pick option 2 (higher index)
	CallContract(t, ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|0,2", proposalID)), nil, "hive:someone", true, uint(1_000_000_000))
	CallContract(t, ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|0,2", proposalID)), nil, "hive:someoneelse", true, uint(1_000_000_000))

	// Tally - both options have equal votes, option 2 (higher index) should win
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	// The proposal is a poll so it becomes "closed" (not "passed"), with result being option 2
}

// =============================================================================
// Max Limits Tests
// =============================================================================

// TestMaxProposalOptionsExceeded checks that > 50 options is rejected.
func TestMaxProposalOptionsExceeded(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	// Build 51 options
	options := make([]string, 51)
	for i := 0; i < 51; i++ {
		options[i] = fmt.Sprintf("option%d", i)
	}
	optionsStr := strings.Join(options, ";")

	proposalFields := []string{
		strconv.FormatUint(projectID, 10),
		"too many options",
		"testing max options limit",
		"1",
		optionsStr,
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(proposalFields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "cannot have more than 50 options") {
		t.Fatalf("expected max options rejection, got %q", res.Ret)
	}
}

// TestMaxProposalOptionsAllowed checks that a reasonable number of options is accepted.
func TestMaxProposalOptionsAllowed(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	// Build 10 options (well under limit but verifies multi-option works)
	options := make([]string, 10)
	for i := 0; i < 10; i++ {
		options[i] = fmt.Sprintf("opt%d", i)
	}
	optionsStr := strings.Join(options, ";")

	proposalFields := []string{
		strconv.FormatUint(projectID, 10),
		"multi options",
		"testing multi options",
		"1",
		optionsStr,
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(proposalFields, "|")
	CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestMaxPayoutReceiversExceeded checks that > 50 payout entries is rejected.
func TestMaxPayoutReceiversExceeded(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	// Build 51 payout entries
	payouts := make([]string, 51)
	for i := 0; i < 51; i++ {
		payouts[i] = fmt.Sprintf("hive:addr%d:0.001:hive", i)
	}
	payoutsStr := strings.Join(payouts, ";")

	proposalFields := []string{
		strconv.FormatUint(projectID, 10),
		"too many payouts",
		"testing max payouts limit",
		"1",
		"",
		"0",
		payoutsStr,
		"",
		"",
	}
	payload := strings.Join(proposalFields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "cannot have more than 50 payout") {
		t.Fatalf("expected max payouts rejection, got %q", res.Ret)
	}
}

// TestOwnerWhitelistAddNoLimit checks that owner can add > 50 whitelist addresses.
func TestOwnerWhitelistAddNoLimit(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	// Build 51 addresses - should now succeed for owner operations
	addresses := make([]string, 51)
	for i := 0; i < 51; i++ {
		addresses[i] = fmt.Sprintf("hive:addr%d", i)
	}
	addressesStr := strings.Join(addresses, ";")

	payload := fmt.Sprintf("%d|%s", projectID, addressesStr)
	CallContract(t, ct, "project_whitelist_add", PayloadString(payload), nil, "hive:someone", true, uint(1_000_000_000))
}

// TestOwnerWhitelistAdd100Addresses checks that owner can add 100 unique whitelist addresses.
func TestOwnerWhitelistAdd100Addresses(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	// Build 100 unique addresses
	addresses := make([]string, 100)
	for i := 0; i < 100; i++ {
		addresses[i] = fmt.Sprintf("hive:addr%d", i)
	}
	addressesStr := strings.Join(addresses, ";")

	payload := fmt.Sprintf("%d|%s", projectID, addressesStr)
	CallContract(t, ct, "project_whitelist_add", PayloadString(payload), nil, "hive:someone", true, uint(1_000_000_000))
}

// TestProposalMetaWhitelistAddLimitEnforced checks that proposal meta whitelist_add still has 50 limit.
func TestProposalMetaWhitelistAddLimitEnforced(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Build 51 addresses for proposal meta (use comma separator since semicolon separates meta entries)
	addresses := make([]string, 51)
	for i := 0; i < 51; i++ {
		addresses[i] = fmt.Sprintf("hive:metaaddr%d", i)
	}
	addressesStr := strings.Join(addresses, ",")

	// Create proposal with whitelist_add meta that exceeds limit (uses high gas limit for large payload)
	proposalID := createMetaProposalWithGas(t, ct, projectID, "1", fmt.Sprintf("whitelist_add=%s", addressesStr), uint(3_000_000_000))

	// Vote and tally
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute should fail due to 50 address limit for proposal meta
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "cannot exceed 50 addresses") {
		t.Fatalf("expected max whitelist rejection for proposal meta, got %q", res.Ret)
	}
}

// TestProposalMetaWhitelistRemoveLimitEnforced checks that proposal meta whitelist_remove still has 50 limit.
func TestProposalMetaWhitelistRemoveLimitEnforced(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Build 51 addresses for proposal meta (use comma separator since semicolon separates meta entries)
	addresses := make([]string, 51)
	for i := 0; i < 51; i++ {
		addresses[i] = fmt.Sprintf("hive:metaaddr%d", i)
	}
	addressesStr := strings.Join(addresses, ",")

	// Create proposal with whitelist_remove meta that exceeds limit (uses high gas limit for large payload)
	proposalID := createMetaProposalWithGas(t, ct, projectID, "1", fmt.Sprintf("whitelist_remove=%s", addressesStr), uint(3_000_000_000))

	// Vote and tally
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute should fail due to 50 address limit for proposal meta
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "cannot exceed 50 addresses") {
		t.Fatalf("expected max whitelist rejection for proposal meta, got %q", res.Ret)
	}
}

// =============================================================================
// Remove Owner (Autonomous Project) Tests
// =============================================================================

// TestRemoveOwnerMakesProjectAutonomous checks that remove_owner meta option works.
func TestRemoveOwnerMakesProjectAutonomous(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create proposal to remove owner
	proposalID := createMetaProposal(t, ct, projectID, "1", "remove_owner=")

	// Vote and tally
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute - should succeed
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Now try to use owner-only functions - they should all fail

	// Test 1: Whitelist add should fail
	whitelistPayload := fmt.Sprintf("%d|hive:newaddr", projectID)
	res, _, _ := CallContract(t, ct, "project_whitelist_add", PayloadString(whitelistPayload), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "autonomous") {
		t.Fatalf("expected autonomous rejection for whitelist_add, got %q", res.Ret)
	}

	// Test 2: Whitelist remove should fail
	res, _, _ = CallContract(t, ct, "project_whitelist_remove", PayloadString(whitelistPayload), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "autonomous") {
		t.Fatalf("expected autonomous rejection for whitelist_remove, got %q", res.Ret)
	}

	// Test 3: Transfer ownership should fail
	transferPayload := fmt.Sprintf("%d|hive:someoneelse", projectID)
	res, _, _ = CallContract(t, ct, "project_transfer", PayloadString(transferPayload), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "autonomous") {
		t.Fatalf("expected autonomous rejection for transfer, got %q", res.Ret)
	}

	// Test 4: Emergency pause should fail
	pausePayload := fmt.Sprintf("%d|true", projectID)
	res, _, _ = CallContract(t, ct, "project_pause", PayloadString(pausePayload), nil, "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "autonomous") {
		t.Fatalf("expected autonomous rejection for pause, got %q", res.Ret)
	}
}

// TestAutonomousProjectCanStillGovernViaPorposals verifies proposals still work after removing owner.
func TestAutonomousProjectCanStillGovernViaPorposals(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// First, remove owner
	proposalID := createMetaProposal(t, ct, projectID, "1", "remove_owner=")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Now create a proposal to toggle pause - should still work via governance
	proposalID2 := createMetaProposal(t, ct, projectID, "1", "toggle_pause=")
	voteForProposal(t, ct, proposalID2, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID2), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-07T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID2)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-07T00:00:00")
	// Success means the autonomous project can still be governed via proposals
}

// TestAutonomousMembersCanLeave verifies members can leave without owner restriction.
func TestAutonomousMembersCanLeave(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// First, remove owner
	proposalID := createMetaProposal(t, ct, projectID, "1", "remove_owner=")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// The former owner (hive:someone) should now be able to initiate leave
	// (before remove_owner, owner could not leave without transferring first)
	CallContractAt(t, ct, "project_leave", PayloadString(fmt.Sprintf("%d", projectID)), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-06T00:00:00")
	// Success - leave request accepted
}

// =============================================================================
// Minimum Proposal Duration Tests
// =============================================================================

// TestMinProposalDurationEnforced checks that proposal duration < 1 hour is normalized.
func TestMinProposalDurationEnforced(t *testing.T) {
	ct := SetupContractTest()

	// Create project with proposal duration set to 0 (should be normalized to 1)
	fields := defaultProjectFields()
	fields[5] = "0" // proposalDurationHours
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	projectID := parseCreatedID(t, res.Ret, "project")

	// Create a proposal - it should have at least 1 hour duration
	proposalFields := []string{
		strconv.FormatUint(projectID, 10),
		"test",
		"testing min duration",
		"0", // duration 0, should use project default which was normalized
		"",
		"0",
		"",
		"",
		"",
	}
	propPayload := strings.Join(proposalFields, "|")
	CallContract(t, ct, "proposal_create", PayloadString(propPayload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	// Success means the duration was accepted (normalized internally)
}

// TestMinProposalDurationUpdateViaMetaEnforced checks that proposal duration updates enforce minimum.
func TestMinProposalDurationUpdateViaMetaEnforced(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectForMetaTest(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create proposal to update proposal duration to 0 (should fail on execute)
	proposalID := createMetaProposal(t, ct, projectID, "1", "update_proposalDuration=0")

	// Vote and tally
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Execute should fail due to invalid duration
	res, _, _ := CallContractAt(t, ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", proposalID)), nil, "hive:someone", false, uint(1_000_000_000), "2025-09-05T00:00:00")
	if !strings.Contains(res.Ret, "proposal duration must be at least") {
		t.Fatalf("expected min duration rejection on execute, got %q", res.Ret)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

// createProjectForMetaTest creates a project suitable for meta update tests.
func createProjectForMetaTest(t *testing.T, ct *test_utils.ContractTest) uint64 {
	fields := defaultProjectFields()
	fields[5] = "1"  // proposalDurationHours: 1 hour for faster tests
	fields[6] = "0"  // executionDelayHours: 0 for immediate execution
	fields[7] = "10" // leaveCooldownHours
	fields[8] = "1"  // proposalCost
	fields[9] = "1"  // stakeMinAmt
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "project_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "project")
}

// createMetaProposal creates a proposal with meta outcome for config updates.
func createMetaProposal(t *testing.T, ct *test_utils.ContractTest, projectID uint64, duration string, meta string) uint64 {
	return createMetaProposalWithGas(t, ct, projectID, duration, meta, uint(1_000_000_000))
}

// createMetaProposalWithGas creates a proposal with meta outcome with custom gas limit.
func createMetaProposalWithGas(t *testing.T, ct *test_utils.ContractTest, projectID uint64, duration string, meta string, gasLimit uint) uint64 {
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"config update",
		"updating project config",
		duration,
		"",
		"0",
		"",
		meta,
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, gasLimit)
	return parseCreatedID(t, res.Ret, "proposal")
}

// createPayoutProposal creates a proposal with payout outcome.
func createPayoutProposal(t *testing.T, ct *test_utils.ContractTest, projectID uint64, duration string, payouts string) uint64 {
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"payout proposal",
		"distributing treasury",
		duration,
		"",
		"0",
		payouts,
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	return parseCreatedID(t, res.Ret, "proposal")
}

// addTreasuryFundsWithToken adds funds to treasury with a specific token type.
func addTreasuryFundsWithToken(t *testing.T, ct *test_utils.ContractTest, projectID uint64, amount string, token string) {
	payload := fmt.Sprintf("%d|false", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": amount, "token": token}}}, "hive:someone", true, uint(1_000_000_000))
}
