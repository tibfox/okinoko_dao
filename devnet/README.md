# Okinoko DAO — multi-node devnet test

`okinoko_dao_devnet_test.go` deploys the DAO to a **real 5-node, Hive-anchored
devnet** and drives a full governance lifecycle, asserting **multi-node
consensus** on every money-moving effect. This is the validation a single-node
harness cannot provide: each effect is read back from two independent nodes and
must agree.

## Result (validated against `vsc-eco/go-vsc-node` main)

| Step | On-chain effect | Cross-node |
|---|---|---|
| DAO wasm deployed | contract active on-chain | ✓ |
| `contract_init` + `project_create` | owner staked 1.000 | ✓ |
| node2 joins | 100.000 → **99.000** | agree ✓ |
| node3 joins | 100.000 → **99.000** | agree ✓ |
| treasury funded | owner → **94.000** | agree ✓ |
| proposal created | node2 → **98.000** (1.000 cost) | agree ✓ |
| votes (2 of 3 = 66.7%) | quorum + threshold met | ✓ |
| **tally → execute** | grantee **100.000 → 102.000** | **agree ✓** |

Real deployment, real Hive block anchoring, real multi-node consensus, real
treasury funds moved by governance vote.

## Running it

The test uses the node repo's `devnet` package internals, so it must live inside
that package:

```bash
# 1. build the DAO wasm
cd okinoko_dao
GOTOOLCHAIN=go1.25.7 tinygo build -gc=custom -scheduler=none -panic=trap \
  -no-debug -target=wasm-unknown -o builds/devnet-dao.wasm ./contract

# 2. drop the test into the node repo and run it
cp devnet/okinoko_dao_devnet_test.go <go-vsc-node>/tests/devnet/
cd <go-vsc-node>
DAO_WASM=$(pwd)/../okinoko_dao/builds/devnet-dao.wasm \
  go test -run TestOkinokoDAODevnet -count=1 -timeout 150m -v ./tests/devnet/
```

Set `DEVNET_KEEP=1` to leave the containers up for inspection.

Runtime is ~1h50m: ~30–40 min bring-up (image build + HAF replay + genesis
election) plus a ~64 min wait for the DAO's 1-hour minimum voting window, which
elapses in real block time.

### Requirements
- Docker, and **~20–40 GB free disk** (HAF images + replay + the magi image build)
- **5 nodes** — the framework's minimum is 4, but contract deploy briefly stops
  one node while the storage-proof quorum (`MinSpSigners=3`) must still clear;
  4 leaves exactly 3 and flakes. 3 nodes is not supported.

## Gotchas worth knowing (each cost a debugging round)

### 1. RC budget is tiny — don't poll from one account
Devnet accounts have **`max_rcs = 10000`** and RC regenerates slowly. An early
version polled `tally`+`execute` every ~78s from a single account (~130 calls),
draining it to RC 1000. Every subsequent call from that account then failed.

**Symptom is nasty:** the tx shows `status: FAILED`, and GraphQL's
`ContractOutputResult` exposes only `{ok: false, ret: ""}` — **the abort reason
is not surfaced**. It looks identical to a contract bug.

Mitigations used here: modest `rc_limit` (5000), calls spread across accounts,
generous spacing, and the payout driven from a different account than the one
that created/voted.

**Relevant to app authors** (e.g. okinoko.io): space contract calls, spread them
across accounts, and don't rely on GraphQL to tell you *why* a call failed.

### 2. Diagnosing a silent failure
Since GraphQL hides abort reasons, the fastest triage is to **replay the exact
payload through the single-node harness** (`test/`), which surfaces the message
directly. That's how the contract was cleared here: the identical payloads that
`FAILED` on devnet succeeded instantly locally, proving it was an RC/driver
issue rather than contract logic.

### 3. Time-gated governance is real time
The DAO enforces `MinProposalDurationHours = 1` against block timestamps, and
devnet chain time tracks wall-clock (~3s blocks). A payout cycle therefore needs
a genuine ~1 hour wait — it cannot be short-circuited.

### 4. Deposits don't retry
`Devnet.Deposit` broadcasts once; a transient
"server closed connection before returning the first response byte" from the
drone endpoint will fail the run. The test wraps deposits in a retry.
