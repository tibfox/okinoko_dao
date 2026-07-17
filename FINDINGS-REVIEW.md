# Okinoko DAO — code & logic review

Adversarial review of `contract/` with 33 new logic tests in
`test/adversarial_test.go`. Full suite: **180 tests, 174 pass, 6 fail**. Every
failing test reproduces a real bug (each asserts the behaviour a correct contract
*should* have). The original 147 tests are unaffected.

Run: `go test -p 1 -count=1 -run 'TestBreak_|TestGuard_|TestBound_' ./test/...`

---

## Confirmed bugs (failing tests)

### C1 — CRITICAL: a down-voted proposal still executes its payout/meta/ICC
`proposals.go` tally (`:170-212`) marks a proposal `Passed` when the *highest-weight*
option clears quorum+threshold, and stores the winner in `ResultOptionID`.
`ExecuteProposal` (`:229-551`) then runs `Outcome.Payout/Meta/ICC` whenever
`State == Passed` and **never reads `ResultOptionID`**. Default options are
`no`=0 / `yes`=1, so a proposal the members vote **down** (majority on "no")
executes anyway.
*Test:* `TestBreak_NoOptionWinningStillPaysOut` — recipient balance went
`199000 → 199500` after everyone voted NO. Treasury drained on a rejected proposal.
*Fix direction:* execute must gate on the winning option's semantics — e.g. only a
designated "approve" option (index 1 for the default ballot) may trigger the
outcome; custom multi-option proposals should be poll-only (they already are).

### C2 — HIGH: quorum counts per-option votes, not distinct voters
`votes.go:166-167` increments each chosen option's `VoterCount`; tally sums
`VoterCount` across options (`proposals.go:180`) and compares to a member-count
quorum. A single member selecting N options contributes N to the count.
*Test:* `TestBreak_QuorumInflationViaMultiSelect` — one whale voting `"0,1"` meets a
2-of-3 quorum alone and drains the treasury.
*Fix direction:* count distinct voters (e.g. increment a per-proposal voter set
once per ballot, not per option).

### C3 — HIGH: free-membership DAOs can never vote (governance dead)
`votes.go:110-113` derives weight only from stake history and aborts on
`weight == 0`. Free DAOs (`StakeMinAmt <= 0`) store `Stake = 0` for every member,
so every vote aborts. Democratic mode has no equal-weight branch.
*Test:* `TestBreak_FreeMembershipDaoCanVote` — a member of a free DAO cannot vote.
*Fix direction:* in democratic mode use weight 1; distinguish "no history" from a
legitimate 0 stake.

### C4 — HIGH: negative payout amounts are accepted
`parsePayoutField` (`payload.go:423`) has no positivity check (the ICC path at
`:642` does). A negative amount passes the `treasury < amount` guard, and
`removeTreasuryFunds` computes `current - (-x) = current + x`, inflating treasury
accounting; `HiveTransfer` also receives a negative value.
*Test:* `TestBreak_NegativePayoutRejected` — a proposal with `...:-100:hive` is
accepted at creation.
*Fix direction:* `if amount <= 0 { abort }` in `parsePayoutField` (mirror ICC).

### C5 — MEDIUM: proposal-duration integer overflow → immediate tally
`CreatedAt + int64(DurationHours)*3600` (`proposals.go:165`) overflows for large
`DurationHours` (e.g. `MaxUint64` → `int64(-1)*3600` = negative), putting the
deadline in the past so the proposal can be tallied instantly.
*Test:* `TestBreak_ProposalDurationOverflow`.
*Fix direction:* bound `DurationHours` (and `ExecutionDelayHours`) to a sane max;
same overflow exists in the execution-delay math.

### C6 — LOW: `AddFunds` ignores project pause
`JoinProject` and `LeaveProject` reject paused projects; `AddFunds`
(`projects.go:366`) does not, so a member can grow stake/voting weight while paused.
*Test:* `TestBreak_AddFundsWhilePausedBlocked`.
*Fix direction:* add the `if prj.Paused { abort }` check.

---

## Noted (analysed, not cleanly triggered as a failing test)

- **Payout-lock griefing** — locks are set at *creation* for any proposal naming an
  address, for the whole (attacker-chosen) voting window, blocking that member's
  `leave`/`kick` even if the proposal never passes. `TestBound_PayoutTargetCannotLeaveDuringVote`
  documents the (intended) lock; the griefing angle is the unbounded duration.
- **Departed-voter weight retention** — `leave`/`kick` refund stake but never remove
  the member's already-cast option weight or vote receipt; snapshot semantics keep
  it counting at tally. In `TestBreak_VoteThenLeaveWeightRetained` quorum happened to
  block the payout, so no loss was observed — but the weight is retained.
- **kickMember lacks the `StakeTotal` underflow guard** that `LeaveProject` has
  (`projects.go:467` vs `:314-317`).
- **create-vs-join deposit asymmetry** — creator's excess over `StakeMinAmt` goes to
  treasury; a joiner's entire deposit becomes personal stake (voting weight).
- **Autonomous projects are re-ownable** via a passing `update_owner` proposal.
- **Map-iteration determinism** — `Outcome.Meta`/`ICC` iterate Go maps; order is
  per-key idempotent today, but multi-key proposals rely on that staying true.

## Hardening verified working (passing GUARD tests)
`validateAddress` (length / control-char / delimiter rejection), the stake-history
key-collision fix, `FloatToAmount` overflow guard, and the treasury/stake
overflow-safe math all hold under the new tests.

---

## Round 2 & 3 (deeper adversarial passes)

Added `test/adversarial_round2_test.go` (12) and `test/adversarial_round3_test.go` (14).
Full suite now **206 tests, all green**. Two more real bugs found and fixed:

### C7 — HIGH: empty ballots inflated quorum (regression from the C2 fix)
The new distinct-voter counter bumped `VoterCount` for *any* ballot, including one
that selected no options — so members casting empty ballots could satisfy quorum
without supporting anything. Fixed: `VoteProposal` now rejects empty ballots
(`len(Choices) == 0`). *Test:* `TestBreak_EmptyBallotDoesNotInflateQuorum`.

### C8 — HIGH: ICC feature completely unreachable (dead feature)
`decodeCreateProposalArgs` read only `parts[10]` for the ICC field, but an ICC
entry is `contract|function|payload|assets` — its own pipes make it span every
part from index 10 on. Every ICC proposal (including the README's own example)
aborted with "invalid ICC entry format". Fixed: rejoin `parts[10:]` as the ICC
field. *Tests:* `TestBreak_ICCReachableAfterDecoderFix`,
`TestBreak_ICCOnlyCreatorCanExecute`.

### Design notes (intended behaviour, documented not changed)
- **Soft voting deadline** — votes are accepted after `DurationHours` until someone
  tallies (asserted intended by `TestVoteAllowedAfterDurationBeforeTally`). Carries
  a last-mover-advantage caveat. `TestBreak_VoteAfterDeadlineStillAllowed_DesignNote`.
- **Majority self-payout** — a majority can pay the treasury to itself; the
  guardrails are quorum/threshold, not payout-target limits.
  `TestBreak_MajoritySelfPayoutDrains_DesignNote`.
- **Same-block stake top-up** — `getStakeAtTime` uses `Timestamp <= creationTime`, so
  a top-up in the *same block* as proposal creation would count; a reactive top-up
  lands in a later block (later timestamp) and does not. Left as-is (`<` would break
  legitimate same-block join→propose→vote).

### Confirmed robust (passing probes)
Poll payouts never execute; duplicate choices don't multiply weight; democratic
2/3-passes-1/3-fails math; quorum exact boundary; cancel lock-release, double-cancel,
cancel-executed; ownership transfer/remove_owner lifecycle; whitelist consumption on
join; NFT gating; historical stake weight; kick-via-proposal refunds; and malformed
payloads all abort cleanly.
