package devnet

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestOkinokoDAODevnet deploys the Okinoko DAO to a real 5-node, Hive-anchored
// devnet and drives a complete governance lifecycle end-to-end:
//
//	deploy -> contract_init -> project_create (democratic) -> members join
//	(stake draws) -> fund treasury -> payout proposal -> votes -> tally ->
//	execute -> the grantee is paid from the treasury.
//
// Every money-moving effect is read back from TWO independent nodes and must
// agree — that multi-node consensus is the thing a single-node harness cannot
// prove. This test was validated green against vsc-eco/go-vsc-node main.
//
// HOW TO RUN (this file must live inside the node repo's devnet package):
//
//	cp devnet/okinoko_dao_devnet_test.go <go-vsc-node>/tests/devnet/
//	cd <go-vsc-node>
//	DAO_WASM=/abs/path/to/okinoko_dao/builds/devnet-dao.wasm \
//	  go test -run TestOkinokoDAODevnet -count=1 -timeout 150m -v ./tests/devnet/
//
// Build the wasm first:
//
//	GOTOOLCHAIN=go1.25.7 tinygo build -gc=custom -scheduler=none -panic=trap \
//	  -no-debug -target=wasm-unknown -o builds/devnet-dao.wasm ./contract
//
// IMPORTANT — RC BUDGET (learned the hard way, see README.md):
// devnet accounts have max_rcs = 10000 and RC regenerates slowly. Polling
// contract calls in a tight loop from ONE account drains it; every subsequent
// call then fails silently (tx status FAILED, and GraphQL's
// ContractOutputResult exposes only {ok:false, ret:""} — no abort reason).
// Hence: modest rc_limits, calls spread across accounts, generous spacing.
func TestOkinokoDAODevnet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DAO devnet test in short mode")
	}
	requireDocker(t)

	daoWasm := os.Getenv("DAO_WASM")
	if daoWasm == "" {
		daoWasm = "/home/dockeruser/magi/testnet/final-dao/builds/devnet-dao.wasm"
	}
	if _, err := os.Stat(daoWasm); err != nil {
		t.Fatalf("DAO wasm not found at %s: %v", daoWasm, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	cfg := DefaultConfig()
	// 5 nodes: contract deploy briefly stops one, and the storage-proof quorum
	// (MinSpSigners=3) must still clear with margin. 4 leaves exactly 3 and flakes.
	cfg.Nodes = 5
	cfg.GenesisNode = 5
	// Remap host ports off any live testnet stack on the same box.
	cfg.GQLBasePort = 28080
	cfg.P2PBasePort = 21720
	cfg.MongoPort = 28057
	cfg.HivePort = 28091
	cfg.DronePort = 29000
	cfg.BitcoindRPCPort = 28543
	cfg.DashdRPCPort = 29898
	if os.Getenv("DEVNET_KEEP") != "" {
		cfg.KeepRunning = true
	}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("creating devnet: %v", err)
	}
	t.Cleanup(func() { d.Stop() })
	if err := d.Start(ctx); err != nil {
		t.Fatalf("starting devnet: %v", err)
	}

	const (
		deployNode = 1
		qA, qB     = 2, 3  // independent query nodes for consensus checks
		rcModest   = 5000  // well under max_rcs (10000)
	)

	acct := func(n int) string { return fmt.Sprintf("hive:%s%d", cfg.WitnessPrefix, n) }
	transferAllow := func(limit string) []map[string]interface{} {
		return []map[string]interface{}{{"type": "transfer.allow", "args": map[string]interface{}{"token": "hive", "limit": limit}}}
	}
	bal := func(node int, a string) int64 {
		b, err := d.GetAccountBalance(ctx, node, a)
		if err != nil || b == nil {
			return -1
		}
		return b.Hive
	}
	waitBal := func(a string, pred func(int64) bool, what string, timeout time.Duration) int64 {
		t.Helper()
		dl := time.Now().Add(timeout)
		for {
			if v := bal(qA, a); v >= 0 && pred(v) {
				return v
			}
			if time.Now().After(dl) {
				t.Fatalf("timeout waiting for %s (last=%d)", what, bal(qA, a))
			}
			time.Sleep(5 * time.Second)
		}
	}
	consensus := func(a, what string) int64 {
		t.Helper()
		for i := 0; i < 40; i++ {
			if va, vb := bal(qA, a), bal(qB, a); va >= 0 && va == vb {
				t.Logf("consensus OK: %s hive=%d on nodes %d & %d (%s)", a, va, qA, qB, what)
				return va
			}
			time.Sleep(3 * time.Second)
		}
		t.Fatalf("CONSENSUS MISMATCH %s (%s)", a, what)
		return 0
	}
	var contractID string
	call := func(node int, action, payload string, intents []map[string]interface{}) {
		t.Helper()
		if _, err := d.CallContractWithIntents(ctx, node, contractID, action, payload, intents, rcModest); err != nil {
			t.Fatalf("call %s node %d: %v", action, node, err)
		}
		time.Sleep(10 * time.Second) // ordering buffer: VSC ingests L1 in order
	}
	depositRetry := func(node int, amount string) { // framework Deposit does not retry
		t.Helper()
		var last error
		for i := 0; i < 6; i++ {
			if _, err := d.Deposit(ctx, node, amount, "hive"); err == nil {
				return
			} else {
				last = err
				time.Sleep(4 * time.Second)
			}
		}
		t.Fatalf("deposit node %d: %v", node, last)
	}

	// ───────── deploy ─────────
	t.Log("deploying the Okinoko DAO...")
	contractID, err = d.DeployContract(ctx, ContractDeployOpts{WasmPath: daoWasm, Name: "okinoko-dao", DeployerNode: deployNode})
	if err != nil {
		t.Fatalf("deploy DAO: %v", err)
	}
	dl := time.Now().Add(5 * time.Minute)
	for {
		c, err := d.ActiveContract(ctx, qA, contractID)
		if err == nil && c != nil {
			t.Logf("DAO active: id=%s code=%s", contractID, c.Code)
			break
		}
		if time.Now().After(dl) {
			t.Fatalf("DAO never became active: %v", err)
		}
		time.Sleep(3 * time.Second)
	}

	// ───────── fund L2 balances ─────────
	for n := 1; n <= 4; n++ {
		depositRetry(n, "100.000")
	}
	for _, n := range []int{1, 2, 3} {
		waitBal(acct(n), func(v int64) bool { return v >= 100000 }, fmt.Sprintf("node%d deposit", n), 4*time.Minute)
	}
	t.Log("L2 deposits credited ✓")

	// ───────── init + democratic project ─────────
	call(1, "contract_init", "public", nil)
	projFields := "grantdao|community grants|0|50.001|50.001|1|0|10|1|1|||||1|||"
	call(1, "project_create", projFields, transferAllow("1.000"))

	// ───────── members join: each stake draw debits 1.000, agreed across nodes ─────────
	b2, b3 := bal(qA, acct(2)), bal(qA, acct(3))
	call(2, "project_join", "0", transferAllow("1.000"))
	call(3, "project_join", "0", transferAllow("1.000"))
	waitBal(acct(2), func(v int64) bool { return v <= b2-1000 }, "node2 stake draw", 3*time.Minute)
	waitBal(acct(3), func(v int64) bool { return v <= b3-1000 }, "node3 stake draw", 3*time.Minute)
	if m := consensus(acct(2), "node2 after join"); m != b2-1000 {
		t.Errorf("node2 draw: before=%d after=%d want -1000", b2, m)
	}
	if m := consensus(acct(3), "node3 after join"); m != b3-1000 {
		t.Errorf("node3 draw: before=%d after=%d want -1000", b3, m)
	}
	t.Log("member stake draws executed + agreed across nodes ✓")

	// ───────── fund the treasury ─────────
	b1 := bal(qA, acct(1))
	call(1, "project_funds", "0|false", transferAllow("5.000"))
	waitBal(acct(1), func(v int64) bool { return v <= b1-5000 }, "treasury funding", 3*time.Minute)
	consensus(acct(1), "owner after treasury funding")
	t.Log("treasury funded (owner -5.000) + agreed across nodes ✓")

	// ───────── payout proposal from node2 (NOT node1 — keep RC headroom) ─────────
	p2 := bal(qA, acct(2))
	propFields := fmt.Sprintf("0|writer grant|fund node4|1|||%s:2.000:hive||", acct(4))
	call(2, "proposal_create", propFields, transferAllow("1.000"))
	created := time.Now()
	waitBal(acct(2), func(v int64) bool { return v <= p2-1000 }, "proposal cost draw", 4*time.Minute)
	consensus(acct(2), "node2 after proposal cost")
	t.Log("payout proposal created (cost drawn + agreed) ✓")

	// votes: node2 + node3 = 2 of 3 members = 66.7% (> 50.001% threshold, meets 2-voter quorum)
	call(2, "proposals_vote", "0|1", nil)
	call(3, "proposals_vote", "0|1", nil)
	t.Log("votes submitted by node2 + node3 ✓")

	// ───────── wait out the 1h voting window (chain time tracks wall time) ─────────
	if wait := 64*time.Minute - time.Since(created); wait > 0 {
		t.Logf("waiting %s for the voting deadline...", wait.Truncate(time.Second))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			t.Fatal("ctx expired waiting for deadline")
		}
	}

	// ───────── tally + execute from node4 (highest RC), spaced ─────────
	g0 := bal(qA, acct(4))
	paid := false
	for attempt := 1; attempt <= 12 && !paid; attempt++ {
		t.Logf("tally+execute attempt %d...", attempt)
		d.CallContractWithIntents(ctx, 4, contractID, "proposal_tally", "0", nil, rcModest)
		time.Sleep(20 * time.Second)
		d.CallContractWithIntents(ctx, 4, contractID, "proposal_execute", "0", nil, rcModest)
		time.Sleep(25 * time.Second)
		if bal(qA, acct(4)) >= g0+2000 {
			paid = true
			break
		}
		time.Sleep(2 * time.Minute) // let RC regenerate
	}
	if !paid {
		t.Fatalf("payout not executed (grantee still %d)", bal(qA, acct(4)))
	}
	if after := consensus(acct(4), "grantee after payout"); after != g0+2000 {
		t.Errorf("payout wrong: before=%d after=%d want +2000", g0, after)
	}
	t.Log("PAYOUT end-to-end ✓✓ node4 received 2.000 HIVE from the DAO treasury, agreed across nodes")
}
