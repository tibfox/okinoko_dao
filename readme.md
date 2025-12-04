# Oinoko DAO – User Guide

This smart contract powers community-driven projects on the [Magi Ecosystem](https://github.com/vsc-eco/). It allows users to create projects, join them, make proposals, vote on proposals, and manage shared funds - all in a transparent, on-chain way powered by Magi smart contracts.

---

## 1. What You Can Do

- **Create a Project**  
  Start your own community project with a name, description, voting rules, and a shared treasury.
  You decide:
  - Who can make proposals (just you, all members or anyone)
  - How members join (fixed fee for equal voting or stake-based for weighted voting)
  - The percentage of votes needed for approval
  - Proposal cost (goes into project funds)
  - Proposal duration (in hours)
  - Minimum/Exact join amounts needed for users to join
  - Optional: Adding an nft on the upcoming Magi nft contract that functions as whitelist for joins

- **Join a Project**  
  Become a member by sending the required join amount (set by the project).
  - In **Democratic voting** projects, every member’s vote counts equally.
  - In **Stake-based voting**, your vote weight depends on your contribution amount.

- **Make a Proposal**  
  Suggest an action or ask the community a question.
  Proposal types:
  - **Yes/No** (can also execute fund transfers or project meta changes if approved)
  - **Single Choice Poll**
  - **Multiple Choice Poll**
  
  Every proposal has:
  - Title, description, and optional metadata
  - Duration for voting
  - Receiver (only for Yes/No fund transfers)
  - Cost (defined by the project, goes into treasury)
  - Project meta keys to update

- **Vote on Proposals**  
  Members vote according to the project’s rules. Votes can be changed until the proposal gets tallied. Tallying and execution are explicit calls (`proposal_tally`, `proposal_execute`) and can be managed through [terminal.okinoko.io](https://terminal.okinoko.io).

- **Send Additional Funds**  
  You can add more funds to the project’s treasury at any time to help the community achieve its goals. In stake-based projects you can also increase your personal stake (and vote weight) by calling `project_funds` with `toStake=1`.
  

- **Project Pause Switch**  
  The project owner can activate a pause switch for the project. No new proposal can be created and no proposal can be executed in this stage. One big exception is a proposal with the only outcome to deactivate that pause switch. These proposals can be created, voted and then executed.
  
  

---

## 2. How Voting Works

1. **Democratic Voting** – Every member has **1 vote**, regardless of stake.  
2. **Stake-based Voting** – Your vote weight = your current stake. You can top it up after joining by adding funds with `toStake=1`.

Every member can change their decision as often as they want until the proposal got tallied.

---

## 3. Proposal Results

When voting ends (and someone calls `proposal_tally`):
- If **Yes/No** and passes → Funds are sent (if applicable) and/or project meta settings are changed.  
- If a poll → Results are recorded on-chain for everyone to see.  
- The project treasury is updated automatically if funds leave the project.

After tallying, anyone can call `proposal_execute` once the configured execution delay has elapsed. Passed proposals remain pending until execution (or cancellation) succeeds.

---

## 4. Contract Calls

All actions are invoked by calling the WASM contract entry points below. Payloads are pipe-separated strings unless stated otherwise.

| Action / Export | Payload | Description | Return |
|-----------------|---------|-------------|--------|
| `project_create` | `name\|description\|votingSystem\|threshold\|quorum\|proposalDuration\|executionDelay\|leaveCooldown\|proposalCost\|stakeMin\|membershipContract?\|membershipFn?\|membershipNftId?\|proposalMetadata?\|proposalCreatorRestriction\|membershipPayloadFormat?\|projectUrl?` | Creates a new project. Membership payload defaults to `{nft}\|{caller}` and must include both placeholders. Proposal creator restriction `1` = members only, `0` = public. | ID of the new project (`msg:<id>`) |
| `project_join` | `projectId` | Joins a project using the caller’s first `transfer.allow` intent. Aborts if paused or the caller fails NFT membership checks. | `"joined"` |
| `project_leave` | `projectId` | Starts/finishes the leave cooldown. Blocks when payouts targeting the member are still active. | `"exit requested"` / `"exit finished"` |
| `project_funds` | `projectId\|toStakeFlag` | Adds funds either to the treasury (`false`) or increases the caller’s stake (`true`, stake systems only). | `"funds added"` |
| `project_transfer` | `projectId\|newOwner` | Owner-only direct transfer of ownership to an existing member. | `"ownership transferred"` |
| `project_pause` | `projectId\|true|false` | Owner-only immediate pause/unpause. Paused mode blocks new proposals/execution except meta proposals that only toggle pause. | `"paused"` / `"unpaused"` |
| `proposal_create` | `projectId\|name\|description\|duration\|options?\|forcePoll?\|payouts?\|meta?\|metadata?\|proposalUrl?` | Creates a proposal. `payouts` uses `member:amount;member:amount`. `meta` is a `key=value;key=value` string and can update project config (threshold, quorum, execution delay, pause toggle, owner, membership payload, etc.). Cost is debited automatically. | ID of the proposal |
| `proposals_vote` | `proposalId\|choices` | Casts or updates votes for a proposal. Weight comes from stake. Choices can be comma or semicolon separated indices. | `"voted"` |
| `proposal_tally` | `proposalId` | Closes voting after duration. Sets proposal to `passed`, `closed`, `failed`, or `cancelled`. | `"tallied"` |
| `proposal_execute` | `proposalId` | Executes passed proposals after the execution delay. Handles treasury payouts and meta updates. | `"executed"` |
| `proposal_cancel` | `proposalId` | Creator or owner can cancel an active proposal. Owner-initiated cancels refund the proposal cost to the creator if treasury funds exist. | `"cancelled"` |

**Meta actions accepted in proposal outcome (`meta` payload):**

- `update_threshold=<float>`  
- `update_quorum=<float>`  
- `update_proposalDuration=<hours>`  
- `update_executionDelay=<hours>`  
- `update_leaveCooldown=<hours>`  
- `update_proposalCost=<float>`  
- `update_membershipNFT=<nftId>`  
- `update_membershipNFTContract=<contractName>`  
- `update_membershipNFTContractFunction=<methodName>`  
- `update_membershipNFTPayload=<format>` (must contain `{nft}` and `{caller}`)  
- `update_proposalCreatorRestriction=<0|1>`  
- `update_url=<https://example.com>` (empty clears it)  
- `update_owner=<memberAccount>`  
- `toggle_pause=1`

Only `toggle_pause` proposals may be created and executed while the project is paused, ensuring the DAO can unfreeze itself even if the owner disappears.

---

## 5. Tips for Success

- Make your **proposal descriptions clear** so members know exactly what they are voting for.
- Always check the **voting deadline** before you submit your vote.
- If joining a stake-based project, your **initial stake matters** — it defines your voting power.
- Proposal costs go to the project treasury, so even failed proposals contribute to the community.

---

## 6. Safety

- All votes and results are public and stored on-chain.  
- The project owner can hand over control to another member via a special transfer function.  
- Only members can vote, and only according to the project’s rules.

---

## 7. Events

The contract logs concise events for indexing:

| Event | Description | Example |
|-------|-------------|---------|
| `dc` (`dc\|id:<project>\|by:<creator>`) | Project created (full snapshot including metadata + url) | `dc\|id:1\|by:hive:alice\|name:Demo\|description:test\|metadata:\|url:https://dao.example` |
| `mj` / `ml` (`mj\|id:<project>\|by:<member>`) | Member joined / left | `mj\|id:1\|by:hive:bob` |
| `af` / `rf` (`af\|id:<project>\|by:<member>\|am:<float>\|as:<asset>\|s:<bool>`) | Funds added/removed (stake or treasury) | `af\|id:1\|by:hive:bob\|am:1.000000\|as:hive\|s:true` |
| `pc` (`pc\|id:<proposal>\|by:<creator>`) | Proposal created (includes metadata + url snapshot) | `pc\|id:5\|by:hive:alice\|name:Idea\|description:something\|metadata:\|url:https://example` |
| `ps` (`ps\|id:<proposal>\|s:<state>`) | Proposal state changed (`active`, `passed`, `executed`, `failed`, `cancelled`) | `ps\|id:5\|s:passed` |
| `px` (`px\|pId:<project>\|prId:<proposal>\|ready:<unix>`) | Proposal becomes executable at timestamp | `px\|pId:1\|prId:5\|ready:1757020800` |
| `pr` (`pr\|pId:<project>\|prId:<proposal>\|r:<result>`) | Result note (“meta changed”, “funds transferred”) | `pr\|pId:1\|prId:5\|r:funds transferred` |
| `pm` (`pm\|pId:<project>\|prId:<proposal>\|f:<field>\|old:<val>\|new:<val>`) | Config/meta diffs per field (threshold, pause, owner, etc.) | `pm\|pId:1\|prId:6\|f:owner\|old:hive:alice\|new:hive:bob` |
| `v` (`v\|id:<proposal>\|by:<member>\|cs:<choices>\|w:<weight>`) | Vote casted/updated | `v\|id:5\|by:hive:alice\|cs:1\|w:1.000000` |

---

## 8. Example Flow (Alice, Bob, and Carol)

Below is a simple stake-based DAO walkthrough where Bob requests funds and Carol receives an update. All payloads assume `|` as separator.

### 8.1 Simplified Flow Diagram

```
Project created (Alice)
     |
Bob joins (stakes 1 HIVE)
     |
Bob creates payout proposal → Alice + Bob vote yes
     |
Proposal tallied → executed (Bob receives funds)
     |
Alice submits meta proposal to lower threshold → Carol votes
     |
Proposal executed (new threshold in effect)
```

### 8.2 Step-by-step payloads

1. **Alice creates project (stake-based, 1 HIVE minimum):**
   ```
   project_create
   dao|Stake DAO|1|50.001|50.001|24|4|24|1|1| | | | |1|{nft}|{caller}
   ```

2. **Bob joins (stakes 1 HIVE):**
   ```
   project_join
   1
   ```

3. **Bob proposes a payout (0.5 HIVE to himself):**
   ```
   proposal_create
   1|Writer Grant|Fund Bob for documentation|24||1|hive:bob:0.500|||
   ```

4. **Alice and Bob both vote yes (exceeding the 50.001% threshold):**
   ```
   proposals_vote
   <proposalId>|1   (Alice)

   proposals_vote
   <proposalId>|1   (Bob)
   ```

5. **Bob tallies after voting period, Alice executes:**
   ```
   proposal_tally
   <proposalId>

   proposal_execute
   <proposalId>
   ```

6. **Alice submits a meta proposal to lower the threshold to 40% (Alice and Carol vote yes):**
   ```
   proposal_create
   1|Tune Threshold|Lower approval bar|24||1||update_threshold=40|

   proposals_vote
   <proposalId>|1   (Alice)

   proposals_vote
   <proposalId>|1   (Carol)
   ```

7. **Carol tallies and Alice executes to apply the new threshold:**
   ```
   proposal_tally
   <proposalId>

   proposal_execute
   <proposalId>
   ```

Now the DAO has adjusted its governance parameters and paid out Bob’s request. Use [terminal.okinoko.io](https://terminal.okinoko.io) to submit these payloads without building raw strings manually.
