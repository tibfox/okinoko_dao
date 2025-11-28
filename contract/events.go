package main

import (
	"fmt"
	"okinoko_dao/contract/dao"
	"okinoko_dao/sdk"
	"strconv"
)

// emitJoinedEvent writes a tiny "mj" log so watchers know someone fresh just joined the project adress.
func emitJoinedEvent(projectId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"mj|id:%d|by:%s",
		projectId,
		memberAddress,
	))
}

// emitLeaveEvent mirrors the join ping but signals a seat freed up inside the dao.
func emitLeaveEvent(projectId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"ml|id:%d|by:%s",
		projectId,
		memberAddress,
	))
}

// emitProjectCreatedEvent gives explorers a neat ping without scanning full storage diffs.
func emitProjectCreatedEvent(projectId uint64, createdByAddress string) {
	sdk.Log(fmt.Sprintf(
		"dc|id:%d|by:%s",
		projectId,
		createdByAddress,
	))
}

// emitProposalCreatedEvent keeps observers updated with a short pc line for every new idea.
func emitProposalCreatedEvent(proposalId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"pc|id:%d|by:%s",
		proposalId,
		memberAddress,
	))
}

// emitProposalStateChangedEvent is the swiss army knife log entry for any state flip.
func emitProposalStateChangedEvent(proposalId uint64, proposalState dao.ProposalState) {
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
