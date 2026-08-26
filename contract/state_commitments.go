package main

import (
	"fmt"
	"strconv"
	"strings"

	"okinoko_dao/sdk"
)

// -----------------------------------------------------------------------------
// Committed balances
//
// The committed balance is the portion of a project's funds that a passed
// `commit_funds` proposal has RESERVED for a later milestone vote. Commit moves
// the amount OUT of the treasury key and INTO this one, which is what makes the
// reservation real: every existing spend path (payout entries, ICC asset
// intents) checks getTreasuryBalance, so once the amount has left that key no
// other proposal can reach it. Nothing reads the committed balance to authorise
// a spend — only release/cancel drain it.
// -----------------------------------------------------------------------------

// getCommittedBalance retrieves the reserved balance of one asset for a project.
func getCommittedBalance(projectID uint64, asset sdk.Asset) Amount {
	key := projectCommittedKey(projectID, asset)
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

// setCommittedBalance writes the reserved balance of one asset for a project.
func setCommittedBalance(projectID uint64, asset sdk.Asset, amount Amount) {
	key := projectCommittedKey(projectID, asset)
	sdk.StateSetObject(key, fmt.Sprintf("%d", amount))
}

// addCommittedFunds increases the reserved balance for an asset.
func addCommittedFunds(projectID uint64, asset sdk.Asset, amount Amount) {
	current := getCommittedBalance(projectID, asset)
	setCommittedBalance(projectID, asset, safeAddAmount(current, amount))
}

// removeCommittedFunds decreases the reserved balance, reporting false if the
// record does not cover the amount. A false here means the committed ledger has
// desynced from the commitment records, so every caller aborts on it rather than
// silently paying out funds that were never reserved.
func removeCommittedFunds(projectID uint64, asset sdk.Asset, amount Amount) bool {
	current := getCommittedBalance(projectID, asset)
	if current < amount {
		return false
	}
	setCommittedBalance(projectID, asset, current-amount)
	return true
}

// -----------------------------------------------------------------------------
// Commitment records
// -----------------------------------------------------------------------------

// encodeCommitment serialises a commitment as projectID|state|entries, where
// entries is a comma-separated list of addr:rawAmount:asset.
//
// The amount is the RAW scaled int64, not the human float: a commitment is
// re-read on release and paid out verbatim, so round-tripping it through a
// decimal string would let a cent evaporate between commit and release.
func encodeCommitment(c *Commitment) string {
	parts := make([]string, 0, len(c.Entries))
	for _, entry := range c.Entries {
		parts = append(parts, fmt.Sprintf("%s:%d:%s",
			AddressToString(entry.Address),
			AmountToInt64(entry.Amount),
			AssetToString(entry.Asset),
		))
	}
	return fmt.Sprintf("%d|%d|%s", c.ProjectID, uint8(c.State), strings.Join(parts, ","))
}

// decodeCommitment parses the encodeCommitment form, aborting on anything it did
// not write itself.
func decodeCommitment(raw string) *Commitment {
	parts := strings.SplitN(raw, "|", 3)
	if len(parts) != 3 {
		sdk.Abort("corrupt commitment record")
	}

	projectID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		sdk.Abort("corrupt commitment project id")
	}
	stateVal, err := strconv.ParseUint(parts[1], 10, 8)
	if err != nil || CommitmentState(stateVal) == CommitmentUnset || stateVal > uint64(CommitmentCancelled) {
		sdk.Abort("corrupt commitment state")
	}

	commitment := &Commitment{
		ProjectID: projectID,
		State:     CommitmentState(stateVal),
	}

	if strings.TrimSpace(parts[2]) == "" {
		sdk.Abort("corrupt commitment: no entries")
	}
	for _, entry := range strings.Split(parts[2], ",") {
		// Addresses are namespaced ("hive:alice"), so they contain the same ':'
		// that separates the fields. Read amount and asset off the END and rejoin
		// everything before them, exactly as parsePayoutField does.
		fields := strings.Split(entry, ":")
		if len(fields) < 3 {
			sdk.Abort("corrupt commitment entry")
		}
		asset := AssetFromString(fields[len(fields)-1])
		rawAmount, err := strconv.ParseInt(fields[len(fields)-2], 10, 64)
		if err != nil {
			sdk.Abort("corrupt commitment amount")
		}
		commitment.Entries = append(commitment.Entries, PayoutEntry{
			Address: AddressFromString(strings.Join(fields[:len(fields)-2], ":")),
			Amount:  Amount(rawAmount),
			Asset:   asset,
		})
	}
	return commitment
}

// saveCommitment persists a commitment under its source proposal's id.
func saveCommitment(proposalID uint64, c *Commitment) {
	sdk.StateSetObject(commitmentKey(proposalID), encodeCommitment(c))
}

// loadCommitment reads a commitment, aborting if the proposal never created one.
func loadCommitment(proposalID uint64) *Commitment {
	ptr := sdk.StateGetObject(commitmentKey(proposalID))
	if ptr == nil || *ptr == "" {
		sdk.Abort(fmt.Sprintf("no commitment created by proposal %d", proposalID))
	}
	return decodeCommitment(*ptr)
}

// commitmentExists reports whether a proposal has already created a commitment.
func commitmentExists(proposalID uint64) bool {
	ptr := sdk.StateGetObject(commitmentKey(proposalID))
	return ptr != nil && *ptr != ""
}

// loadPendingCommitment resolves a release/cancel target and enforces the two
// invariants both actions share: the commitment belongs to the project whose
// members just voted, and it has not already been settled.
//
// The project check is the cross-project guard — commitment ids come from a
// single global proposal counter, so without it a one-member throwaway project
// could vote to release funds committed by an unrelated DAO. The state check is
// the double-spend guard: release and cancel each drain the committed balance,
// so a second settlement of the same record would pay out funds that are no
// longer reserved (or, worse, that a previous cancel already returned to the
// treasury and a third proposal has since spent).
func loadPendingCommitment(projectID uint64, proposalID uint64) *Commitment {
	commitment := loadCommitment(proposalID)
	if commitment.ProjectID != projectID {
		sdk.Abort(fmt.Sprintf("commitment %d belongs to project %d", proposalID, commitment.ProjectID))
	}
	if commitment.State != CommitmentPending {
		sdk.Abort(fmt.Sprintf("commitment %d is already %s", proposalID, commitment.State.String()))
	}
	return commitment
}
