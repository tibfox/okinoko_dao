package main

import (
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
	"strings"
)

// decodeCreateProjectArgs unpacks the pipe-delimited payload used for project_create calls.
func decodeCreateProjectArgs(payload *string) *CreateProjectArgs {
	raw := unwrapPayload(payload, "project payload missing")
	parts := strings.Split(raw, "|")
	get := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}

	args := &CreateProjectArgs{
		Name:        strings.TrimSpace(get(0)),
		Description: strings.TrimSpace(get(1)),
		Metadata:    normalizeOptionalField(get(13)),
		URL:         normalizeOptionalField(get(16)),
	}
	cfg := ProjectConfig{
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
		if !contractExists(v) {
			sdk.Abort(fmt.Sprintf("membership NFT contract not found: %s", v))
		}
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
	cfg.WhitelistOnly = parseBoolField(get(17))
	normalizeProjectConfig(&cfg)
	args.ProjectConfig = cfg
	return args
}

// decodeCreateProposalArgs splits the string payload and normalizes optional bits like payouts.
func decodeCreateProposalArgs(payload *string) *CreateProposalArgs {
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
	iccCalls := parseICCField(get(10))

	var outcome *ProposalOutcome
	if len(payouts) > 0 || len(metaOutcome) > 0 || len(iccCalls) > 0 {
		outcome = &ProposalOutcome{
			Meta:   metaOutcome,
			Payout: payouts,
			ICC:    iccCalls,
		}
	}

	return &CreateProposalArgs{
		ProjectID:        projectID,
		Name:             strings.TrimSpace(get(1)),
		Description:      strings.TrimSpace(get(2)),
		OptionsList:      options,
		ProposalOutcome:  outcome,
		ProposalDuration: duration,
		Metadata:         metadata,
		ForcePoll:        forcePoll,
		URL:              normalizeOptionalField(get(9)),
	}
}

// decodeVoteProposalArgs expects `proposalId|choices` and converts indexes into uint slice.
func decodeVoteProposalArgs(payload *string) *VoteProposalArgs {
	raw := unwrapPayload(payload, "vote payload missing")
	parts := strings.Split(raw, "|")
	if len(parts) < 2 {
		sdk.Abort("vote payload requires proposalId|choices")
	}
	proposalID := parseUintField(parts[0], "proposal id")
	choices := parseChoiceField(parts[1])
	return &VoteProposalArgs{
		ProposalID: proposalID,
		Choices:    choices,
	}
}

// decodeAddFundsArgs extracts project id plus staking flag from the user payload.
func decodeAddFundsArgs(payload *string) *AddFundsArgs {
	raw := unwrapPayload(payload, "add funds payload missing")
	parts := strings.Split(raw, "|")
	if len(parts) < 2 {
		sdk.Abort("add funds payload requires projectId|toStake")
	}
	projectID := parseUintField(parts[0], "project id")
	toStake := parseBoolField(parts[1])
	return &AddFundsArgs{
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

// parseFloatField trims the input and aborts with a friendly field name on errors.
func parseFloatField(val string, field string) float64 {
	val = strings.TrimSpace(val)
	if val == "" {
		return -1
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
// Each option can be in format "text" or "text###url"
// The delimiter ### is used to separate text from URL to avoid conflicts with colons in URLs
func parseOptionsField(val string) []ProposalOptionInput {
	val = strings.TrimSpace(val)
	if val == "" {
		return []ProposalOptionInput{}
	}
	raw := strings.Split(val, ";")
	opts := make([]ProposalOptionInput, 0, len(raw))
	for _, opt := range raw {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}

		// Split text from optional URL using ### delimiter
		var text, url string
		delimiterIdx := strings.Index(opt, "###")

		if delimiterIdx > 0 {
			// Found delimiter - split text and URL
			text = strings.TrimSpace(opt[:delimiterIdx])
			url = strings.TrimSpace(opt[delimiterIdx+3:])
		} else if delimiterIdx == 0 {
			// Delimiter at the beginning with no text
			sdk.Abort("option text cannot be empty")
		} else {
			// No delimiter found - entire string is text
			text = opt
		}

		// Validate lengths
		if len(text) == 0 {
			sdk.Abort("option text cannot be empty")
		}
		if len(text) > MaxOptionTextLength {
			sdk.Abort(fmt.Sprintf("option text exceeds maximum length of %d characters", MaxOptionTextLength))
		}
		if len(url) > MaxURLLength {
			sdk.Abort(fmt.Sprintf("option URL exceeds maximum length of %d characters", MaxURLLength))
		}

		// Validate URL scheme if URL is provided - only HTTPS allowed
		if url != "" {
			if !strings.HasPrefix(url, "https://") {
				sdk.Abort("option URL must start with https://")
			}
		}

		opts = append(opts, ProposalOptionInput{
			Text: text,
			URL:  url,
		})
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
		key := strings.TrimSpace(split[0])
		value := strings.TrimSpace(split[1])
		// Validate contract existence for NFT contract updates
		if key == "update_membershipNFTContract" && value != "" {
			if !contractExists(value) {
				sdk.Abort(fmt.Sprintf("membership NFT contract not found: %s", value))
			}
		}
		meta[key] = value
	}
	return meta
}

// normalizeProjectConfig ensures configs always have sane fallbacks before persisting.
func normalizeProjectConfig(cfg *ProjectConfig) {
	// VotingSystem defaults to Democratic (0), no normalization needed
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
	if cfg.ProposalCost < 0 {
		cfg.ProposalCost = FallbackProposalCost
	}
	cfg.MembershipNftPayloadFormat = normalizeMembershipPayloadFormat(cfg.MembershipNftPayloadFormat)
}

// normalizeOptionalField trims funky placeholders like "" so metadata stays clean.
func normalizeOptionalField(val string) string {
	val = strings.TrimSpace(val)
	if val == "" || val == "\"\"" || val == "''" {
		return ""
	}
	return val
}

// parsePayoutField parses payout entries in format addr:amount:asset or addr:amount (legacy).
// New format: addr:amount:asset (e.g., "hive:alice:10:hive")
// Legacy format: addr:amount (e.g., "hive:alice:10") - asset will be nil
func parsePayoutField(val string) map[sdk.Address]PayoutEntry {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	payouts := map[sdk.Address]PayoutEntry{}
	entries := strings.Split(val, ";")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Split by colons to detect format
		parts := strings.Split(entry, ":")
		if len(parts) < 2 {
			sdk.Abort("invalid payout entry format (need addr:amount or addr:amount:asset)")
		}

		// Find the amount position (last numeric part before optional asset)
		// Format: protocol:address:amount or protocol:address:amount:asset
		var addr sdk.Address
		var amount Amount
		var asset sdk.Asset

		// Try new format first: addr:amount:asset
		if len(parts) >= 3 {
			// Last part might be asset
			lastPart := parts[len(parts)-1]
			secondLastPart := parts[len(parts)-2]

			// Check if last part is an asset (non-numeric)
			if _, err := strconv.ParseFloat(lastPart, 64); err != nil {
				// Last part is asset
				asset = AssetFromString(lastPart)
				amount = FloatToAmount(mustParseFloat(secondLastPart, "invalid payout amount"))
				addr = AddressFromString(strings.Join(parts[:len(parts)-2], ":"))
			} else {
				// Legacy format: last part is amount
				amount = FloatToAmount(mustParseFloat(lastPart, "invalid payout amount"))
				addr = AddressFromString(strings.Join(parts[:len(parts)-1], ":"))
				asset = sdk.Asset("") // Will be filled with project's default asset
			}
		} else {
			sdk.Abort("invalid payout entry (need at least addr:amount)")
		}

		payouts[addr] = PayoutEntry{Amount: amount, Asset: asset}
	}
	return payouts
}

// mustParseFloat parses a float or aborts with the given message.
func mustParseFloat(s string, errMsg string) float64 {
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		sdk.Abort(errMsg)
	}
	return val
}

// parseAddressList accepts comma/semicolon separated addresses and normalizes them.
func parseAddressList(val string) []sdk.Address {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	parts := strings.FieldsFunc(val, func(r rune) bool {
		return r == ';' || r == ',' || r == '\n' || r == '\t'
	})
	seen := map[string]struct{}{}
	addresses := make([]sdk.Address, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		addresses = append(addresses, AddressFromString(part))
	}
	if len(addresses) == 0 {
		return nil
	}
	return addresses
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

// parseVotingSystem accepts friendly strings or digits, defaulting to stake voting for safety.
func parseVotingSystem(val string) VotingSystem {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "0":
		return VotingSystemDemocratic
	case "1":
		return VotingSystemStake
	default:
		return VotingSystemStake
	}
}

// decodeWhitelistPayload reads projectId|addr1;addr2 and returns both parts.
func decodeWhitelistPayload(payload *string) (uint64, []sdk.Address) {
	raw := unwrapPayload(payload, "whitelist payload required")
	parts := strings.Split(raw, "|")
	if len(parts) < 2 {
		sdk.Abort("whitelist payload requires projectId|addresses")
	}
	projectID := parseUintField(parts[0], "project id")
	addresses := parseAddressList(parts[1])
	if len(addresses) == 0 {
		sdk.Abort("whitelist payload requires addresses")
	}
	return projectID, addresses
}

// parseICCField parses inter-contract call entries.
// Format: contract_addr|function|payload|asset1=amount1,asset2=amount2;contract_addr2|...
// Example: "contract:foo|myFunc|{\"arg\":1}|HIVE=1.5,HBD=2.0;contract:bar|otherFunc|{}|HIVE=1.0"
// Assets are optional. Multiple calls are separated by semicolons.
func parseICCField(val string) []InterContractCall {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}

	calls := []InterContractCall{}
	entries := strings.Split(val, ";")

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Split by pipes to get: contract_addr|function|payload[|assets]
		parts := strings.Split(entry, "|")
		if len(parts) < 3 {
			sdk.Abort("invalid ICC entry format (need contract|function|payload[|assets])")
		}

		contractAddr := strings.TrimSpace(parts[0])
		function := strings.TrimSpace(parts[1])
		payload := strings.TrimSpace(parts[2])

		if contractAddr == "" {
			sdk.Abort("ICC contract address cannot be empty")
		}
		if !contractExists(contractAddr) {
			sdk.Abort(fmt.Sprintf("ICC contract not found: %s", contractAddr))
		}
		if function == "" {
			sdk.Abort("ICC function cannot be empty")
		}

		// Parse assets if provided
		var assets map[sdk.Asset]Amount
		if len(parts) >= 4 && strings.TrimSpace(parts[3]) != "" {
			assets = parseICCAssets(parts[3])
		}

		calls = append(calls, InterContractCall{
			ContractAddress: contractAddr,
			Function:        function,
			Payload:         payload,
			Assets:          assets,
		})
	}

	return calls
}

// parseICCAssets parses asset mappings for ICC.
// Format: asset1=amount1,asset2=amount2
// Example: "HIVE=1.5,HBD=2.0"
// Each asset can only appear once.
func parseICCAssets(val string) map[sdk.Asset]Amount {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}

	assets := map[sdk.Asset]Amount{}
	pairs := strings.Split(val, ",")

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			sdk.Abort("invalid ICC asset format (need ASSET=amount)")
		}

		assetStr := strings.TrimSpace(strings.ToUpper(parts[0]))
		amountStr := strings.TrimSpace(parts[1])

		if assetStr == "" {
			sdk.Abort("ICC asset name cannot be empty")
		}

		asset := AssetFromString(assetStr)

		// Check if asset already exists
		if _, exists := assets[asset]; exists {
			sdk.Abort(fmt.Sprintf("ICC asset %s specified multiple times", assetStr))
		}

		amount := FloatToAmount(mustParseFloat(amountStr, "invalid ICC asset amount"))
		if amount <= 0 {
			sdk.Abort("ICC asset amount must be positive")
		}

		assets[asset] = amount
	}

	return assets
}
