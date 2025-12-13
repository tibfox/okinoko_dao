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

	var outcome *ProposalOutcome
	if len(payouts) > 0 || len(metaOutcome) > 0 {
		outcome = &ProposalOutcome{
			Meta:   metaOutcome,
			Payout: payouts,
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

// parsePayoutField parses addr:amount entries and converts floats to Amount scale.
func parsePayoutField(val string) map[sdk.Address]Amount {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	payouts := map[sdk.Address]Amount{}
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
		addr := AddressFromString(strings.TrimSpace(entry[:idx]))
		amountStr := strings.TrimSpace(entry[idx+1:])
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			sdk.Abort("invalid payout amount")
		}
		payouts[addr] = FloatToAmount(amount)
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
