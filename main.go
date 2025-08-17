////////////////////////////////////////////////////////////////////////////////
// Okinoko DAO: A universal DAO for the vsc network
// created by tibfox 2025-08-12
////////////////////////////////////////////////////////////////////////////////

package main

import (
	"okinoko_dao/contract"
)

func main() {
	debug := true
	contract.InitState(debug)    // true = use MockState
	contract.InitSKMocks(debug)  // enable mock env/sdk
	contract.InitENVMocks(debug) // enable mock env/sdk
}
