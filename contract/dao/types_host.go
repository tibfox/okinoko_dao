//go:build !wasm

package dao

type Address string
type Asset string

// newAddress wraps host-side strings so shared code can treat wasm + host equally.
func newAddress(s string) Address { return Address(s) }

// addressString unwraps a host Address into string.
func addressString(a Address) string {
	return string(a)
}

// newAsset parallels newAddress but for assets when running tests on host go.
func newAsset(s string) Asset { return Asset(s) }

// assetString unwraps the host Asset into string.
func assetString(a Asset) string {
	return string(a)
}
