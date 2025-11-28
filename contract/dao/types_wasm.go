//go:build wasm

package dao

import "okinoko_dao/sdk"

type Address = sdk.Address
type Asset = sdk.Asset

// newAddress adapts go strings into sdk.Address when compiled for wasm.
func newAddress(s string) Address { return sdk.Address(s) }

// addressString calls the sdk helper so we respect address.String() formatting.
func addressString(a Address) string {
	return a.String()
}

// newAsset mirrors newAddress for sdk.Asset wrappers.
func newAsset(s string) Asset { return sdk.Asset(s) }

// assetString goes through Asset.String() to keep case consistent.
func assetString(a Asset) string {
	return a.String()
}
