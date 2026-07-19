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
			limit, err := strconv.ParseFloat(limitStr, 64)
			// ParseFloat accepts "-5"/"NaN" with a nil error; reject any
			// non-positive or non-finite limit before it reaches HiveDraw.
			if err != nil || !(limit > 0) {
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

// getAllTransferAllows scans all intents and returns all valid transfer.allow intents.
// Unlike getFirstTransferAllow, this does not cache and returns all matching intents.
func getAllTransferAllows() []TransferAllow {
	var transfers []TransferAllow
	for _, intent := range currentIntents() {
		if intent.Type == "transfer.allow" {
			token := intent.Args["token"]
			if !isValidAsset(token) {
				sdk.Abort("invalid intent asset")
			}
			limitStr := intent.Args["limit"]
			limit, err := strconv.ParseFloat(limitStr, 64)
			if err != nil || !(limit > 0) {
				sdk.Abort("invalid intent limit")
			}
			transfers = append(transfers, TransferAllow{
				Limit: limit,
				Token: sdk.Asset(token),
			})
		}
	}
	return transfers
}

// getActorAddress returns the authenticated identity for the current call: the
// ORIGINAL TRANSACTION SIGNER (msg.sender), not the immediate caller.
//
// This is a DELIBERATE product decision, not an oversight. It enables delegation:
// a member can call a helper/integration contract, and that contract acts on the
// DAO *as the member* — the member keeps their own membership, stake and votes
// even when interacting through tooling. Authorizing on msg.caller would instead
// make the helper contract itself the member, which is not the intended UX.
//
// ACCEPTED RISK — confused deputy. The host propagates msg.sender verbatim into
// nested contract call frames (execution-context.go: Caller becomes
// "contract:<id>", Sender is passed through unchanged). Consequently ANY contract
// a member calls can, within that same transaction, call back into this DAO and
// act with that member's full authority: transfer their project away, cast their
// stake, cancel their proposals, force a leave.
//
// The mitigation is social, not technical: members must only interact with
// contracts they trust, exactly as with an ERC-20 approval. If that assumption
// ever becomes untenable, the fix is to authorize on env.Caller instead (a
// one-line change here) — at the cost of the delegation feature above.
//
// NOTE: this does NOT re-open the ICC re-entrancy drain. That was fixed
// independently in ExecuteProposal by committing the terminal state before any
// payout or external call (checks-effects-interactions), so a re-entrant frame
// can no longer observe ProposalPassed and replay a payout.
func getActorAddress() sdk.Address {
	return currentEnv().Sender.Address
}
