package main

import "okinoko_dao/sdk"

// stateSetIfChanged avoids unnecessary writes so we dont thrash storage fees.
func stateSetIfChanged(key, value string) {
	if existing := sdk.StateGetObject(key); existing != nil && *existing == value {
		return
	}
	sdk.StateSetObject(key, value)
}
