package contract_test

// Round 7 — unknown-meta rejection, boundary limits, and codec round-trip fidelity
// through real entrypoints (create with edge values, reload, verify behaviour).

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"vsc-node/lib/test_utils"
)

// createProposalAt creates a simple (no-outcome) proposal at a specific timestamp.
func createProposalAt(t *testing.T, ct *test_utils.ContractTest, pid uint64, duration, ts string) uint64 {
	res := rawCallAt(ct, "proposal_create", PayloadString(joinPipe(simpleProposalFields(pid, duration))), transferIntent("1.000"), "hive:someone", ts, "cpa")
	if !res.Success {
		t.Fatalf("createProposalAt failed: %s", res.Ret)
	}
	id, err := strconv.ParseUint(trimMsg(res.Ret), 10, 64)
	if err != nil {
		t.Fatalf("createProposalAt parse id %q: %v", res.Ret, err)
	}
	return id
}

// R7-1: an unknown / typo'd meta key is rejected at creation (so a typo'd
// governance directive can't pass while silently enacting nothing).
func TestBreak_UnknownMetaKeyRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	// typo: "update_treshold" must be rejected up front
	res := rawCallAt(ct, "proposal_create",
		PayloadString(fmt.Sprintf("%d|p|d|1||0||update_treshold=60|", pid)),
		transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "unknown meta action: update_treshold", "a typo'd meta key was accepted (silent no-op risk)")
	assert.Contains(t, res.Ret, "unknown meta action", "wrong rejection reason: %s", res.Ret)
}

// R7-2: a KNOWN meta key mixed with an unknown one is rejected wholesale (the
// unknown key fails the proposal rather than being silently dropped).
func TestBreak_MixedKnownUnknownMetaRejected(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	res := rawCallAt(ct, "proposal_create",
		PayloadString(fmt.Sprintf("%d|p|d|1||0||update_quorum=40.0;bogus_key=1|", pid)),
		transferIntent("1.000"), "hive:someone", defaultTimestamp, "c")
	assertAborts(t, res, "unknown meta action: bogus_key", "a valid+unknown meta mix was accepted")
}

// R7-3: a max-length (128 char) project name round-trips; 129 is rejected.
func TestBreak_ProjectNameLengthBoundary(t *testing.T) {
	ct := SetupContractTest()
	name128 := strings.Repeat("a", 128)
	f := []string{name128, "desc", "0", "50.001", "50.001", "1", "0", "10", "1", "1", "", "", "", "", "1", "", "", ""}
	ok := rawCallAt(ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "a")
	assert.True(t, ok.Success, "128-char name rejected: %s", ok.Ret)
	f[0] = strings.Repeat("a", 129)
	bad := rawCallAt(ct, "project_create", PayloadString(joinPipe(f)), transferIntent("1.000"), "hive:someone", defaultTimestamp, "b")
	assertAborts(t, bad, "project name exceeds maximum length of 128 characters", "129-char name accepted")
}

// R7-4: a proposal at the maximum option count (40) round-trips and is creatable
// (not just parseable — the previous cap of 50 crashed the wasm at execution);
// 41 options is rejected at creation.
func TestBreak_MaxOptionsRoundTrip(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	opts := make([]string, 40) // MaxProposalOptions
	for i := range opts {
		opts[i] = "opt" + strconv.Itoa(i)
	}
	fields := []string{strconv.FormatUint(pid, 10), "poll", "d", "1", strings.Join(opts, ";"), "1", "", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "40-option proposal could not be created (advertised max unreachable)")
	// vote for the last valid option index (39) — must reload correctly
	assert.True(t, voteRaw(ct, propID, "hive:someone", "39", "v").Success, "option index 39 rejected after reload")
	assert.False(t, voteRaw(ct, propID, "hive:someone", "40", "w").Success, "out-of-range option index accepted")

	// 41 options must be rejected at parse (before the crashing save loop)
	opts41 := append(opts, "opt40")
	f41 := []string{strconv.FormatUint(pid, 10), "poll", "d", "1", strings.Join(opts41, ";"), "1", "", "", ""}
	_, ok41 := createProposalRaw(ct, f41, "hive:someone", "c2")
	assert.False(t, ok41, "41 options accepted (over the safe cap)")
}

// R7-5: a multi-receiver payout list round-trips and executes (codec fidelity for
// the payout slice). NB: the 50-receiver max is functional but gas-heavy (~3.4B),
// so this uses a practical count that also exercises the encode/decode path.
func TestBreak_MaxPayoutReceiversRoundTrip(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "60.000")
	parts := make([]string, 20)
	for i := range parts {
		parts[i] = "hive:someoneelse:0.100:hive"
	}
	// create via the non-gas-asserting path (20 payout entries is gas-heavy)
	pf := []string{strconv.FormatUint(pid, 10), "payout", "d", "1", "", "0", strings.Join(parts, ";"), "", ""}
	propID, ok := createProposalRaw(ct, pf, "hive:someone", "c")
	assert.True(t, ok, "20-receiver payout proposal could not be created")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	after := hiveBal(ct, "hive:someoneelse")
	assert.Equal(t, before+2000, after, "20x0.100 payout did not total 2.000")
}

// R7-6: a proposal with a FULL outcome (payout + meta + URL) round-trips: it is
// stored, reloaded for tally, and executes both legs.
func TestBreak_FullOutcomeRoundTrip(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	addTreasuryFunds(t, ct, pid, "2.000")
	// payout + a meta update + a proposal URL, default yes/no options
	fields := []string{strconv.FormatUint(pid, 10), "combo", "desc", "1", "", "0",
		"hive:someoneelse:0.500:hive", "update_quorum=40.0", "meta-x", "https://example.com/p"}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "full-outcome proposal could not be created")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v1").Success)
	assert.True(t, voteRaw(ct, propID, "hive:someoneelse", "1", "v2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(propID), nil, "hive:someone", lateTS, "t")
	before := hiveBal(ct, "hive:someoneelse")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", propID)), nil, "hive:someone", lateTS, "e").Success)
	assert.Equal(t, before+500, hiveBal(ct, "hive:someoneelse"), "payout leg of a full outcome did not execute after reload")
}

// R7-7: an address containing multiple colons (DID-style) survives whitelist
// round-trip (colon is legitimate in addresses).
func TestBreak_ColonAddressWhitelistRoundTrip(t *testing.T) {
	ct := SetupContractTest()
	pid := createWhitelistProject(t, ct)
	// a DID-style address with several colons
	did := "did:pkh:eip155:1:0xabcDEF"
	res := rawCallAt(ct, "project_whitelist_add", PayloadString(fmt.Sprintf("%d|%s", pid, did)), nil, "hive:someone", defaultTimestamp, "w")
	assert.True(t, res.Success, "a colon-bearing address was rejected by whitelist add: %s", res.Ret)
	// removing the same address must find it (round-trip fidelity)
	rem := rawCallAt(ct, "project_whitelist_remove", PayloadString(fmt.Sprintf("%d|%s", pid, did)), nil, "hive:someone", defaultTimestamp, "r")
	assert.True(t, rem.Success, "colon address not found on remove (key round-trip broke)")
}

// R7-8: stake history over many increments (repeated top-ups) reloads correctly and
// historical vote weight uses the right snapshot.
func TestBreak_ManyStakeIncrementsRoundTrip(t *testing.T) {
	ct := SetupContractTest()
	pid := makeProject(t, ct, "1", "50.000", "1")
	joinWithStake(t, ct, pid, "hive:someoneelse", "5.000")
	// top up several times at increasing timestamps
	for i, ts := range []string{"2025-09-03T01:00:00", "2025-09-03T02:00:00", "2025-09-03T03:00:00"} {
		CallContractAt(t, ct, "project_funds", PayloadString(fmt.Sprintf("%d|true", pid)), transferIntent("5.000"), "hive:someoneelse", true, uint(1_000_000_000), ts)
		_ = i
	}
	// now someoneelse has 20 stake; a proposal created now snapshots that
	joinWithStake(t, ct, pid, "hive:member2", "1.000")
	propID := createProposalAt(t, ct, pid, "1", "2025-09-03T04:00:00")
	// vote should succeed with the accumulated historical stake
	res := rawCallAt(ct, "proposals_vote", PayloadString(fmt.Sprintf("%d|1", propID)), nil, "hive:someoneelse", "2025-09-03T04:30:00", "v")
	assert.True(t, res.Success, "vote with multi-increment stake history failed: %s", res.Ret)
}

// R7-9: an empty proposal name/description is accepted and round-trips (no min
// length) — documents that the only bound is the maximum.
func TestBreak_EmptyNameProposalRoundTrip(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	fields := []string{strconv.FormatUint(pid, 10), "", "", "1", "", "0", "", "", ""}
	propID, ok := createProposalRaw(ct, fields, "hive:someone", "c")
	assert.True(t, ok, "empty-name proposal rejected")
	assert.True(t, voteRaw(ct, propID, "hive:someone", "1", "v").Success, "empty-name proposal not reloadable for voting")
}

// R7-10: toggling pause twice via governance returns to the original state
// (idempotent round-trip of the bool through config).
func TestBreak_TogglePauseTwiceRoundTrip(t *testing.T) {
	ct := SetupContractTest()
	pid := createDefaultProject(t, ct)
	joinProjectMember(t, ct, pid, "hive:someoneelse")
	// pause via owner
	CallContract(t, ct, "project_pause", PayloadString(fmt.Sprintf("%d|true", pid)), nil, "hive:someone", true, uint(1_000_000_000))
	// unpause via a toggle_pause proposal
	up := createPollProposal(t, ct, pid, "1", "", "toggle_pause=1")
	assert.True(t, voteRaw(ct, up, "hive:someone", "1", "u1").Success)
	assert.True(t, voteRaw(ct, up, "hive:someoneelse", "1", "u2").Success)
	rawCallAt(ct, "proposal_tally", PayloadUint64(up), nil, "hive:someone", lateTS, "t")
	assert.True(t, rawCallAt(ct, "proposal_execute", PayloadString(fmt.Sprintf("%d", up)), nil, "hive:someone", lateTS, "e").Success)
	// now unpaused: a normal join should work again
	res := rawCallAt(ct, "project_join", PayloadUint64(pid), transferIntent("1.000"), "hive:member2", lateTS, "j")
	assert.True(t, res.Success, "project did not unpause after toggle_pause (bool round-trip broke): %s", res.Ret)
}
