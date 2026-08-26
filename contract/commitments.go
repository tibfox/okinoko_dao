package main

import (
	"fmt"

	"okinoko_dao/sdk"
)

// -----------------------------------------------------------------------------
// Milestone commitments
//
// Two votes, one payout. A `commit_funds` proposal asks the membership to set
// money aside for a beneficiary; a later `release_commitment` proposal asks them
// whether the milestone was actually met and, if so, pays it. `cancel_commitment`
// is the other exit: the milestone was missed, and the funds go back to the
// treasury.
//
// What makes the first vote worth anything is that commit MOVES the money out of
// the treasury key and into the committed key. Every spend path in this contract
// (payout entries in ExecuteProposal, ICC asset intents) authorises itself
// against getTreasuryBalance, so from the moment a commitment exists no other
// proposal — nor a later payout in the same project — can reach those funds.
// Without that move the first vote would be a promise the treasury could not
// keep: nothing today reserves anything, and two proposals that each pass for
// more than the treasury holds simply race, with the loser aborting at execute.
// -----------------------------------------------------------------------------

// commitFunds reserves treasury funds against the CURRENT proposal's id.
//
// The commitment is keyed by the committing proposal's own id rather than by a
// separate counter, so the milestone proposal that will later release it can be
// drafted the moment this one is created — the id is known at creation time, not
// at execution time, and there is no event to scrape in between.
func commitFunds(prj *Project, proposalID uint64, value string) {
	entries := parseCommitmentField(value)

	// One commitment per proposal, and a proposal executes once. This is belt and
	// braces against a second commit overwriting the record while its funds stay
	// reserved — which would strand them, since release/cancel only ever drain
	// what the (overwritten) record lists.
	if commitmentExists(proposalID) {
		sdk.Abort(fmt.Sprintf("proposal %d already committed funds", proposalID))
	}

	// Move each entry from spendable to reserved. removeTreasuryFunds re-reads the
	// balance per call, so several entries drawing on the same asset deduct
	// cumulatively and the last one over the line is the one that aborts.
	for _, entry := range entries {
		if !removeTreasuryFunds(prj.ID, entry.Asset, entry.Amount) {
			sdk.Abort(fmt.Sprintf("insufficient %s funds in treasury to commit", AssetToString(entry.Asset)))
		}
		addCommittedFunds(prj.ID, entry.Asset, entry.Amount)
	}

	saveCommitment(proposalID, &Commitment{
		ProjectID: prj.ID,
		State:     CommitmentPending,
		Entries:   entries,
	})

	// Hold the beneficiaries in the project for as long as the money is earmarked
	// for them, exactly as a passed payout does at tally. Released by whichever of
	// release/cancel settles the commitment.
	incrementPayoutLocks(prj.ID, entries)

	emitCommitmentEvent(prj.ID, proposalID, CommitmentPending, entries)
}

// releaseCommitment pays a pending commitment out to its beneficiaries.
func releaseCommitment(prj *Project, proposalID uint64, value string) {
	targetID := parseCommitmentTarget(value, "release_commitment")
	rejectSelfSettlement(proposalID, targetID, "release_commitment")
	commitment := loadPendingCommitment(prj.ID, targetID)

	// CHECKS-EFFECTS-INTERACTIONS: settle the record BEFORE moving any funds, for
	// the same reason ExecuteProposal commits the proposal state before its
	// payouts. Any abort below reverts this write along with everything else.
	commitment.State = CommitmentReleased
	saveCommitment(targetID, commitment)

	for _, entry := range commitment.Entries {
		// A false here means the committed ledger no longer covers a record that
		// says it is pending. Abort rather than transfer: paying out an unreserved
		// amount would come from whatever else the contract happens to hold.
		if !removeCommittedFunds(prj.ID, entry.Asset, entry.Amount) {
			sdk.Abort(fmt.Sprintf("committed %s balance does not cover commitment %d", AssetToString(entry.Asset), targetID))
		}
		sdk.HiveTransfer(entry.Address, AmountToInt64(entry.Amount), entry.Asset)
		emitFundsRemoved(prj.ID, AddressToString(entry.Address), AmountToFloat(entry.Amount), AssetToString(entry.Asset), false)
	}

	decrementPayoutLocks(prj.ID, commitment.Entries)
	emitCommitmentEvent(prj.ID, targetID, CommitmentReleased, commitment.Entries)
}

// cancelCommitment returns a pending commitment's funds to the treasury.
func cancelCommitment(prj *Project, proposalID uint64, value string) {
	targetID := parseCommitmentTarget(value, "cancel_commitment")
	rejectSelfSettlement(proposalID, targetID, "cancel_commitment")
	commitment := loadPendingCommitment(prj.ID, targetID)

	commitment.State = CommitmentCancelled
	saveCommitment(targetID, commitment)

	for _, entry := range commitment.Entries {
		if !removeCommittedFunds(prj.ID, entry.Asset, entry.Amount) {
			sdk.Abort(fmt.Sprintf("committed %s balance does not cover commitment %d", AssetToString(entry.Asset), targetID))
		}
		addTreasuryFunds(prj.ID, entry.Asset, entry.Amount)
	}

	decrementPayoutLocks(prj.ID, commitment.Entries)
	emitCommitmentEvent(prj.ID, targetID, CommitmentCancelled, commitment.Entries)
}

// rejectSelfSettlement blocks a proposal from settling the commitment it created
// in the same execution.
//
// Meta actions run in sorted key order, so "commit_funds" is applied before
// "release_commitment": without this, one proposal carrying both would reserve
// the funds and immediately pay them out. That is a plain payout wearing a
// commitment's clothes — the two-vote guarantee this feature exists to provide
// would be optional, and readers of the proposal would have to notice that the
// referenced id is the proposal's own to see it. Direct payouts have their own
// field; a commitment must outlive the vote that created it.
func rejectSelfSettlement(proposalID uint64, targetID uint64, action string) {
	if proposalID == targetID {
		sdk.Abort(fmt.Sprintf("%s cannot settle the commitment created by the same proposal", action))
	}
}
