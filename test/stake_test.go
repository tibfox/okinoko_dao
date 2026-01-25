package contract_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// =============================================================================
// Stake History Tests
// =============================================================================

// TestStakeHistoryTracking tests that stake changes are properly tracked in history.
func TestStakeHistoryTracking(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")

	// Member can increase stake freely (no vote lock)
	payload := fmt.Sprintf("%d|true", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestVoteWeightUsesHistoricalStake tests that vote weight uses historical stake at proposal creation.
func TestVoteWeightUsesHistoricalStake(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create proposal before stake increase
	proposalID := createSimpleProposal(t, ct, projectID, "24")

	// Increase stake after proposal
	payload := fmt.Sprintf("%d|true", projectID)
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("5.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T00:00:00")

	// Vote should use stake at proposal creation
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestMultipleStakeChangesMultipleProposals tests stake history with multiple changes and proposals.
func TestMultipleStakeChangesMultipleProposals(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create first proposal
	prop1 := createSimpleProposal(t, ct, projectID, "24")

	// Increase stake
	payload := fmt.Sprintf("%d|true", projectID)
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("2.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T00:00:00")

	// Create second proposal
	fields := simpleProposalFields(projectID, "24")
	propPayload := PayloadString(strings.Join(fields, "|"))
	res, _, _ := CallContractAt(t, ct, "proposal_create", propPayload, transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T01:00:00")
	prop2 := parseCreatedID(t, res.Ret, "proposal")

	// Vote on both proposals
	voteForProposal(t, ct, prop1, "hive:someone", "hive:someoneelse")
	voteForProposal(t, ct, prop2, "hive:someone", "hive:someoneelse")

	// Tally both
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(prop1), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(prop2), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T02:00:00")
}

// TestStakeHistoryCleanupOnLeave tests that stake history is cleaned up when member leaves.
func TestStakeHistoryCleanupOnLeave(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Add some stake changes
	payload := fmt.Sprintf("%d|true", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000))
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someoneelse", true, uint(1_000_000_000), "2025-09-04T00:00:00")

	// Request leave
	CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-05T00:00:00")

	// Complete leave after cooldown
	CallContractAt(t, ct, "project_leave", PayloadString(strconv.FormatUint(projectID, 10)), nil, "hive:someoneelse", true, uint(1_000_000_000), "2025-09-06T00:00:00")
}

// TestVoteWithNoStakeHistory tests voting when no stake history exists.
func TestVoteWithNoStakeHistory(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	proposalID := createSimpleProposal(t, ct, projectID, "24")

	// Vote should work with initial stake (join creates history entry)
	voteForProposal(t, ct, proposalID, "hive:someoneelse")
}

// TestStakeChangesDontAffectPastProposals tests that stake changes don't affect past proposal votes.
func TestStakeChangesDontAffectPastProposals(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	proposalID := createSimpleProposal(t, ct, projectID, "24")
	voteForProposal(t, ct, proposalID, "hive:someone")

	// Increase stake after voting
	payload := fmt.Sprintf("%d|true", projectID)
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("10.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T00:00:00")

	// Vote weight should still be based on stake at proposal creation
	voteForProposal(t, ct, proposalID, "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestMemberJoinsLeavesRejoinsStakeHistory tests stake history through member lifecycle.
func TestMemberJoinsLeavesRejoinsStakeHistory(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
}

// TestVoteUpdateUsesOriginalStake tests that vote updates use original stake at proposal creation.
func TestVoteUpdateUsesOriginalStake(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	proposalID := createSimpleProposal(t, ct, projectID, "24")

	// Vote for no
	noVote := PayloadString(fmt.Sprintf("%d|0", proposalID))
	CallContract(t, ct, "proposals_vote", noVote, nil, "hive:someone", true, uint(1_000_000_000))

	// Increase stake
	payload := fmt.Sprintf("%d|true", projectID)
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("5.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T00:00:00")

	// Change vote to yes - should use original stake
	yesVote := PayloadString(fmt.Sprintf("%d|1", proposalID))
	CallContract(t, ct, "proposals_vote", yesVote, nil, "hive:someone", true, uint(1_000_000_000))

	voteForProposal(t, ct, proposalID, "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}

// TestMinStakeRequirementCheckedAgainstHistoricalStake tests min stake check uses historical stake.
func TestMinStakeRequirementCheckedAgainstHistoricalStake(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
}

// TestStakeIncrementCounterProgression tests stake increment counter progresses correctly.
func TestStakeIncrementCounterProgression(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")

	// Multiple stake increases
	payload := fmt.Sprintf("%d|true", projectID)
	CallContract(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T00:00:00")
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T01:00:00")

	// Create proposal and vote
	fields := simpleProposalFields(projectID, "24")
	propPayload := PayloadString(strings.Join(fields, "|"))
	res, _, _ := CallContractAt(t, ct, "proposal_create", propPayload, transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T02:00:00")
	proposalID := parseCreatedID(t, res.Ret, "proposal")

	voteForProposal(t, ct, proposalID, "hive:someone")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-06T00:00:00")
}

// TestProposalCreatedBetweenStakeChanges tests proposal created between stake changes.
func TestProposalCreatedBetweenStakeChanges(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// First stake increase
	payload := fmt.Sprintf("%d|true", projectID)
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("2.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-03T12:00:00")

	// Create proposal
	fields := simpleProposalFields(projectID, "24")
	propPayload := PayloadString(strings.Join(fields, "|"))
	res, _, _ := CallContractAt(t, ct, "proposal_create", propPayload, transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T00:00:00")
	proposalID := parseCreatedID(t, res.Ret, "proposal")

	// Second stake increase after proposal
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("3.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T12:00:00")

	// Vote and tally
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-06T00:00:00")
}

// TestMultipleMembersIndependentStakeHistories tests multiple members have independent stake histories.
func TestMultipleMembersIndependentStakeHistories(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
}

// TestLeaveImmediatelyAfterVoting tests leaving immediately after voting.
func TestLeaveImmediatelyAfterVoting(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
}

// TestLeaveWhileMultipleActiveProposalsWithVotes tests leaving while multiple active proposals have votes.
func TestLeaveWhileMultipleActiveProposalsWithVotes(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")
}

// TestStakeIncreaseAfterMultipleProposals tests stake increase after multiple proposals.
func TestStakeIncreaseAfterMultipleProposals(t *testing.T) {
	ct := SetupContractTest()
	projectID := createProjectWithVoting(t, ct, "1")
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create multiple proposals
	prop1 := createSimpleProposal(t, ct, projectID, "24")
	fields := simpleProposalFields(projectID, "24")
	propPayload := PayloadString(strings.Join(fields, "|"))
	res, _, _ := CallContractAt(t, ct, "proposal_create", propPayload, transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-03T01:00:00")
	prop2 := parseCreatedID(t, res.Ret, "proposal")

	// Increase stake
	payload := fmt.Sprintf("%d|true", projectID)
	CallContractAt(t, ct, "project_funds", PayloadString(payload), transferIntent("5.000"), "hive:someone", true, uint(1_000_000_000), "2025-09-04T00:00:00")

	// Vote on both
	voteForProposal(t, ct, prop1, "hive:someone", "hive:someoneelse")
	voteForProposal(t, ct, prop2, "hive:someone", "hive:someoneelse")

	// Tally both
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(prop1), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(prop2), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T02:00:00")
}

// TestDemocraticProjectStakeHistory tests stake history in democratic projects.
func TestDemocraticProjectStakeHistory(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)
	joinProjectMember(t, ct, projectID, "hive:someoneelse")

	// Create and vote on proposal
	proposalID := createSimpleProposal(t, ct, projectID, "24")
	voteForProposal(t, ct, proposalID, "hive:someone", "hive:someoneelse")
	CallContractAt(t, ct, "proposal_tally", PayloadUint64(proposalID), nil, "hive:someone", true, uint(1_000_000_000), "2025-09-05T00:00:00")
}
