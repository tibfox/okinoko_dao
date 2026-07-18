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

---

## Rounds 4–8 (deep adversarial passes — 113 adversarial tests total, suite 259 green)

Added `adversarial_round4..8_test.go`. Five more real bugs found and fixed:

### C9 — HIGH: pause bypass via payout/ICC rider
`allowsPauseMeta` only inspected `Outcome.Meta` (a single toggle_pause/owner key), so a
`toggle_pause` proposal could also carry a `Payout`/`ICC` and execute it **while the
project was frozen**. New `outcomeIsPauseSafe` requires no payout/ICC on any proposal
created or executed under the pause exception. *(TestBreak_PauseBypassPayoutRider)*

### C10 — MEDIUM: conflicting owner meta / map-iteration determinism
A proposal could carry both `update_owner` and `remove_owner`; the final owner depended
on Go map-iteration order (consensus hazard). Now rejected at creation, and the meta +
ICC-asset loops iterate in **sorted order** so every validator applies them identically.
*(TestBreak_ConflictingOwnerMetaRejected)*

### C11 — MEDIUM: sub-milliunit cost/stake rounded to zero → free proposals
A positive `ProposalCost`/`StakeMinAmt` below `1/AmountScale` (e.g. 0.0004) silently
became 0. Rejected at config-normalize and via `update_proposalCost`. *(TestBreak_SubMilliunit*)*

### C12 — MEDIUM: unvalidated intent limits
`ParseFloat` accepts `-5`/`NaN` with no error, so a negative/NaN/zero `transfer.allow`
limit reached `HiveDraw`. Now rejected (`!(limit > 0)`). Also `FloatToAmount`'s upper
guard used `>` against `float64(MaxInt64)` (== 2^63), letting exactly 2^63 wrap/ trap;
changed to `>=`. *(TestBreak_NegativeIntentLimitRejected, TestBreak_HugeDepositGracefulReject)*

### C13 — MEDIUM: >45 proposal options crash the wasm
`MaxProposalOptions` was 50, but a proposal with ~46+ options exhausts the wasm heap
(nil-deref trap) during the per-option save loop — the advertised max was unreachable.
Capped to **40** (proven safe with margin) so 41+ reject cleanly at parse.
*(TestBreak_MaxOptionsRoundTrip)*  Also hardened `readString` against a 32-bit length-
prefix truncation (defense-in-depth; not entrypoint-reachable).

### Design notes (intended behaviour — surfaced, not changed)
- **Kick is all-or-nothing** — a batch naming the owner/a payout-locked member aborts
  entirely (TestKickMemberCannotKickOwner). Documented.
- **Direct owner whitelist add is uncapped** (TestOwnerWhitelistAddNoLimit); only the
  proposal meta path enforces the 50 cap. Left as-is: measured safe to 2000+ entries
  (no wasm crash, owner pays gas). Optional generous ceiling could be added.
- **Exact-50 threshold is inclusive** (`>=`) — set >50 (default 50.001) for strict majority.
- **Free + democratic DAOs have no Sybil resistance**; **anyone can donate to any
  treasury**; **ICC debits the full allowance** (a callee that under-draws strands the
  difference); **kicked/departed voters keep their cast weight** (snapshot). All documented.

### Confirmed robust (passing probes)
Multi-asset treasury isolation & conservation; cross-project isolation (treasury, votes,
IDs, membership enforcement via getMember); duplicate-asset draw reverts; codec round-trip
fidelity (full outcome, colon/DID addresses, max name, many stake increments, empty name);
poll-never-executes; fractional-amount precision; quorum ceil; toggle-pause round-trip.

---

## Review passes 9–10 (suite 280 green)

Added `adversarial_round9_test.go` (NaN/Inf config) and `adversarial_round10_test.go`
(URL/scheme + length bounds). Two more real bugs found and fixed:

### C14 — HIGH: NaN/Inf floats bypassed all config validation
`ParseFloat` accepts `NaN`/`Inf` with no error, and every NaN comparison is false, so a
`NaN` threshold/quorum/cost slipped past every `< Min || > Max` range check:
- **NaN threshold** → `weight/denom >= NaN` is always false → the DAO can never pass
  anything (governance bricked).
- **NaN quorum** → `uint64(ceil(NaN))` == 0 → quorum always met (bypass).
- **NaN cost** → `cost > 0` is false → every proposal is free.

Fixed: `parseFloatField` rejects NaN/Inf; threshold/quorum meta bounds rewritten in
positive form (`!(v >= Min && v <= Max)`, which catches NaN); `update_proposalCost`
guards NaN/Inf. Reachable at project creation **and** via governance meta updates.
*(TestBreak_NaN{Threshold,Quorum,Cost}Rejected + ...ViaMetaRejected)*

### C15 — LOW/MED: unbounded metadata/URL fields (gas-griefing / state bloat)
Proposal/project name and description were length-capped, but the free-form
**metadata** and **URL** fields were not — an oversized blob bloats the proposal record
that every vote/tally reloads. Now bounded (`MaxDescriptionLength` / `MaxURLLength`) at
create and via `update_url`. *(TestBreak_*LengthBounded)*

### Confirmed robust (passing probes)
Option URLs are https-only; non-owner and autonomous-project direct pause rejected;
empty/whitespace payloads rejected on every entrypoint (no panic); negative/typo'd vote
choices rejected; empty option text rejected; member-cache coherency (save updates,
delete evicts); valid configs still work after the NaN guard.

---

## Follow-up: unknown-meta-key rejection (was a documented footgun, now fixed)

### C16 — MED: unknown/typo'd governance meta keys were silently ignored
The execute-time meta `switch` has no default case, so a proposal with a typo'd key
(e.g. `update_treshold=60`) passed quorum/threshold and "executed" while enacting
nothing — voters believed a change took effect that silently didn't. Now
`parseMetadataField` rejects any key not in `isKnownMetaKey` at proposal creation.
Flipped the author's `TestWhitelistProposalUnknownKeyIgnored` (and two round-7
design-note tests) to assert rejection. Suite: 280 green on current prod node.
