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
			"votingSystem":     "democratic",
			"democraticAmount": 1,
			"threshold":        51,
			"quorum":           2,
			"proposalDuration": 10,
			"executionDelay":   10,
			"leaveCooldown":    10,
			"proposalCost":     1,
		},
		"meta": map[string]string{
			"a": "a value",
			"b": "b value",
		},
	}), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "1.000", "token": "hive"}}}, "hive:someone", true, uint(100_000_000))
}
