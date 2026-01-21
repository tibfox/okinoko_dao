package main

import (
	"fmt"
	"strconv"

	"okinoko_dao/sdk"
)

// getTreasuryBalance retrieves the balance of a specific asset in the project treasury.
func getTreasuryBalance(projectID uint64, asset sdk.Asset) Amount {
	key := projectTreasuryKey(projectID, asset)
	dataPtr := sdk.StateGetObject(key)
	if dataPtr == nil {
		return 0
	}

	balance, err := strconv.ParseInt(*dataPtr, 10, 64)
	if err != nil {
		return 0
	}
	return Amount(balance)
}

// setTreasuryBalance sets the balance of a specific asset in the project treasury.
func setTreasuryBalance(projectID uint64, asset sdk.Asset, amount Amount) {
	key := projectTreasuryKey(projectID, asset)
	value := fmt.Sprintf("%d", amount)
	sdk.StateSetObject(key, value)
}

// addTreasuryFunds adds funds to a specific asset in the project treasury.
func addTreasuryFunds(projectID uint64, asset sdk.Asset, amount Amount) {
	current := getTreasuryBalance(projectID, asset)
	setTreasuryBalance(projectID, asset, current+amount)
}

// removeTreasuryFunds removes funds from a specific asset in the project treasury.
// Returns false if insufficient balance.
func removeTreasuryFunds(projectID uint64, asset sdk.Asset, amount Amount) bool {
	current := getTreasuryBalance(projectID, asset)
	if current < amount {
		return false
	}
	setTreasuryBalance(projectID, asset, current-amount)
	return true
}

// loadTreasuryMap loads all treasury balances into a map.
// This is useful for migrations and displaying full treasury state.
func loadTreasuryMap(projectID uint64, knownAssets []sdk.Asset) map[sdk.Asset]Amount {
	treasury := make(map[sdk.Asset]Amount)
	for _, asset := range knownAssets {
		balance := getTreasuryBalance(projectID, asset)
		if balance > 0 {
			treasury[asset] = balance
		}
	}
	return treasury
}
