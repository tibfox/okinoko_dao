package main

import (
	"okinoko_dao/sdk"
	"strconv"
	"time"
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

// validAssets lists the supported asset types.
var validAssets = []string{sdk.AssetHbd.String(), sdk.AssetHive.String()}

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

// nowUnix returns the current Unix timestamp.
// It prefers the chain's block timestamp from the environment if available.
func nowUnix() int64 {
	if ts := currentEnv().Timestamp; ts != "" {
		if v, ok := parseTimestamp(ts); ok {
			return v
		}
	}
	if tsPtr := sdk.GetEnvKey("block.timestamp"); tsPtr != nil && *tsPtr != "" {
		if v, ok := parseTimestamp(*tsPtr); ok {
			return v
		}
	}
	return time.Now().Unix()
}

// parseTimestamp accepts unix seconds or iso-ish strings since the env flips formats sometimes.
func parseTimestamp(val string) (int64, bool) {
	if v, err := strconv.ParseInt(val, 10, 64); err == nil {
		return v, true
	}
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t.Unix(), true
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", val, time.UTC); err == nil {
		return t.Unix(), true
	}
	return 0, false
}
