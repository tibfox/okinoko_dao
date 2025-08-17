package tests

import (
	"fmt"
	"okinoko_dao/contract"
	"testing"
)

func TestCreateProject(t *testing.T) {
	debug := true
	contract.InitState(debug)
	contract.InitSKMocks(debug)
	contract.InitENVMocks(debug)

	fmt.Println("All contract initialization done.")

	fmt.Println("#### PROJECTS")
	fmt.Println("CREATE")
	contractId := contract.CreateProject("testtitle", "testdesc", "testjso", "testcfg", 1, "testasset")
	fmt.Println(*contractId)
	fmt.Println("LOAD")
	fmt.Println(contract.GetProject(*contractId))
}
