package contract_test

import (
	"fmt"
	"testing"
)

// admin tests
func TestCreateProjectDemo(t *testing.T) {
	ct := SetupContractTest()

	fmt.Printf("%d", ct.StateEngine.BlockHeight)
	// CallContract(t, ct, "project_create", PayloadToJSON(map[string]any{
	// 	"name": "my dao project",
	// 	"desc": "project description",
	// 	"config": map[string]any{
	// 		"votingSystem":     "democratic",
	// 		"democraticAmount": 1,
	// 		"threshold":        2,
	// 		"quorum":           2,
	// 		"proposalDuration": 10,
	// 		"executionDelay":   10,
	// 		"leaveCooldown":    10,
	// 		"proposalCost":     1,
	// 	},
	// 	"meta": map[string]string{
	// 		"a": "a value",
	// 		"b": "b value",
	// 	},
	// }), []contracts.Intent{{Type: "transfer.allow", Args: map[string]string{"limit": "1.123", "token": "hive"}}}, "hive:userA", true, uint(100_000_000))
}
