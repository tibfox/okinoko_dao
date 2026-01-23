Confirmed Issues
1. Proposal Cost Uses Deprecated Field (HIGH)
proposals.go:115 - Proposal costs go to prj.Funds (deprecated), not the treasury:


prj.Funds += costAmount
But payout checks use getTreasuryBalance():


treasuryBalance := getTreasuryBalance(prj.ID, asset)  // Line 272
Impact: Proposal costs are stored in prj.Funds but payouts are drawn from treasury - these are separate balances! Proposal cost fees aren't available for payouts.

Similarly, proposals.go:566-570 for refunds:


if prj.Funds < refundAmount {
    refund = false
} else {
    prj.Funds -= refundAmount
2. NFT Membership Check Too Loose (MEDIUM)
projects.go:309:


return editions != nil && *editions != "[]" && *editions != ""
Any non-empty response (including error strings like "invalid" or "error") passes the NFT check. Should validate it's actually a valid JSON array with NFT data.

Issues That Are NOT Bugs (Clarification)
Vote weight subtraction: Actually correct - each voter's weight applies to each option they vote for, so subtracting from each previously voted option is correct behavior.

Empty votes: The loop doesn't run with empty choices, so VoterCount isn't incremented - empty votes don't affect quorum.

Zero member snapshot: A project always has at least 1 member (creator joins on create), and only members can vote, so this edge case can't be exploited.

Would you like me to fix the proposal cost issue (make it use treasury instead of deprecated Funds field)?