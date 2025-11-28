package main

import (
	"fmt"
	"okinoko_dao/contract/dao"
	"okinoko_dao/sdk"
	"strconv"
	"strings"
	"time"
)

// cachedEnv holds the environment for the current contract call.
var (
	cachedEnv       sdk.Env
	cachedEnvLoaded bool
	cachedTransfer  *TransferAllow
	cachedMembers   map[string]*dao.Member
)

const (
	kProjectMeta       byte = 0x01
	kProjectConfig     byte = 0x02
	kProjectFinance    byte = 0x03
	kProjectMember     byte = 0x04
	kProjectPayoutLock byte = 0x05
	kProposalMeta      byte = 0x10
	kProposalOption    byte = 0x11
	kVoteReceipt       byte = 0x20
)

// packU64LEInline sprinkles a uint64 into dst in little-endian order so our keys stay compact.
func packU64LEInline(x uint64, dst []byte) {
	dst[0] = byte(x)
	dst[1] = byte(x >> 8)
	dst[2] = byte(x >> 16)
	dst[3] = byte(x >> 24)
	dst[4] = byte(x >> 32)
	dst[5] = byte(x >> 40)
	dst[6] = byte(x >> 48)
	dst[7] = byte(x >> 56)
}

// packU32LEInline mirrors the 64-bit helper but for smaller option indexes.
func packU32LEInline(x uint32, dst []byte) {
	dst[0] = byte(x)
	dst[1] = byte(x >> 8)
	dst[2] = byte(x >> 16)
	dst[3] = byte(x >> 24)
}

// packU64LE appends the encoded number to dst and returns the new slice.
func packU64LE(x uint64, dst []byte) []byte {
	return append(dst,
		byte(x),
		byte(x>>8),
		byte(x>>16),
		byte(x>>24),
		byte(x>>32),
		byte(x>>40),
		byte(x>>48),
		byte(x>>56),
	)
}

// currentEnv caches the env per tx.id so we dont poke the host api every few lines.
func currentEnv() *sdk.Env {
	var currentTx string
	if txPtr := sdk.GetEnvKey("tx.id"); txPtr != nil {
		currentTx = *txPtr
	}
	if !cachedEnvLoaded || cachedEnv.TxId != currentTx {
		cachedEnv = sdk.GetEnv()
		cachedEnvLoaded = true
		cachedTransfer = nil
		cachedMembers = map[string]*dao.Member{}
	}
	return &cachedEnv
}

// currentIntents is just a tiny helper to access intents already pulled above.
func currentIntents() []sdk.Intent {
	return currentEnv().Intents
}

////////////////////////////////////////////////////////////////////////////////
// Helpers: keys, guids, time
////////////////////////////////////////////////////////////////////////////////

// TransferAllow represents arguments extracted from a transfer.allow intent.
// It specifies the allowed transfer amount (`Limit`) and the asset (`Token`).
type TransferAllow struct {
	Limit float64
	Token sdk.Asset
}

// validAssets lists the supported asset types.
var validAssets = []string{sdk.AssetHbd.String(), sdk.AssetHive.String()}

// isValidAsset checks if a given token string is one of the supported assets.
func isValidAsset(token string) bool {
	for _, a := range validAssets {
		if token == a {
			return true
		}
	}
	return false
}

// getFirstTransferAllow scans the provided intents and returns the first valid
// transfer.allow intent as a TransferAllow object. Returns nil if none found.
func getFirstTransferAllow() *TransferAllow {
	if cachedTransfer != nil {
		return cachedTransfer
	}
	for _, intent := range currentIntents() {
		if intent.Type == "transfer.allow" {
			token := intent.Args["token"]
			if !isValidAsset(token) {
				sdk.Abort("invalid intent asset")
			}
			limitStr := intent.Args["limit"]
			limit, err := strconv.ParseFloat(limitStr, 32)
			if err != nil {
				sdk.Abort("invalid intent limit")
			}
			ta := &TransferAllow{
				Limit: limit,
				Token: sdk.Asset(token),
			}
			cachedTransfer = ta
			return ta
		}
	}
	return nil
}

// getSenderAddress returns the address of the current transaction sender.
func getSenderAddress() sdk.Address {
	return currentEnv().Sender.Address
}

// projectKey builds a storage key string for a project by ID.
func projectKey(id uint64) string {
	var buf [9]byte
	buf[0] = kProjectMeta
	packU64LEInline(id, buf[1:])
	return string(buf[:])
}

// projectConfigKey uses prefix 0x02 so configs sit next to meta but not collide.
func projectConfigKey(id uint64) string {
	var buf [9]byte
	buf[0] = kProjectConfig
	packU64LEInline(id, buf[1:])
	return string(buf[:])
}

// projectFinanceKey sits in prefix 0x03 for quick aggregated lookups.
func projectFinanceKey(id uint64) string {
	var buf [9]byte
	buf[0] = kProjectFinance
	packU64LEInline(id, buf[1:])
	return string(buf[:])
}

// memberKey mixes project id plus address bytes to avoid nested maps in host storage.
func memberKey(projectID uint64, addr dao.Address) string {
	addrStr := dao.AddressToString(addr)
	buf := make([]byte, 0, 1+8+len(addrStr))
	buf = append(buf, kProjectMember)
	buf = packU64LE(projectID, buf)
	buf = append(buf, addrStr...)
	return string(buf)
}

// payoutLockKey counts pending payouts for a member so we can block exits safely.
func payoutLockKey(projectID uint64, addr dao.Address) string {
	addrStr := dao.AddressToString(addr)
	buf := make([]byte, 0, 1+8+len(addrStr))
	buf = append(buf, kProjectPayoutLock)
	buf = packU64LE(projectID, buf)
	buf = append(buf, addrStr...)
	return string(buf)
}

// proposalKey builds a storage key string for a proposal by ID.
// proposalKey encodes id under 0x10 prefix keeping metadata lumps contiguous.
func proposalKey(id uint64) string {
	var buf [9]byte
	buf[0] = kProposalMeta
	packU64LEInline(id, buf[1:])
	return string(buf[:])
}

// proposalOptionKey stores options sequentially under 0x11 prefix.
func proposalOptionKey(id uint64, idx uint32) string {
	var buf [13]byte
	buf[0] = kProposalOption
	packU64LEInline(id, buf[1:])
	packU32LEInline(idx, buf[9:])
	return string(buf[:])
}

// nowUnix returns the current Unix timestamp.
// It prefers the chain's block timestamp from the environment if available.
func nowUnix() int64 {
	if ts := currentEnv().Timestamp; ts != "" {
		if v, ok := parseTimestamp(ts); ok {
			return v
		}
	}
	if tsPtr := sdk.GetEnvKey("block.timestamp"); tsPtr != nil && *tsPtr != "" {
		if v, ok := parseTimestamp(*tsPtr); ok {
			return v
		}
	}
	return time.Now().Unix()
}

// parseTimestamp accepts unix seconds or iso-ish strings since the env flips formats sometimes.
func parseTimestamp(val string) (int64, bool) {
	if v, err := strconv.ParseInt(val, 10, 64); err == nil {
		return v, true
	}
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t.Unix(), true
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", val, time.UTC); err == nil {
		return t.Unix(), true
	}
	return 0, false
}

///////////////////////////////////////////////////
// Payload decoding helpers
///////////////////////////////////////////////////

// decodeCreateProjectArgs unpacks the pipe-delimited payload used for project_create calls.
func decodeCreateProjectArgs(payload *string) *dao.CreateProjectArgs {
	raw := unwrapPayload(payload, "project payload missing")
	parts := strings.Split(raw, "|")
	get := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}

	args := &dao.CreateProjectArgs{
		Name:        strings.TrimSpace(get(0)),
		Description: strings.TrimSpace(get(1)),
		Metadata:    normalizeOptionalField(get(13)),
	}
	cfg := dao.ProjectConfig{
		VotingSystem: parseVotingSystem(get(2)),
	}
	cfg.ThresholdPercent = parseFloatField(get(3), "threshold")
	cfg.QuorumPercent = parseFloatField(get(4), "quorum")
	cfg.ProposalDurationHours = parseUintField(get(5), "proposal duration")
	execDelayField := strings.TrimSpace(get(6))
	if execDelayField == "" {
		cfg.ExecutionDelayHours = FallbackExecutionDelayHours
	} else {
		cfg.ExecutionDelayHours = parseUintField(execDelayField, "execution delay")
	}
	cfg.LeaveCooldownHours = parseUintField(get(7), "leave cooldown")
	cfg.ProposalCost = parseFloatField(get(8), "proposal cost")
	cfg.StakeMinAmt = parseFloatField(get(9), "min stake")
	if v := strings.TrimSpace(get(10)); v != "" {
		cfg.MembershipNFTContract = strptr(v)
	}
	if v := strings.TrimSpace(get(11)); v != "" {
		cfg.MembershipNFTContractFunction = strptr(v)
	}
	if v := strings.TrimSpace(get(12)); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			sdk.Abort("invalid membership nft id")
		}
		cfg.MembershipNFT = &parsed
	}
	cfg.ProposalsMembersOnly = parseCreatorRestrictionField(get(14))
	if v := strings.TrimSpace(get(15)); v != "" {
		cfg.MembershipNftPayloadFormat = v
	}
	normalizeProjectConfig(&cfg)
	args.ProjectConfig = cfg
	return args
}

// decodeCreateProposalArgs splits the string payload and normalizes optional bits like payouts.
func decodeCreateProposalArgs(payload *string) *dao.CreateProposalArgs {
	raw := unwrapPayload(payload, "proposal payload missing")
	parts := strings.Split(raw, "|")
	get := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	projectID := parseUintField(get(0), "project id")
	duration := parseUintField(get(3), "proposal duration")
	options := parseOptionsField(get(4))
	forcePoll := parseBoolField(get(5))
	payouts := parsePayoutField(get(6))
	metaOutcome := parseMetadataField(get(7))
	metadata := normalizeOptionalField(get(8))

	var outcome *dao.ProposalOutcome
	if len(payouts) > 0 || len(metaOutcome) > 0 {
		outcome = &dao.ProposalOutcome{
			Meta:   metaOutcome,
			Payout: payouts,
		}
	}

	return &dao.CreateProposalArgs{
		ProjectID:        projectID,
		Name:             strings.TrimSpace(get(1)),
		Description:      strings.TrimSpace(get(2)),
		OptionsList:      options,
		ProposalOutcome:  outcome,
		ProposalDuration: duration,
		Metadata:         metadata,
		ForcePoll:        forcePoll,
	}
}

// decodeVoteProposalArgs expects `proposalId|choices` and converts indexes into uint slice.
func decodeVoteProposalArgs(payload *string) *dao.VoteProposalArgs {
	raw := unwrapPayload(payload, "vote payload missing")
	parts := strings.Split(raw, "|")
	if len(parts) < 2 {
		sdk.Abort("vote payload requires proposalId|choices")
	}
	proposalID := parseUintField(parts[0], "proposal id")
	choices := parseChoiceField(parts[1])
	return &dao.VoteProposalArgs{
		ProposalID: proposalID,
		Choices:    choices,
	}
}

// decodeAddFundsArgs extracts project id plus staking flag from the user payload.
func decodeAddFundsArgs(payload *string) *dao.AddFundsArgs {
	raw := unwrapPayload(payload, "add funds payload missing")
	parts := strings.Split(raw, "|")
	if len(parts) < 2 {
		sdk.Abort("add funds payload requires projectId|toStake")
	}
	projectID := parseUintField(parts[0], "project id")
	toStake := parseBoolField(parts[1])
	return &dao.AddFundsArgs{
		ProjectID: projectID,
		ToStake:   toStake,
	}
}

// unwrapPayload trims quotes and whitespace, aborting if the payload is empty.
func unwrapPayload(payload *string, errMsg string) string {
	if payload == nil {
		sdk.Abort(errMsg)
	}
	raw := strings.TrimSpace(*payload)
	if raw == "" {
		sdk.Abort(errMsg)
	}
	if len(raw) >= 2 {
		first := raw[0]
		last := raw[len(raw)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			if unquoted, err := strconv.Unquote(raw); err == nil {
				return unquoted
			}
			raw = strings.TrimSpace(raw[1 : len(raw)-1])
			if raw == "" {
				sdk.Abort(errMsg)
			}
		}
	}
	return raw
}

// saveMember writes both storage and cache copy so repeated reads stay cheap.
func saveMember(projectID uint64, member *dao.Member) {
	key := memberKey(projectID, member.Address)
	data := dao.EncodeMember(member)
	sdk.StateSetObject(key, string(data))
	if cachedMembers != nil {
		cp := *member
		cachedMembers[key] = &cp
	}
}

// loadMember tries cache first and decodes wasm bytes when needed.
func loadMember(projectID uint64, addr dao.Address) (*dao.Member, bool) {
	key := memberKey(projectID, addr)
	if cachedMembers != nil {
		if cached, ok := cachedMembers[key]; ok {
			cp := *cached
			return &cp, true
		}
	}
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return nil, false
	}
	member, err := dao.DecodeMember([]byte(*ptr))
	if err != nil {
		sdk.Abort("failed to decode member")
	}
	if cachedMembers != nil {
		cp := *member
		cachedMembers[key] = &cp
	}
	return member, true
}

// deleteMember evicts member state and removes cached clone to avoid stale reads.
func deleteMember(projectID uint64, addr dao.Address) {
	key := memberKey(projectID, addr)
	sdk.StateDeleteObject(key)
	if cachedMembers != nil {
		delete(cachedMembers, key)
	}
}

// getPayoutLockCount reads how many pending payouts block withdrawals for this addr.
func getPayoutLockCount(projectID uint64, addr dao.Address) uint64 {
	key := payoutLockKey(projectID, addr)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return 0
	}
	val, err := strconv.ParseUint(*ptr, 10, 64)
	if err != nil {
		sdk.Abort("invalid payout lock value")
	}
	return val
}

// incrementPayoutLock bumps the counter when a new payout is tied to the member.
func incrementPayoutLock(projectID uint64, addr dao.Address) {
	key := payoutLockKey(projectID, addr)
	count := getPayoutLockCount(projectID, addr) + 1
	sdk.StateSetObject(key, strconv.FormatUint(count, 10))
}

// decrementPayoutLock lowers the counter and deletes key when it reaches zero.
func decrementPayoutLock(projectID uint64, addr dao.Address) {
	key := payoutLockKey(projectID, addr)
	count := getPayoutLockCount(projectID, addr)
	if count == 0 {
		return
	}
	count--
	if count == 0 {
		sdk.StateDeleteObject(key)
	} else {
		sdk.StateSetObject(key, strconv.FormatUint(count, 10))
	}
}

// incrementPayoutLocks loops the payout map so each beneficiary gets a lock entry.
func incrementPayoutLocks(projectID uint64, payout map[dao.Address]dao.Amount) {
	if payout == nil {
		return
	}
	for addr := range payout {
		incrementPayoutLock(projectID, addr)
	}
}

// decrementPayoutLocks removes all locks once a proposal outcome resolves.
func decrementPayoutLocks(projectID uint64, payout map[dao.Address]dao.Amount) {
	if payout == nil {
		return
	}
	for addr := range payout {
		decrementPayoutLock(projectID, addr)
	}
}

// saveProposalOption stores each option separately to avoid rewriting the whole proposal blob.
func saveProposalOption(proposalID uint64, idx uint32, opt *dao.ProposalOption) {
	key := proposalOptionKey(proposalID, idx)
	data := dao.EncodeProposalOption(opt)
	sdk.StateSetObject(key, string(data))
}

// loadProposalOption decodes a single option and aborts loudly when missing.
func loadProposalOption(proposalID uint64, idx uint32) *dao.ProposalOption {
	key := proposalOptionKey(proposalID, idx)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("proposal option not found")
	}
	opt, err := dao.DecodeProposalOption([]byte(*ptr))
	if err != nil {
		sdk.Abort("failed to decode proposal option")
	}
	return opt
}

// loadProposalOptions iterates indexes and returns a flat slice for tallying.
func loadProposalOptions(proposalID uint64, count uint32) []dao.ProposalOption {
	opts := make([]dao.ProposalOption, count)
	for i := uint32(0); i < count; i++ {
		opt := loadProposalOption(proposalID, i)
		opts[i] = *opt
	}
	return opts
}

// stateSetIfChanged avoids unnecessary writes so we dont thrash storage fees.
func stateSetIfChanged(key, value string) {
	if existing := sdk.StateGetObject(key); existing != nil && *existing == value {
		return
	}
	sdk.StateSetObject(key, value)
}

// parseVotingSystem accepts friendly strings or digits, defaulting to stake voting for safety.
func parseVotingSystem(val string) dao.VotingSystem {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "0":
		return dao.VotingSystemDemocratic
	case "1":
		return dao.VotingSystemStake
	default:
		return dao.VotingSystemStake
	}
}

// parseFloatField trims the input and aborts with a friendly field name on errors.
func parseFloatField(val string, field string) float64 {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		sdk.Abort(fmt.Sprintf("invalid %s", field))
	}
	return f
}

// parseUintField is the uint variant used for durations and ids.
func parseUintField(val string, field string) uint64 {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0
	}
	n, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		sdk.Abort(fmt.Sprintf("invalid %s", field))
	}
	return n
}

// parseBoolField accepts a couple of truthy keywords, defaulting to false for unknown text.
func parseBoolField(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "y", "poll":
		return true
	default:
		return false
	}
}

// parseOptionsField splits the list by ';' and trims each option.
func parseOptionsField(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" {
		return []string{}
	}
	raw := strings.Split(val, ";")
	opts := make([]string, 0, len(raw))
	for _, opt := range raw {
		opt = strings.TrimSpace(opt)
		if opt != "" {
			opts = append(opts, opt)
		}
	}
	return opts
}

// parseChoiceField allows comma or semicolon separators and returns clean indexes.
func parseChoiceField(val string) []uint {
	val = strings.TrimSpace(val)
	if val == "" {
		return []uint{}
	}
	raw := strings.FieldsFunc(val, func(r rune) bool {
		return r == ',' || r == ';'
	})
	choices := make([]uint, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			sdk.Abort("invalid choice index")
		}
		choices = append(choices, uint(idx))
	}
	return choices
}

// parseMetadataField lets payload authors include semi-colon separated key=value pairs.
func parseMetadataField(val string) map[string]string {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
		val = strings.TrimSpace(val[1 : len(val)-1])
	}
	if val == "" {
		return nil
	}
	meta := map[string]string{}
	pairs := strings.Split(val, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		split := strings.SplitN(pair, "=", 2)
		if len(split) != 2 {
			sdk.Abort("invalid metadata entry (use key=value)")
		}
		meta[strings.TrimSpace(split[0])] = strings.TrimSpace(split[1])
	}
	return meta
}

// normalizeProjectConfig ensures configs always have sane fallbacks before persisting.
func normalizeProjectConfig(cfg *dao.ProjectConfig) {
	if cfg.VotingSystem == dao.VotingSystemUnspecified {
		cfg.VotingSystem = dao.VotingSystemStake
	}
	if cfg.ThresholdPercent <= 0 {
		cfg.ThresholdPercent = FallbackThresholdPercent
	}
	if cfg.QuorumPercent <= 0 {
		cfg.QuorumPercent = FallbackQuorumPercent
	}
	if cfg.ProposalDurationHours <= 0 {
		cfg.ProposalDurationHours = FallbackProposalDurationHours
	}
	if cfg.LeaveCooldownHours <= 0 {
		cfg.LeaveCooldownHours = FallbackLeaveCooldownHours
	}
	if cfg.ProposalCost <= 0 {
		cfg.ProposalCost = FallbackProposalCost
	}
	cfg.MembershipNftPayloadFormat = normalizeMembershipPayloadFormat(cfg.MembershipNftPayloadFormat)
}

// normalizeOptionalField trims funky placeholders like  so metadata stays clean.
func normalizeOptionalField(val string) string {
	val = strings.TrimSpace(val)
	if val == "" || val == "\"\"" || val == "''" {
		return ""
	}
	return val
}

// parsePayoutField parses addr:amount entries and converts floats to Amount scale.
func parsePayoutField(val string) map[dao.Address]dao.Amount {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	payouts := map[dao.Address]dao.Amount{}
	entries := strings.Split(val, ";")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		idx := strings.LastIndex(entry, ":")
		if idx <= 0 || idx == len(entry)-1 {
			sdk.Abort("invalid payout entry (addr:amount)")
		}
		addr := dao.AddressFromString(strings.TrimSpace(entry[:idx]))
		amountStr := strings.TrimSpace(entry[idx+1:])
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			sdk.Abort("invalid payout amount")
		}
		payouts[addr] = dao.FloatToAmount(amount)
	}
	return payouts
}

// parseCreatorRestrictionField lets payloads toggle between members-only and public creators.
func parseCreatorRestrictionField(val string) bool {
	val = strings.TrimSpace(strings.ToLower(val))
	if val == "" {
		return FallbackProposalCreatorsMembersOnly
	}
	switch val {
	case "1", "true", "yes", "members":
		return true
	case "0", "false", "no", "public", "any":
		return false
	default:
		sdk.Abort("invalid proposal creator restriction")
	}
	return true
}

// normalizeMembershipPayloadFormat verifies the placeholders exist (nft + caller) else falls back.
func normalizeMembershipPayloadFormat(format string) string {
	format = strings.TrimSpace(format)
	if format == "" {
		return FallbackMembershipPayloadFormat
	}
	if !strings.Contains(format, "{nft}") || !strings.Contains(format, "{caller}") {
		return FallbackMembershipPayloadFormat
	}
	return format
}

// formatMembershipPayload simply replaces {nft}/{caller} tokens right before making the contract call.
func formatMembershipPayload(format string, nftID string, caller string) string {
	if format == "" {
		format = FallbackMembershipPayloadFormat
	}
	result := strings.ReplaceAll(format, "{nft}", nftID)
	result = strings.ReplaceAll(result, "{caller}", caller)
	return result
}

// strptr is a tiny helper so we can take a literal string and hand a pointer to sdk calls quickly.
func strptr(s string) *string { return &s }

///////////////////////////////////////////////////
// Counter helpers
///////////////////////////////////////////////////

// Index key prefixes for counting entities.
const (
	// VotesCount holds an integer counter for votes (used for generating IDs).
	VotesCount = "count:v"

	// ProposalsCount holds an integer counter for proposals (used for generating IDs).
	ProposalsCount = "count:props"

	// ProjectsCount holds an integer counter for projects (used for generating IDs).
	ProjectsCount = "count:proj"
)

// getCount reads the string counter under the key and defaults to zero, nothing magical here.
func getCount(key string) uint64 {
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return 0
	}
	n, _ := strconv.ParseUint(*ptr, 10, 64)
	return n
}

// setCount stores uint64 counters back as decimal strings for the host kv.
func setCount(key string, n uint64) {
	sdk.StateSetObject(key, strconv.FormatUint(n, 10))
}

// StringToUInt64 converts optional string inputs (like payloads) into ids while screaming on bad data.
// Example payload: StringToUInt64(strptr("42"))
func StringToUInt64(ptr *string) uint64 {
	if ptr == nil {
		sdk.Abort("input is empty")
	}
	val, err := strconv.ParseUint(*ptr, 10, 64) // base 10, 64-bit
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to parse '%s' to uint64: %w", *ptr, err))
	}
	return val
}

// UInt64ToString turns an id back into decimal text for logs or env payload building.
// Example payload: UInt64ToString(9001)
func UInt64ToString(val uint64) string {
	return strconv.FormatUint(val, 10)
}

// UIntSliceToString helps event logging since we encode []uint choices as 1,2,5 etc.
// Example payload: UIntSliceToString([]uint{0,2,3})

func UIntSliceToString(nums []uint) string {
	strNums := make([]string, len(nums))
	for i, n := range nums {
		strNums[i] = strconv.FormatUint(uint64(n), 10)
	}
	return strings.Join(strNums, ",")
}
// allowsPauseMeta checks whether the meta payload only toggles pause state.
func allowsPauseMeta(meta map[string]string) bool {
	if meta == nil {
		return false
	}
	if len(meta) == 1 {
		if _, ok := meta["toggle_pause"]; ok {
			return true
		}
	}
	return false
}

// proposalAllowsExecutionWhilePaused reuses the meta helper so pause votes can execute safely.
func proposalAllowsExecutionWhilePaused(prpsl *dao.Proposal) bool {
	if prpsl == nil || prpsl.Outcome == nil || prpsl.Outcome.Meta == nil {
		return false
	}
	return allowsPauseMeta(prpsl.Outcome.Meta)
}
