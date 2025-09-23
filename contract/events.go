package main

import (
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
)

func emitJoinedEvent(projectId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"MemberJoined|id:%d|by:%s",
		projectId,
		memberAddress,
	))
}

func emitLeaveEvent(projectId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"MemberLeft|id:%d|by:%s",
		projectId,
		memberAddress,
	))
}

func emitProjectCreatedEvent(projectId uint64, createdByAddress string) {
	sdk.Log(fmt.Sprintf(
		"ProjectCreated|id:%d|by:%s",
		projectId,
		createdByAddress,
	))
}

func emitProposalCreatedEvent(proposalId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"ProposalCreated|id:%d|by:%s",
		proposalId,
		memberAddress,
	))
}

func emitProposalStateChangedEvent(proposalId uint64, proposalState ProposalState) {
	sdk.Log(fmt.Sprintf(
		"ProposalState|id:%d|state:%s",
		proposalId,
		proposalState,
	))
}

func emitProposalTalliedEvent(proposalId uint64, proposalState string) {
	sdk.Log(fmt.Sprintf(
		"ProposalTallied|id:%d|state:%s",
		proposalId,
		proposalState,
	))
}

func emitVoteCasted(proposalId uint64, voter string, choices []uint, weight float64) {
	sdk.Log(fmt.Sprintf(
		"Vote|id:%d|by:%s|choices:%s|weight:%f",
		proposalId,
		voter,
		UIntSliceToString(choices),
		weight,
	))
}

func emitFundsAdded(projectId uint64, addedByAddress string, ta TransferAllow, toStake bool) {
	sdk.Log(fmt.Sprintf(
		"AddFunds|id:%d|by:%s|amount:%f|asset:%s|stake:%s",
		projectId,
		addedByAddress,
		ta.Limit,
		ta.Token.String(),
		strconv.FormatBool(toStake),
	))
}

func emitFundsRemoved(projectId uint64, removedToAddress string, amount float64, asset string, fromStake bool) {
	sdk.Log(fmt.Sprintf(
		"RmoveFunds|id:%d|to:%s|amount:%f|asset:%s|fromStake:%s",
		projectId,
		removedToAddress,
		amount,
		asset,
		strconv.FormatBool(fromStake),
	))
}
