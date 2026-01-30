package contract_test

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"vsc-node/lib/test_utils"
	"vsc-node/modules/db/vsc/contracts"
	ledgerDb "vsc-node/modules/db/vsc/ledger"
	stateEngine "vsc-node/modules/state-processing"

	"github.com/stretchr/testify/assert"
)

var _ = embed.FS{} // just so "embed" can be imported

const ContractID = "vsctestcontract"
const ownerAddress = "hive:tibfox"
const defaultTimestamp = "2025-09-03T00:00:00"

//go:embed artifacts/main.wasm
var ContractWasm []byte

// Setup an Instance of a test (initializes contract with public project creation)
func SetupContractTest() *test_utils.ContractTest {
	return setupContractTestWithMode("public")
}

// SetupContractTestOwnerOnly initializes the contract with owner-only project creation
func SetupContractTestOwnerOnly() *test_utils.ContractTest {
	return setupContractTestWithMode("owner-only")
}

// SetupContractTestUninitialized returns a test instance without calling contract_init
func SetupContractTestUninitialized() *test_utils.ContractTest {
	CleanBadgerDB()
	ct := test_utils.NewContractTest()
	ct.RegisterContract(ContractID, ownerAddress, ContractWasm)
	ct.Deposit("hive:someone", 200000, ledgerDb.AssetHive)
	ct.Deposit("hive:someone", 200000, ledgerDb.AssetHbd)
	ct.Deposit("hive:someoneelse", 200000, ledgerDb.AssetHive)
	ct.Deposit("hive:someoneelse", 200000, ledgerDb.AssetHbd)
	ct.Deposit("hive:member2", 200000, ledgerDb.AssetHive)
	ct.Deposit("hive:member2", 200000, ledgerDb.AssetHbd)
	ct.Deposit("hive:outsider", 200000, ledgerDb.AssetHive)
	ct.Deposit("hive:outsider", 200000, ledgerDb.AssetHbd)
	ct.Deposit(ownerAddress, 200000, ledgerDb.AssetHive)
	ct.Deposit(ownerAddress, 200000, ledgerDb.AssetHbd)
	return &ct
}

// setupContractTestWithMode is the internal setup that handles initialization mode
func setupContractTestWithMode(mode string) *test_utils.ContractTest {
	CleanBadgerDB()
	ct := test_utils.NewContractTest()
	ct.RegisterContract(ContractID, ownerAddress, ContractWasm)
	ct.Deposit("hive:someone", 200000, ledgerDb.AssetHive)
	ct.Deposit("hive:someone", 200000, ledgerDb.AssetHbd)
	ct.Deposit("hive:someoneelse", 200000, ledgerDb.AssetHive)
	ct.Deposit("hive:someoneelse", 200000, ledgerDb.AssetHbd)
	ct.Deposit("hive:member2", 200000, ledgerDb.AssetHive)
	ct.Deposit("hive:member2", 200000, ledgerDb.AssetHbd)
	ct.Deposit("hive:outsider", 200000, ledgerDb.AssetHive)
	ct.Deposit("hive:outsider", 200000, ledgerDb.AssetHbd)
	ct.Deposit(ownerAddress, 200000, ledgerDb.AssetHive)
	ct.Deposit(ownerAddress, 200000, ledgerDb.AssetHbd)
	// NOTE: hbd_savings deposits don't work via ct.Deposit() - the ledger system
	// calculates hbd_savings from "stake" operations, not deposits.

	// Initialize the contract
	initContract(&ct, mode)

	return &ct
}

// initContract calls contract_init with the specified mode
func initContract(ct *test_utils.ContractTest, mode string) {
	ct.Call(stateEngine.TxVscCallContract{
		Caller: ownerAddress,
		Self: stateEngine.TxSelf{
			TxId:                 "init-tx",
			BlockId:              "block0",
			Index:                0,
			OpIndex:              0,
			Timestamp:            defaultTimestamp,
			RequiredAuths:        []string{ownerAddress},
			RequiredPostingAuths: []string{},
		},
		ContractId: ContractID,
		Action:     "contract_init",
		Payload:    json.RawMessage(strconv.Quote(mode)),
		RcLimit:    100000,
		Intents:    nil,
	})
}

// clean the db for multiple (sequential) tests
func CleanBadgerDB() {
	err := os.RemoveAll("data/badger")
	if err != nil {
		panic("failed to remove data/badger")
	}
}

// CallContract executes a contract action and asserts basic success
func CallContract(t *testing.T, ct *test_utils.ContractTest, action string, payload json.RawMessage, intents []contracts.Intent, authUser string, expectedResult bool, maxGas uint) (stateEngine.TxResult, uint, map[string][]string) {
	return callContractWithTimestamp(t, ct, action, payload, intents, authUser, expectedResult, maxGas, defaultTimestamp)
}

// CallContractAt executes a call but lets tests override the timestamp for expiry checks.
func CallContractAt(t *testing.T, ct *test_utils.ContractTest, action string, payload json.RawMessage, intents []contracts.Intent, authUser string, expectedResult bool, maxGas uint, timestamp string) (stateEngine.TxResult, uint, map[string][]string) {
	if timestamp == "" {
		timestamp = defaultTimestamp
	}
	return callContractWithTimestamp(t, ct, action, payload, intents, authUser, expectedResult, maxGas, timestamp)
}

// callContractWithTimestamp performs the real invocation, logging gas usage and asserting outcome.
func callContractWithTimestamp(t *testing.T, ct *test_utils.ContractTest, action string, payload json.RawMessage, intents []contracts.Intent, authUser string, expectedResult bool, maxGas uint, timestamp string) (stateEngine.TxResult, uint, map[string][]string) {
	if timestamp == "" {
		timestamp = defaultTimestamp
	}
	fmt.Println(action)
	result, gasUsed, logs := ct.Call(stateEngine.TxVscCallContract{
		Caller: authUser,

		Self: stateEngine.TxSelf{
			TxId:                 fmt.Sprintf("%s-tx", action),
			BlockId:              "block1",
			Index:                0,
			OpIndex:              0,
			Timestamp:            timestamp,
			RequiredAuths:        []string{authUser},
			RequiredPostingAuths: []string{},
		},
		ContractId: ContractID,
		Action:     action,
		Payload:    payload,
		RcLimit:    100000,
		Intents:    intents,
	})

	PrintLogs(logs)
	PrintErrorIfFailed(result)
	fmt.Printf("return msg: %s\n", result.Ret)
	fmt.Printf("gas used: %d\n", gasUsed)
	fmt.Printf("gas max : %d\n", maxGas)
	fmt.Printf("RC used : %d\n", result.RcUsed)

	assert.LessOrEqual(t, gasUsed, maxGas, fmt.Sprintf("Gas %d exceeded limit %d", gasUsed, maxGas))

	if expectedResult {
		assert.True(t, result.Success, "Contract action failed with "+result.Ret)
	} else {
		assert.False(t, result.Success, "Contract action did not fail (as expected)")
	}
	return result, gasUsed, logs
}

// PrintLogs prints all logs from a contract call
func PrintLogs(logs map[string][]string) {
	for key, values := range logs {
		for _, v := range values {
			fmt.Printf("[%s] %s\n", key, v)
		}
	}
}

// PrintErrorIfFailed prints error if the contract call failed
func PrintErrorIfFailed(result stateEngine.TxResult) {
	if !result.Success {
		fmt.Println(result.Err)
	}
}

// PayloadString wraps a Go string into JSON the contract expects.
func PayloadString(val string) json.RawMessage {
	return json.RawMessage([]byte(strconv.Quote(val)))
}

// PayloadUint64 converts an id to its ASCII digits for wasm call payloads.
func PayloadUint64(val uint64) json.RawMessage {
	return json.RawMessage([]byte(strconv.FormatUint(val, 10)))
}

// =============================================================================
// Test Helper Functions
// =============================================================================

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
