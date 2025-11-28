package sdk

import (
	"encoding/json"
	_ "okinoko_dao/runtime"
	"strconv"
)

//go:wasmimport sdk console.log
func log(s *string) *string

// Log writes a message to the wasm console so we can trace contract steps.
// Example payload: sdk.Log("hello dao")
func Log(s string) {
	log(&s)
}

//go:wasmimport sdk db.set_object
func stateSetObject(key *string, value *string) *string

//go:wasmimport sdk db.get_object
func stateGetObject(key *string) *string

//go:wasmimport sdk db.rm_object
func stateDeleteObject(key *string) *string

//go:wasmimport sdk system.get_env
func getEnv(arg *string) *string

//go:wasmimport sdk system.get_env_key
func getEnvKey(arg *string) *string

//go:wasmimport sdk hive.get_balance
func getBalance(arg1 *string, arg2 *string) *string

//go:wasmimport sdk hive.draw
func hiveDraw(arg1 *string, arg2 *string) *string

//go:wasmimport sdk hive.transfer
func hiveTransfer(arg1 *string, arg2 *string, arg3 *string) *string

//go:wasmimport sdk hive.withdraw
func hiveWithdraw(arg1 *string, arg2 *string, arg3 *string) *string

//go:wasmimport sdk contracts.read
func contractRead(contractId *string, key *string) *string

//go:wasmimport sdk contracts.call
func contractCall(contractId *string, method *string, payload *string, options *string) *string

// var envMap = []string{
// 	"contract.id",
// 	"tx.origin",
// 	"tx.id",
// 	"tx.index",
// 	"tx.op_index",
// 	"block.id",
// 	"block.height",
// 	"block.timestamp",
// }

//go:wasmimport env abort
func abort(msg, file *string, line, column *int32)

//go:wasmimport env revert
func revert(msg, symbol *string)

// Abort stops execution immediately and surfaces the message to the chain, so use sparingly.
// Example payload: sdk.Abort("no stake")
func Abort(msg string) {
	ln := int32(0)
	abort(&msg, nil, &ln, &ln)
	panic(msg)
}

// Revert throws a named error back to the caller (like revert in solidity) with a short symbol.
// Example payload: sdk.Revert("bad input", "input_error")
func Revert(msg string, symbol string) {
	revert(&msg, &symbol)
}

// StateSetObject stores a key/value string pair into contract kv storage.
// Example payload: sdk.StateSetObject("count", "5")
func StateSetObject(key string, value string) {
	stateSetObject(&key, &value)
}

// StateGetObject fetches a key and returns nil when missing.
// Example payload: sdk.StateGetObject("count")
func StateGetObject(key string) *string {
	return stateGetObject(&key)
}

// StateDeleteObject removes the key entirely, handy for cleanup.
// Example payload: sdk.StateDeleteObject("count")
func StateDeleteObject(key string) {
	stateDeleteObject(&key)
}

// GetEnv pulls the JSON env blob from the chain and maps it to Env struct.
// Example payload: sdk.GetEnv()
func GetEnv() Env {
	envStr := *getEnv(nil)
	env := Env{}
	// envMap := map[string]interface{}{}
	json.Unmarshal([]byte(envStr), &env)
	envMap := map[string]interface{}{}
	json.Unmarshal([]byte(envStr), &envMap)

	requiredAuths := make([]Address, 0)
	for _, auth := range envMap["msg.required_auths"].([]interface{}) {
		addr := auth.(string)
		requiredAuths = append(requiredAuths, Address(addr))
	}
	requiredPostingAuths := make([]Address, 0)
	for _, auth := range envMap["msg.required_posting_auths"].([]interface{}) {
		addr := auth.(string)
		requiredPostingAuths = append(requiredPostingAuths, Address(addr))
	}

	env.Sender = Sender{
		Address:              Address(envMap["msg.sender"].(string)),
		RequiredAuths:        requiredAuths,
		RequiredPostingAuths: requiredPostingAuths,
	}

	// env.ContractId = envMap["contract.id"].(string)
	// env.Index = envMap["tx.index"].(int64)
	// env.OpIndex = envMap["tx.op_index"].(int64)

	// for _, v := range envMap {
	// 	switch v {
	// 	case "contract.id":
	// 		env.CONTRACT_ID = *_GET_ENV(&v)
	// 	case "tx.origin":
	// 		env.TX_ORIGIN = *_GET_ENV(&v)
	// 	case "tx.id":
	// 		env.TX_ID = *_GET_ENV(&v)
	// 	case "tx.index":
	// 		indexStr := *_GET_ENV(&v)
	// 		index, err := strconv.Atoi(indexStr)
	// 		if err != nil {
	// 			Log("Das broken: " + err.Error())
	// 			panic(fmt.Sprintf("Failed to parse index: %s", err))
	// 		}
	// 		env.INDEX = index
	// 	case "tx.op_index":
	// 		opIndexStr := *_GET_ENV(&v)
	// 		opIndex, err := strconv.Atoi(opIndexStr)
	// 		if err != nil {
	// 			panic(fmt.Sprintf("Failed to parse op_index: %s", err))
	// 		}
	// 		env.OP_INDEX = opIndex
	// 	case "block.id":
	// 		env.BLOCK_ID = *_GET_ENV(&v)
	// 	case "block.height":
	// 		heightStr := *_GET_ENV(&v)
	// 		height, err := strconv.ParseUint(heightStr, 10, 64)
	// 		if err != nil {
	// 			panic(fmt.Sprintf("Failed to parse block height: %s", err))
	// 		}
	// 		env.BLOCK_HEIGHT = height
	// 	case "block.timestamp":
	// 		env.TIMESTAMP = *_GET_ENV(&v)
	// 	default:
	// 		panic(fmt.Sprintf("Unknown environment variable: %s", v[0]))
	// 	}
	// }
	return env
}

// GetEnvStr returns the raw JSON environment string without parsing.
// Example payload: sdk.GetEnvStr()
func GetEnvStr() string {
	return *getEnv(nil)
}

// GetEnvKey pulls a single env key (like tx.id) to avoid parsing the whole struct.
// Example payload: sdk.GetEnvKey("tx.id")
func GetEnvKey(key string) *string {
	return getEnvKey(&key)
}

// GetBalance queries hive balance for the given account+asset combo.
// Example payload: sdk.GetBalance(sdk.Address("hive:foo"), sdk.AssetHive)
func GetBalance(address Address, asset Asset) int64 {
	addr := address.String()
	as := asset.String()
	balStr := *getBalance(&addr, &as)
	bal, err := strconv.ParseInt(balStr, 10, 64)
	if err != nil {
		panic(err)
	}
	return bal
}

// HiveDraw pulls tokens from the caller to the contract within the transfer.allow limit.
// Example payload: sdk.HiveDraw(1000, sdk.AssetHive)
func HiveDraw(amount int64, asset Asset) {
	amt := strconv.FormatInt(amount, 10)
	as := asset.String()
	hiveDraw(&amt, &as)
}

// HiveTransfer sends tokens from the contract towards a user address.
// Example payload: sdk.HiveTransfer(sdk.Address("hive:foo"), 500, sdk.AssetHbd)
func HiveTransfer(to Address, amount int64, asset Asset) {
	toaddr := to.String()
	amt := strconv.FormatInt(amount, 10)
	as := asset.String()
	hiveTransfer(&toaddr, &amt, &as)
}

// HiveWithdraw unwraps contract-held funds into the Hive layer (savings etc.).
// Example payload: sdk.HiveWithdraw(sdk.Address("hive:foo"), 50, sdk.AssetHive)
func HiveWithdraw(to Address, amount int64, asset Asset) {
	toaddr := to.String()
	amt := strconv.FormatInt(amount, 10)
	as := asset.String()
	hiveWithdraw(&toaddr, &amt, &as)
}

// ContractStateGet reads another contract's state key (view-only).
// Example payload: sdk.ContractStateGet("contract:demo", "cfg")
func ContractStateGet(contractId string, key string) *string {
	return contractRead(&contractId, &key)
}

// ContractCall performs a synchronous call into another contract with optional intents.
// Example payload: sdk.ContractCall("contract:demo", "ping", "{}", nil)
func ContractCall(contractId string, method string, payload string, options *ContractCallOptions) *string {
	optStr := ""
	if options != nil {
		optByte, err := json.Marshal(&options)
		if err != nil {
			Revert("could not serialize options", "sdk_error")
		}
		optStr = string(optByte)
	}
	return contractCall(&contractId, &method, &payload, &optStr)
}
