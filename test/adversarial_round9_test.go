package contract_test

// Round 9 (review pass 1) — NaN/Inf config validation. NaN comparisons are all
// false, so a NaN threshold/quorum/cost previously slipped past every range check:
// NaN threshold bricks governance (never passes), NaN quorum bypasses it
// (uint64(NaN)==0 always met), NaN cost makes proposals free.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// newProjectFields builds project fields from explicit threshold/quorum/cost/stake.
func newProjectFields(threshold, quorum, cost, stakeMin string) []string {
	return []string{"dao", "desc", "0", threshold, quorum, "1", "0", "10", cost, stakeMin, "", "", "", "", "1", "", "", ""}
}

// R9-1..5: NaN/Inf in each float config field is rejected at project creation.
//
// NOTE: the literal strings "NaN"/"Inf" are now caught by the decimal-syntax filter
// in parseFloatField (they contain letters) BEFORE the math.IsNaN/IsInf guard runs.
// These tests therefore pin the rejection, not that specific guard — see
// TestBreak_OverflowLiteralRejected for one that actually reaches it.
func TestBreak_NaNThresholdRejected(t *testing.T) {
	ct := SetupContractTest()
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(newProjectFields("NaN", "50.001", "1", "1"))), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "invalid threshold", "NaN threshold (governance would be bricked)")
}
func TestBreak_NaNQuorumRejected(t *testing.T) {
	ct := SetupContractTest()
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(newProjectFields("50.001", "NaN", "1", "1"))), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "invalid quorum", "NaN quorum (quorum would be bypassed)")
}
func TestBreak_NaNCostRejected(t *testing.T) {
	ct := SetupContractTest()
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(newProjectFields("50.001", "50.001", "NaN", "1"))), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "invalid proposal cost", "NaN proposal cost (proposals would be free)")
}
func TestBreak_InfThresholdRejected(t *testing.T) {
	ct := SetupContractTest()
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(newProjectFields("Inf", "50.001", "1", "1"))), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "invalid threshold", "Inf threshold")
}
func TestBreak_InfCostRejected(t *testing.T) {
	ct := SetupContractTest()
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(newProjectFields("50.001", "50.001", "Inf", "1"))), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "invalid proposal cost", "Inf proposal cost")
}

// R9-6: NaN threshold via a passing governance proposal is rejected at execution.
func TestBreak_NaNThresholdViaMetaRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "1", "", "update_threshold=NaN")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, res, "threshold must be between 1% and 100%", "NaN threshold set via governance (would brick the DAO)")
}

// R9-7: NaN quorum via governance is rejected at execution.
func TestBreak_NaNQuorumViaMetaRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "1", "", "update_quorum=NaN")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, res, "quorum must be between 1% and 100%", "NaN quorum set via governance (quorum bypass)")
}

// R9-8: NaN cost via governance is rejected at execution.
func TestBreak_NaNCostViaMetaRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	propID := createPollProposal(t, ct, pid, "1", "", "update_proposalCost=NaN")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	res := rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e")
	assertAborts(t, res, "invalid proposal cost update", "NaN proposal cost set via governance (free proposals)")
}

// R9-9: a NaN vote-intent limit is rejected (already guarded in context; confirms it).
func TestBreak_NaNStakeMinRejected(t *testing.T) {
	ct := SetupContractTest()
	// stake project with NaN min stake must be rejected at creation.
	f := []string{"dao", "desc", "1", "50.001", "50.001", "1", "0", "10", "1", "NaN", "", "", "", "", "1", "", "", ""}
	res := rawCallAt(ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "invalid min stake", "NaN min stake accepted")
}

// R9-10: a NaN cost that WOULD have been set is provably absent — a valid DAO after
// the fix still charges/works normally (positive control).
func TestBreak_ValidConfigStillWorksAfterNaNGuard(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct) // threshold/quorum 50.001, cost 1
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// a normal proposal + vote + pass + execute still functions
	addTreasuryFunds(t, ct, pid, "2.000")
	propID := createPollProposal(t, ct, pid, "1", "hive:someoneelse:0.500:hive", "")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	assert.Equal(t, before+500, hiveBal(ct, "hive:someoneelse"))
}
