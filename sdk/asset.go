package sdk

type Asset string

const (
	AssetHive       Asset = "hive"
	AssetHiveCons   Asset = "hive_consensus"
	AssetHbd        Asset = "hbd"
	AssetHbdSavings Asset = "hbd_savings"
)

// String returns the raw ticker string for logging or host calls.
// Example payload: sdk.AssetHive.String()
func (a Asset) String() string {
	return string(a)
}
