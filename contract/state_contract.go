package main

import (
	"okinoko_dao/sdk"
	"strings"
)

// -----------------------------------------------------------------------------
// Contract Configuration State
// -----------------------------------------------------------------------------

// isContractInitialized returns true if the contract has been initialized.
func isContractInitialized() bool {
	ptr := sdk.StateGetObject(ContractConfigKey)
	return ptr != nil && *ptr != ""
}

// requireInitialized aborts if the contract has not been initialized.
func requireInitialized() {
	if !isContractInitialized() {
		sdk.Abort("contract not initialized")
	}
}

// loadContractConfig loads the contract configuration from state.
func loadContractConfig() *ContractConfig {
	ptr := sdk.StateGetObject(ContractConfigKey)
	if ptr == nil || *ptr == "" {
		return nil
	}
	return decodeContractConfig(*ptr)
}

// saveContractConfig stores the contract configuration to state.
func saveContractConfig(cfg *ContractConfig) {
	sdk.StateSetObject(ContractConfigKey, encodeContractConfig(cfg))
}

// getContractOwner returns the contract owner address, or nil if not initialized.
func getContractOwner() *sdk.Address {
	cfg := loadContractConfig()
	if cfg == nil {
		return nil
	}
	return &cfg.Owner
}

// isContractOwner returns true if the given address is the contract owner.
func isContractOwner(addr sdk.Address) bool {
	owner := getContractOwner()
	return owner != nil && *owner == addr
}

// -----------------------------------------------------------------------------
// Contract Config Encoding
// -----------------------------------------------------------------------------

// encodeContractConfig serializes ContractConfig to a pipe-delimited string.
// Format: owner|projectCreationPublic
func encodeContractConfig(cfg *ContractConfig) string {
	publicStr := "0"
	if cfg.ProjectCreationPublic {
		publicStr = "1"
	}
	return cfg.Owner.String() + "|" + publicStr
}

// decodeContractConfig deserializes a pipe-delimited string to ContractConfig.
func decodeContractConfig(data string) *ContractConfig {
	parts := strings.Split(data, "|")
	if len(parts) < 2 {
		return nil
	}
	return &ContractConfig{
		Owner:                 AddressFromString(parts[0]),
		ProjectCreationPublic: parts[1] == "1",
	}
}
