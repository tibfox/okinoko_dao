//go:build wasm

package dao

import "okinoko_dao/sdk"

type Address = sdk.Address
type Asset = sdk.Asset

func newAddress(s string) Address { return sdk.Address(s) }
func addressString(a Address) string {
	return a.String()
}

func newAsset(s string) Asset { return sdk.Asset(s) }
func assetString(a Asset) string {
	return a.String()
}
