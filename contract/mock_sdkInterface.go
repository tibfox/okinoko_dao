package contract

import (
	"okinoko_dao/sdk" // import your real SDK
)

// RealSDK struct will implement the SDK interface using the actual SDK methods.
type RealSDK struct{}

// Log method implementation using the actual SDK's logging functionality.
func (r *RealSDK) Log(msg string) {
	// Assuming sdk has a method Log that takes a string message
	sdk.Log(msg)
}

// SDK interface
type SDKInterface interface {
	Log(msg string)
	// Call(name string, args ...string) string
}

// globals
// var envInterface Env
var sdkInterface SDKInterface

func InitSKMocks(mock bool) {
	if mock {
		// envInterface = &MockEnv{}
		sdkInterface = &MockSDK{}
	} else {
		// envInterface = &RealEnv{}
		sdkInterface = &RealSDK{}
	}
}

// Example mocks
type MockSDK struct{}

func (m *MockSDK) Log(msg string) { println("MOCK LOG:", msg) }

// type MockEnv struct{}

// func (m *MockEnv) Call(name string, args ...string) string {
// 	println("MOCK SDK CALL:", name)
// 	return "mocked"
// }
