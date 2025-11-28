package sdk

import "strings"

type Intent struct {
	Type string            `json:"type"`
	Args map[string]string `json:"args"`
}

type Sender struct {
	Address              Address   `json:"id"`
	RequiredAuths        []Address `json:"required_auths"`
	RequiredPostingAuths []Address `json:"required_posting_auths"`
}

type ContractCallOptions struct {
	Intents []Intent `json:"intents,omitempty"`
}

type AddressDomain string

const (
	AddressDomainUser     AddressDomain = "user"
	AddressDomainContract AddressDomain = "contract"
	AddressDomainSystem   AddressDomain = "system"
)

type AddressType string

const (
	AddressTypeEVM     AddressType = "evm"
	AddressTypeKey     AddressType = "key"
	AddressTypeHive    AddressType = "hive"
	AddressTypeSystem  AddressType = "system"
	AddressTypeBLS     AddressType = "bls"
	AddressTypeUnknown AddressType = "unknown"
)

type Address string

// String returns the literal representation (like hive:alice) of the address.
// Example payload: sdk.Address("hive:foo").String()
func (a Address) String() string {
	return string(a)
}

// Domain quickly checks the prefix to guess if we deal with user/contract/system domain.
// Example payload: sdk.Address("contract:okinoko").Domain()
func (a Address) Domain() AddressDomain {
	if strings.HasPrefix(a.String(), "system:") {
		return AddressDomainSystem
	}
	if strings.HasPrefix(a.String(), "contract:") {
		return AddressDomainContract
	}
	return AddressDomainUser
}

// Type inspects the DID prefix to categorize the address (evm, key, hive,...).
// Example payload: sdk.Address("did:pkh:eip155").Type()
func (a Address) Type() AddressType {
	if strings.HasPrefix(a.String(), "did:pkh:eip155") {
		return AddressTypeEVM
	} else if strings.HasPrefix(a.String(), "did:key:") {
		return AddressTypeKey
	} else if strings.HasPrefix(a.String(), "hive:") {
		return AddressTypeHive
	} else if strings.HasPrefix(a.String(), "system:") {
		return AddressTypeSystem
	} else {
		return AddressTypeUnknown
	}
	//TODO: Detect BLS address type, though it is not used or planned to be supported.
}

// IsValid returns false if the address type detection failed, used as a light sanity check.
// Example payload: sdk.Address("foo").IsValid()
func (a Address) IsValid() bool {
	if a.Type() == AddressTypeUnknown {
		return false
	}
	return true
}
