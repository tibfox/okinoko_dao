package contract

import (
	"okinoko_dao/sdk" // import your real SDK
)

// RealSDK struct will implement the ENV interface using the actual SDK methods.
type RealENV struct{}

func (r *RealENV) GetEnv() sdk.Env {
	return sdk.GetEnv()
}

// Get current execution environment variable by a key
func (r *RealENV) GetEnvKey(key string) *string {
	e := sdk.GetEnv()
	switch key {
	case "block.timestamp":
		return &e.Timestamp
	case "tx.id":
		return &e.TxId
	default:
		return nil

	}

}

// ENV interface
type ENVInterface interface {
	GetEnv() sdk.Env
	GetEnvKey(key string) *string
}

// globals
// var envInterface Env
var envInterface ENVInterface

func InitENVMocks(mock bool) {
	if mock {
		// envInterface = &MockEnv{}
		envInterface = &MockENV{}
	} else {
		// envInterface = &RealEnv{}
		envInterface = &RealENV{}
	}
}

// Example mocks
type MockENV struct{}

// Get current execution environment variable by a key
func (r *MockENV) GetEnvKey(key string) *string {
	timestampVal := "2025-01-01T00:00:00.000"
	txIdVal := "0"
	switch key {
	case "block.timestamp":

		return &timestampVal
	case "tx.id":
		return &txIdVal
	default:
		return nil

	}

}

func (m *MockENV) GetEnv() sdk.Env {
	var mockEnvironment sdk.Env

	mockEnvironment.ContractId = "test_ContractId"
	mockEnvironment.TxId = "test_txId"
	mockEnvironment.Index = 0
	mockEnvironment.OpIndex = 0
	mockEnvironment.BlockId = "test_blockId"
	mockEnvironment.BlockHeight = 0
	mockEnvironment.Timestamp = "2025-01-01T00:00:00.000"
	mockEnvironment.Sender = sdk.Sender{
		Address: "hive:test_senderAddress",
		// RequiredAuths: ["hive:test_senderAddress"]
		// ,RequiredPostingAuths: [],Intents: []
	}
	mockEnvironment.Caller = sdk.Caller{
		Address: "hive:test_callerAddress",
		// RequiredAuths: ["hive:test_senderAddress"]
		// ,RequiredPostingAuths: [],Intents: []
	}
	mockEnvironment.Payer = "hive:test_callerAddress"

	return mockEnvironment

}
