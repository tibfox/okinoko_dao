package main

import (
	"fmt"
	"okinoko_dao/contract/dao"
	"okinoko_dao/sdk"
	"strconv"
)

// emitJoinedEvent logs an event indicating that a member has joined a project.
//
// The event is recorded in the format:
//
//	MemberJoined|id:<projectId>|by:<memberAddress>
func emitJoinedEvent(projectId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"mj|id:%d|by:%s",
		projectId,
		memberAddress,
	))
}

// emitLeaveEvent logs an event indicating that a member has left a project.
//
// The event is recorded in the format:
//
//	MemberLeft|id:<projectId>|by:<memberAddress>
func emitLeaveEvent(projectId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"ml|id:%d|by:%s",
		projectId,
		memberAddress,
	))
}

// emitProjectCreatedEvent logs an event indicating that a new project was created.
//
// The event is recorded in the format:
//
//	ProjectCreated|id:<projectId>|by:<createdByAddress>
func emitProjectCreatedEvent(projectId uint64, createdByAddress string) {
	sdk.Log(fmt.Sprintf(
		"dc|id:%d|by:%s",
		projectId,
		createdByAddress,
	))
}

// emitProposalCreatedEvent logs an event when a proposal is created by a member.
//
// The event is recorded in the format:
//
//	ProposalCreated|id:<proposalId>|by:<memberAddress>
func emitProposalCreatedEvent(proposalId uint64, memberAddress string) {
	sdk.Log(fmt.Sprintf(
		"pc|id:%d|by:%s",
		proposalId,
		memberAddress,
	))
}

// emitProposalStateChangedEvent logs a state change for a given proposal.
//
// The event is recorded in the format:
//
//	ProposalState|id:<proposalId>|state:<proposalState>
func emitProposalStateChangedEvent(proposalId uint64, proposalState dao.ProposalState) {
	sdk.Log(fmt.Sprintf(
		"ps|id:%d|s:%s",
		proposalId,
		proposalState.String(),
	))
}

// emitProposalResultEvent logs the final result of a proposal within a project.
//
// The event is recorded in the format:
//
//	ProposalResult|projectId:<projectId>|proposalId:<proposalId>|result:<result>
func emitProposalResultEvent(projectId uint64, proposalId uint64, result string) {
	sdk.Log(fmt.Sprintf(
		"pr|pId:%d|prId:%d|r:%s",
		projectId,
		proposalId,
		result,
	))
}

// emitVoteCasted logs a vote cast for a proposal by a member.
//
// The event is recorded in the format:
//
//	Vote|id:<proposalId>|by:<voter>|choices:<choices>|weight:<weight>
func emitVoteCasted(proposalId uint64, voter string, choices []uint, weight float64) {
	sdk.Log(fmt.Sprintf(
		"v|id:%d|by:%s|cs:%s|w:%f",
		proposalId,
		voter,
		UIntSliceToString(choices),
		weight,
	))
}

// emitFundsAdded logs when funds are added to a project, optionally staking them.
//
// The event is recorded in the format:
//
//	AddFunds|id:<projectId>|by:<addedByAddress>|amount:<amount>|asset:<asset>|stake:<toStake>
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

// emitFundsRemoved logs when funds are removed from a project, optionally from staking.
//
// The event is recorded in the format:
//
//	RemoveFunds|id:<projectId>|to:<removedToAddress>|amount:<amount>|asset:<asset>|fromStake:<fromStake>
//
// Note: "RmoveFunds" in the log string appears to be a typo and should likely be corrected to "RemoveFunds".
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
