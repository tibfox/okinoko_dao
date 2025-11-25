package contract_test

import (
	"testing"
	"vsc-node/modules/db/vsc/contracts"
)

// admin tests
func TestCreateProject(t *testing.T) {
	ct := SetupContractTest()

	CallContract(t, ct, "project_create", PayloadToJSON(map[string]any{
		"name": "my dao project",
		"desc": "project description",
		"config": map[string]any{
			"votingSystem":          "democratic",
			"threshold":             50.001,
			"quorum":                50.001,
			"proposalDurationHours": 10,
			"executionDelay":        10,
			"leaveCooldown":         10,
			"proposalCost":          1,
			"minStake":              1,
		},
		"meta": map[string]any{
			"a": "a value",
			"b": "b value",
			"c": 1,
		},
	}), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}}}, "hive:someone", true, uint(100_000_000))
	val := ct.StateGet(ContractID, "prj:0")
	t.Log(val)
}

func TestCreateProposal(t *testing.T) {
	ct := SetupContractTest()

	CallContract(t, ct, "project_create", PayloadToJSON(map[string]any{
		"name": "my dao project",
		"desc": "project description",
		"config": map[string]any{
			"votingSystem":          "democratic",
			"threshold":             50.001,
			"quorum":                50.001,
			"proposalDurationHours": 24,
			"executionDelay":        10,
			"leaveCooldown":         10,
			"proposalCost":          1,
			"minStake":              1,
		},
		"jsonMeta": map[string]any{
			"a": "a value",
			"b": "b value",
			"c": 1,
		},
	}), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}}}, "hive:someone", true, uint(100_000_000))

	CallContract(t, ct, "proposal_create", PayloadToJSON(map[string]any{
		"project_id": 0,
		"name":       "my proposal",
		"desc":       "proposal description",

		"payout": map[string]float64{
			"hive:someoneelse":  1,
			"hive:someoneelse2": 1,
		},

		"jsonMeta": map[string]any{
			"a": "a value",
			"b": "b value",
			"c": 1,
		},
	}), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}}}, "hive:someone", true, uint(100_000_000))

	val := ct.StateGet(ContractID, "prpsl:0")
	t.Log(val)
}
