package contract_test

// Round 10 (review pass 2) — URL/scheme validation, length bounds on free-form
// fields, pause authorization, and payload robustness.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// R10-1: an option URL that isn't https is rejected.
func TestBreak_OptionNonHttpsURLRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := []string{strconv.FormatUint(pid, 10), "poll", "d", "1", "yes###http://evil.example;no", "1", "", "", ""}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "option URL must start with https://", "a non-https option URL was accepted")
}

// R10-2: an https option URL is accepted and round-trips.
func TestBreak_OptionHttpsURLAccepted(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := []string{strconv.FormatUint(pid, 10), "poll", "d", "1", "yes###https://ok.example;no", "1", "", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "an https option URL was rejected")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "0", "v").Success)
}

// R10-3: an over-long proposal metadata blob is rejected (gas-griefing guard).
func TestBreak_ProposalMetadataLengthBounded(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	big := strings.Repeat("m", 513) // > MaxDescriptionLength
	fields := []string{strconv.FormatUint(pid, 10), "p", "d", "1", "", "0", "", "", big}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "proposal metadata exceeds maximum length of 512 characters", "an unbounded proposal metadata blob was accepted")
}

// R10-4: an over-long proposal URL is rejected.
func TestBreak_ProposalURLLengthBounded(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	big := "https://x.example/" + strings.Repeat("a", 500)
	fields := []string{strconv.FormatUint(pid, 10), "p", "d", "1", "", "0", "", "", "", big}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "proposal URL exceeds maximum length of 500 characters", "an unbounded proposal URL was accepted")
}

// R10-5: an over-long project metadata blob is rejected.
func TestBreak_ProjectMetadataLengthBounded(t *testing.T) {
	ct := SetupContractTest()
	f := []string{"dao", "desc", "0", "50.001", "50.001", "1", "0", "10", "1", "1", "", "", "", strings.Repeat("m", 513), "1", "", "", ""}
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "project metadata exceeds maximum length of 512 characters", "an unbounded project metadata blob was accepted")
}

// R10-6: over-long URL via update_url governance is rejected at execution.
func TestBreak_UpdateURLLengthBounded(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	big := "https://x/" + strings.Repeat("a", 600)
	pf := []string{strconv.FormatUint(pid, 10), "u", "d", "1", "", "0", "", "update_url=" + big, ""}
	propID, ok := createProposalRaw(ct, pf, "hive:someone", "c") // non-gas-asserting create
	assert.True(t, ok, "proposal creation failed")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, res, "url exceeds maximum length of 500 characters", "an unbounded url was set via governance")
}

// R10-7: only the owner can directly pause the project.
func TestBreak_NonOwnerCannotDirectPause(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	res := rawCallAt(ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someoneelse", defaultTimestamp, "p")
	assertAborts(t, res, "only owner can pause/unpause", "a non-owner directly paused the project")
}

// R10-8: an autonomous project cannot be directly paused (must use a proposal).
func TestBreak_AutonomousCannotDirectPause(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	rp := createPollProposal(t, ct, pid, "1", "", "remove_owner=1")
	assert.True(t, voteRaw(ct, rp, "hive:someone", "1", "r1").Success)
	assert.True(t, voteRaw(ct, rp, "hive:someoneelse", "1", "r2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(rp), nil, "hive:someone", lateTS, "t")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", rp)), nil, "hive:someone", lateTS, "e").Success)
	res := rawCallAt(ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someone", lateTS, "p")
	assertAborts(t, res, "project is autonomous - use proposal to pause/unpause", "an autonomous project was directly paused")
}

// R10-9: an empty/whitespace payload is rejected across entrypoints (no panic).
func TestBreak_EmptyPayloadsRejected(t *testing.T) {
	ct := SetupContractTest()
	_ = createDefaultProject(t, ct)
	for _, action := range []string{"project_join", "project_leave", "proposals_vote", "proposal_tally", "proposal_execute", "proposal_cancel", "project_pause", "project_transfer", "project_funds"} {
		res := rawCallAt(ct, action, PayloadString("   "), nil, "hive:someone", defaultTimestamp, "e-"+action)
		assertAbortsAny(t, res, "%s accepted an empty payload", action)
	}
}

// R10-10: a negative vote-choice index is rejected cleanly.
func TestBreak_NegativeVoteChoiceRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	propID := createSimpleProposal(t, ct, pid, "1")
	res := rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|-1", propID)), nil, "hive:someone", defaultTimestamp, "v")
	assertAborts(t, res, "invalid choice index", "a negative vote choice was accepted")
}

// R10-11: an option with empty text before ### is rejected.
func TestBreak_OptionEmptyTextBeforeDelimiterRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := []string{strconv.FormatUint(pid, 10), "poll", "d", "1", "###https://x.example;no", "1", "", "", ""}
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(fields)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "option text cannot be empty", "an option with empty text was accepted")
}
