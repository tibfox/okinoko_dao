//go:build test
// +build test

package sdk

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// --- WASM function mocks ---

func log(s *string) *string {
	fmt.Println("SDK log:", *s)
	return s
}

func stateSetObject(key *string, value *string) *string {
	fmt.Println("Set object:", *key, *value)
	return nil
}

func stateGetObject(key *string) *string {
	fmt.Println("Get object:", *key)
	dummy := "mock_value"
	return &dummy
}

func stateDeleteObject(key *string) *string {
	fmt.Println("Delete object:", *key)
	return nil
}

func getEnv(arg *string) *string {
	dummy := `{
		"msg.sender": "mock_sender",
		"msg.required_auths": [],
		"msg.required_posting_auths": []
	}`
	return &dummy
}

func getEnvKey(arg *string) *string {
	fmt.Println("GetEnvKey:", *arg)
	dummy := "mock_value"
	return &dummy
}

func getBalance(arg1 *string, arg2 *string) *int64 {
	fmt.Println("GetBalance:", *arg1, *arg2)
	var dummy int64 = 1000
	return &dummy
}

func hiveDraw(arg1 *string, arg2 *string) *string {
	fmt.Println("HiveDraw:", *arg1, *arg2)
	return nil
}

func hiveTransfer(arg1 *string, arg2 *string, arg3 *string) *string {
	fmt.Println("HiveTransfer:", *arg1, *arg2, *arg3)
	return nil
}

func hiveWithdraw(arg1 *string, arg2 *string, arg3 *string) *string {
	fmt.Println("HiveWithdraw:", *arg1, *arg2, *arg3)
	return nil
}

func contractRead(contractId *string, key *string) *string {
	fmt.Println("ContractRead:", *contractId, *key)
	dummy := "mock_contract_value"
	return &dummy
}

func contractCall(contractId *string, method *string, payload *string, options *string) *string {
	fmt.Println("ContractCall:", *contractId, *method, *payload, *options)
	dummy := "mock_call_result"
	return &dummy
}

// --- Wrapper functions (same as in original SDK) ---

func Log(s string) {
	log(&s)
}

func StateSetObject(key string, value string) {
	stateSetObject(&key, &value)
}

func StateGetObject(key string) *string {
	return stateGetObject(&key)
}

func StateDeleteObject(key string) {
	stateDeleteObject(&key)
}

func GetEnv() Env {
	envStr := *getEnv(nil)
	env := Env{}
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

	return env
}

func GetEnvKey(key string) *string {
	return getEnvKey(&key)
}

func GetBalance(address Address, asset Asset) int64 {
	addr := address.String()
	as := asset.String()
	return *getBalance(&addr, &as)
}

func HiveDraw(amount int64, asset Asset) {
	amt := strconv.FormatInt(amount, 10)
	as := asset.String()
	hiveDraw(&amt, &as)
}

func HiveTransfer(to Address, amount int64, asset Asset) {
	toaddr := to.String()
	amt := strconv.FormatInt(amount, 10)
	as := asset.String()
	hiveTransfer(&toaddr, &amt, &as)
}

func HiveWithdraw(to Address, amount int64, asset Asset) {
	toaddr := to.String()
	amt := strconv.FormatInt(amount, 10)
	as := asset.String()
	hiveWithdraw(&toaddr, &amt, &as)
}
