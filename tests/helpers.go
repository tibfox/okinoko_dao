package tests

import (
	"encoding/json"
	"fmt"
)

func printResultObject(executionResult *string) {
	var executionResultData map[string]interface{}
	err := json.Unmarshal([]byte(*executionResult), &executionResultData)

	if err != nil {
		fmt.Println("failed to get result")
	} else {
		executionResultFriendlyData, err := json.Marshal(executionResultData)
		if err != nil {
			fmt.Println("failed to parse result")
		}
		fmt.Println(string(executionResultFriendlyData))
	}
}
