//go:build !wasm

package dao

type Address string
type Asset string

func newAddress(s string) Address { return Address(s) }
func addressString(a Address) string {
	return string(a)
}

func newAsset(s string) Asset { return Asset(s) }
func assetString(a Asset) string {
	return string(a)
}
