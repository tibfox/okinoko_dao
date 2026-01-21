package main

import (
	"strconv"

	"okinoko_dao/sdk"
)

// cachedEnv/cachedTransfer/cachedMembers are scoped to the currently executing transaction.
// Whenever the tx.id changes we refresh sdk.GetEnv() and drop any memoized data to keep reads consistent.
var (
	cachedEnv       sdk.Env
	cachedEnvLoaded bool
	cachedTransfer  *TransferAllow
	cachedMembers   map[string]*Member
)

// currentEnv caches the env per tx.id so we dont poke the host api every few lines and ensures
// subsequent helper calls (intents, sender, timestamps) always see the same snapshot.
func currentEnv() *sdk.Env {
	var currentTx string
	if txPtr := sdk.GetEnvKey("tx.id"); txPtr != nil {
		currentTx = *txPtr
	}
	if !cachedEnvLoaded || cachedEnv.TxId != currentTx {
		cachedEnv = sdk.GetEnv()
		cachedEnvLoaded = true
		cachedTransfer = nil
		cachedMembers = map[string]*Member{}
	}
	return &cachedEnv
}

// currentIntents is just a tiny helper to access intents already pulled above.
func currentIntents() []sdk.Intent {
	return currentEnv().Intents
}

// TransferAllow represents arguments extracted from a transfer.allow intent.
// It specifies the allowed transfer amount (`Limit`) and the asset (`Token`).
type TransferAllow struct {
	Limit float64
	Token sdk.Asset
}

// isValidAsset checks if a given token string is one of the supported assets.
func isValidAsset(token string) bool {
	for _, a := range validAssets {
		if token == a {
			return true
		}
	}
	return false
}

// getFirstTransferAllow scans the provided intents and returns the first valid
// transfer.allow intent as a TransferAllow object. The cached result is cleared automatically
// whenever currentEnv() detects a new transaction so tests do not leak state between calls.
func getFirstTransferAllow() *TransferAllow {
	if cachedTransfer != nil {
		return cachedTransfer
	}
	for _, intent := range currentIntents() {
		if intent.Type == "transfer.allow" {
			token := intent.Args["token"]
			if !isValidAsset(token) {
				sdk.Abort("invalid intent asset")
			}
			limitStr := intent.Args["limit"]
			limit, err := strconv.ParseFloat(limitStr, 32)
			if err != nil {
				sdk.Abort("invalid intent limit")
			}
			ta := &TransferAllow{
				Limit: limit,
				Token: sdk.Asset(token),
			}
			cachedTransfer = ta
			return ta
		}
	}
	return nil
}

// getSenderAddress returns the address of the current transaction sender.
func getSenderAddress() sdk.Address {
	return currentEnv().Sender.Address
}
