// Package main is a tiny companion contract used ONLY by the test suite.
//
// It exists to exercise the cross-contract paths of the DAO that cannot be
// reached from a single-contract harness:
//
//   - reenter:  calls back into the DAO's proposal_execute, so the re-entrancy
//     guard (terminal state committed before payouts/ICC) can be proven.
//   - draw:     draws its transfer.allow allowance, so an ICC that grants assets
//     can be shown to actually deliver them (the token/limit intent fix).
//   - delegate: forwards a call to the DAO, so the intended delegation behaviour
//     (the DAO acts for the original signer, not this contract) is asserted.
//   - noop:     succeeds and does nothing.
//
// Payload format for every method: "<daoContractId>~<arg>" (~ avoids the ICC field
// grammar, which is contract|function|payload|assets).
package main

import (
	"strings"

	"okinoko_dao/sdk"
)

func main() {}

// split returns the DAO contract id and the remaining argument.
func split(payload *string) (string, string) {
	if payload == nil {
		sdk.Abort("payload required")
	}
	raw := strings.TrimSpace(*payload)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	parts := strings.SplitN(raw, "~", 2)
	if len(parts) < 2 {
		sdk.Abort("payload must be <daoId>~<arg>")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

// Reenter calls back into the DAO's proposal_execute for the given proposal id,
// but only ONCE per transaction. Used to prove a hostile ICC callee cannot replay
// a payout.
//
// The one-shot guard is essential to make this a real attack rather than a
// self-DoS: unbounded recursion trips CONTRACT_CALL_MAX_RECURSION_DEPTH (20), and
// that abort unwinds the whole transaction, so the drain would be rolled back and
// invisible. Re-entering exactly once stays under the limit, so the second payout
// COMMITS — which is precisely the bug the DAO must prevent.
//
//go:wasmexport reenter
func Reenter(payload *string) *string {
	dao, proposalID := split(payload)
	if seen := sdk.StateGetObject("r"); seen != nil && *seen != "" {
		done := "already-reentered"
		return &done
	}
	sdk.StateSetObject("r", "1")
	sdk.ContractCall(dao, "proposal_execute", proposalID, nil)
	ok := "reentered"
	return &ok
}

// Draw pulls `arg` (a decimal amount string) of HIVE from the transfer.allow
// allowance granted to this contract, then keeps it. Used to prove that an ICC
// actually delivers the assets the DAO debited from its treasury.
//
//go:wasmexport draw
func Draw(payload *string) *string {
	_, amount := split(payload)
	// amount arrives as base units (e.g. "1000" == 1.000 HIVE)
	var v int64
	for i := 0; i < len(amount); i++ {
		c := amount[i]
		if c < '0' || c > '9' {
			sdk.Abort("draw amount must be digits (base units)")
		}
		v = v*10 + int64(c-'0')
	}
	if v <= 0 {
		sdk.Abort("draw amount must be positive")
	}
	sdk.HiveDraw(v, sdk.AssetHive)
	ok := "drew"
	return &ok
}

// Delegate forwards an arbitrary call to the DAO: payload is
// "<daoId>~<action>::<daoPayload>". The DAO should act for the ORIGINAL signer
// (msg.sender), not for this contract — that is the delegation feature.
//
//go:wasmexport delegate
func Delegate(payload *string) *string {
	dao, rest := split(payload)
	parts := strings.SplitN(rest, "::", 2)
	if len(parts) < 2 {
		sdk.Abort("delegate arg must be <action>::<payload>")
	}
	sdk.ContractCall(dao, strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil)
	ok := "delegated"
	return &ok
}

// Noop succeeds without doing anything (a benign ICC target).
//
//go:wasmexport noop
func Noop(payload *string) *string {
	ok := "noop"
	return &ok
}

// NftOwned / NftNone stand in for a membership-NFT contract. checkNFTMembership
// treats a result of "[]" or "" as "caller does not own the NFT" and anything else
// as ownership, so these two exports let the DAO's membership gate be tested for
// real. Before they existed the gate tests pointed at a contract that was never
// registered, so the join failed with a host-level "contract not found" and the
// gate logic itself was never reached.
//
//go:wasmexport nft_owned
func NftOwned(payload *string) *string {
	editions := "[1]"
	return &editions
}

//go:wasmexport nft_none
func NftNone(payload *string) *string {
	none := "[]"
	return &none
}

// NftBalanceZero / NftBalanceTwo mimic a magi_nft / ERC-1155 balanceOf reply,
// which is {"balance":N} rather than a JSON array. checkNFTMembership must read
// the balance and treat 0 as "not owned"; without that a zero balance (never
// "[]" or "") would wrongly satisfy the gate.
//
//go:wasmexport nft_balance_zero
func NftBalanceZero(payload *string) *string {
	resp := "{\"balance\":0}"
	return &resp
}

//go:wasmexport nft_balance_two
func NftBalanceTwo(payload *string) *string {
	resp := "{\"balance\":2}"
	return &resp
}
