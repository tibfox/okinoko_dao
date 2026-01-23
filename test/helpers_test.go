package contract_test

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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

// Setup an Instance of a test
func SetupContractTest() *test_utils.ContractTest {
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
	// NOTE: hbd_savings deposits don't work via ct.Deposit() - the ledger system
	// calculates hbd_savings from "stake" operations, not deposits.

	return &ct
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
