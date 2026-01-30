////////////////////////////////////////////////////////////////////////////////
// Okinoko DAO: A universal DAO for the vsc network
// modified: tibfox 2025-12-13
////////////////////////////////////////////////////////////////////////////////

package main

import "okinoko_dao/sdk"

// main is left empty on purpose
func main() {

}

// -----------------------------------------------------------------------------
// Contract Initialization
// -----------------------------------------------------------------------------

// ContractInit initializes the contract with the caller as owner.
// Must be called before any other function.
// Payload: "public" or "owner-only" (project creation permission)
//
//go:wasmexport contract_init
func ContractInit(payload *string) *string {
	if isContractInitialized() {
		sdk.Abort("contract already initialized")
	}

	// Parse permission parameter (unwrap from JSON)
	permission := unwrapPayload(payload, "permission mode required (public or owner-only)")

	publicCreation := permission == "public"

	// Store contract config with caller as owner
	cfg := ContractConfig{
		Owner:                 getSenderAddress(),
		ProjectCreationPublic: publicCreation,
	}
	saveContractConfig(&cfg)

	// Emit init event
	emitInitEvent(cfg.Owner.String(), permission)

	if publicCreation {
		return strptr("initialized with public project creation")
	}
	return strptr("initialized with owner-only project creation")
}

