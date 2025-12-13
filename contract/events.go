package main

import (
	"fmt"
	"okinoko_dao/sdk"
	"sort"
	"strconv"
	"strings"
)

// sanitizeEventValue strips delimiter characters from arbitrary user text so logs stay parseable.
func sanitizeEventValue(val string) string {
	val = strings.ReplaceAll(val, "|", " ")
	val = strings.ReplaceAll(val, "\n", " ")
	return strings.TrimSpace(val)
}

func formatOptionalString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return sanitizeEventValue(*ptr)
}

func formatOptionalUint(ptr *uint64) string {
	if ptr == nil {
		return ""
	}
	return strconv.FormatUint(*ptr, 10)
}

func formatMetadataMap(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", sanitizeEventValue(key), sanitizeEventValue(meta[key])))
	}
	return strings.Join(parts, ";")
}

func formatPayoutMap(payout map[sdk.Address]Amount) string {
	if len(payout) == 0 {
		return ""
	}
	type payoutEntry struct {
		addr   string
		amount Amount
	}
	entries := make([]payoutEntry, 0, len(payout))
	for addr, amt := range payout {
		entries = append(entries, payoutEntry{
			addr:   AddressToString(addr),
			amount: amt,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].addr < entries[j].addr
	})
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fmt.Sprintf("%s:%f", entry.addr, AmountToFloat(entry.amount)))
	}
	return strings.Join(out, ";")
}

func formatOptionsList(opts []string) string {
	if len(opts) == 0 {
		return ""
	}
	clean := make([]string, 0, len(opts))
	for _, opt := range opts {
		clean = append(clean, sanitizeEventValue(opt))
	}
	return strings.Join(clean, ";")
}

// emitJoinedEvent writes a tiny "mj" log so watchers know someone fresh just joined the project adress.
func emitJoinedEvent(projectId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"mj|id:%d|by:%s",
		projectId,
		memberAddress,
	))
}

// emitLeaveEvent mirrors the join ping but signals a seat freed up inside the
func emitLeaveEvent(projectId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"ml|id:%d|by:%s",
		projectId,
		memberAddress,
	))
}

// emitProjectCreatedEvent gives explorers a neat ping without scanning full storage diffs.
func emitProjectCreatedEvent(project *Project, createdByAddress string) {
	payload := fmt.Sprintf(
		"dc|id:%d|by:%s|name:%s|description:%s|metadata:%s|url:%s|asset:%s|voting:%s|threshold:%f|quorum:%f|proposalDuration:%d|executionDelay:%d|leaveCooldown:%d|proposalCost:%f|stakeMin:%f|membershipContract:%s|membershipFunction:%s|membershipNft:%s|membershipPayload:%s|membersOnly:%s|whitelistOnly:%s",
		project.ID,
		createdByAddress,
		sanitizeEventValue(project.Name),
		sanitizeEventValue(project.Description),
		sanitizeEventValue(project.Metadata),
		sanitizeEventValue(project.URL),
		AssetToString(project.FundsAsset),
		project.Config.VotingSystem.String(),
		project.Config.ThresholdPercent,
		project.Config.QuorumPercent,
		project.Config.ProposalDurationHours,
		project.Config.ExecutionDelayHours,
		project.Config.LeaveCooldownHours,
		project.Config.ProposalCost,
		project.Config.StakeMinAmt,
		formatOptionalString(project.Config.MembershipNFTContract),
		formatOptionalString(project.Config.MembershipNFTContractFunction),
		formatOptionalUint(project.Config.MembershipNFT),
		sanitizeEventValue(project.Config.MembershipNftPayloadFormat),
		strconv.FormatBool(project.Config.ProposalsMembersOnly),
		strconv.FormatBool(project.Config.WhitelistOnly),
	)
	sdk.Log(payload)
}

// emitProposalCreatedEvent keeps observers updated with a short pc line for every new idea.
func emitProposalCreatedEvent(prpsl *Proposal, projectID uint64, creator string, options []string) {
	var payoutStr string
	var outcomeMeta string
	if prpsl.Outcome != nil {
		payoutStr = formatPayoutMap(prpsl.Outcome.Payout)
		outcomeMeta = formatMetadataMap(prpsl.Outcome.Meta)
	}
	payload := fmt.Sprintf(
		"pc|id:%d|project:%d|by:%s|name:%s|description:%s|metadata:%s|url:%s|duration:%d|isPoll:%s|options:%s|payouts:%s|outcomeMeta:%s",
		prpsl.ID,
		projectID,
		creator,
		sanitizeEventValue(prpsl.Name),
		sanitizeEventValue(prpsl.Description),
		sanitizeEventValue(prpsl.Metadata),
		sanitizeEventValue(prpsl.URL),
		prpsl.DurationHours,
		strconv.FormatBool(prpsl.IsPoll),
		formatOptionsList(options),
		payoutStr,
		outcomeMeta,
	)
	sdk.Log(payload)
}

// emitProposalStateChangedEvent is the swiss army knife log entry for any state flip.
func emitProposalStateChangedEvent(proposalId uint64, proposalState ProposalState) {
	sdk.Log(fmt.Sprintf(
		"ps|id:%d|s:%s",
		proposalId,
		proposalState.String(),
	))
}

// emitProposalExecutionDelayEvent logs when a passed poll becomes executable so runners can queue it.
func emitProposalExecutionDelayEvent(projectId uint64, proposalId uint64, readyAt int64) {
	sdk.Log(fmt.Sprintf(
		"px|pId:%d|prId:%d|ready:%s",
		projectId,
		proposalId,
		strconv.FormatInt(readyAt, 10),
	))
}

// emitProposalResultEvent leaves a short hint whether funds moved or config toggled after execution.
func emitProposalResultEvent(projectId uint64, proposalId uint64, result string) {
	sdk.Log(fmt.Sprintf(
		"pr|pId:%d|prId:%d|r:%s",
		projectId,
		proposalId,
		result,
	))
}

// emitProposalConfigUpdatedEvent spells out field diffs so auditors can track sensitive flips.
func emitProposalConfigUpdatedEvent(projectId uint64, proposalId uint64, field string, old string, new string) {
	sdk.Log(fmt.Sprintf(
		"pm|pId:%d|prId:%d|f:%s|old:%s|new:%s",
		projectId,
		proposalId,
		field,
		old,
		new,
	))
}

// emitVoteCasted includes raw choice indexes plus weight so quorum math can be replayed from logs only.
func emitVoteCasted(proposalId uint64, voter string, choices []uint, weight float64) {
	sdk.Log(fmt.Sprintf(
		"v|id:%d|by:%s|cs:%s|w:%f",
		proposalId,
		voter,
		UIntSliceToString(choices),
		weight,
	))
}

// emitFundsAdded tells indexing bots whether the transfer beefed up treasury or user stake via one bool char.
func emitFundsAdded(projectId uint64, addedByAddress string, amount float64, asset string, toStake bool) {
	sdk.Log(fmt.Sprintf(
		"af|id:%d|by:%s|am:%f|as:%s|s:%s",
		projectId,
		addedByAddress,
		amount,
		asset,
		strconv.FormatBool(toStake),
	))
}

// emitFundsRemoved mirrors the add log but lets us trace payouts and unstaking in a single terse line.
func emitFundsRemoved(projectId uint64, removedToAddress string, amount float64, asset string, fromStake bool) {
	sdk.Log(fmt.Sprintf(
		"rf|id:%d|to:%s|am:%f|as:%s|fs:%s",
		projectId,
		removedToAddress,
		amount,
		asset,
		strconv.FormatBool(fromStake),
	))
}

// emitWhitelistEvent records whitelist additions/removals for downstream indexers.
func emitWhitelistEvent(projectId uint64, action string, addresses []sdk.Address) {
	if len(addresses) == 0 {
		return
	}
	addrs := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		addrs = append(addrs, AddressToString(addr))
	}
	sdk.Log(fmt.Sprintf(
		"wl|id:%d|act:%s|addrs:%s",
		projectId,
		sanitizeEventValue(action),
		strings.Join(addrs, ";"),
	))
}
