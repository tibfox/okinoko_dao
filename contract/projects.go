package main

import (
	"fmt"
	"okinoko_dao/sdk"
	"time"
)

// CreateProjectArgs defines the JSON payload for creating a project
type CreateProjectArgs struct {
	Name          string            `json:"name"`
	ProjectConfig ProjectConfig     `json:"config"` // JSON string representing ProjectConfig
	JsonMetadata  map[string]string `json:"meta,omitempty"`
}

// ProjectConfig contains toggles & params for a project
type ProjectConfig struct {
	VotingSystem          VotingSystem `json:"votingSystem"`     // democratic or stake
	ThresholdPercent      float64      `json:"threshold"`        // minimum percentage an answer needs to have to be valid
	QuorumPercent         float64      `json:"quorum"`           // minimum percentage of individual votes for a proposal to get a valid result
	ProposalDurationHours uint64       `json:"proposalDuration"` // duration for a proposal to run until tallyable
	ExecutionDelayHours   uint64       `json:"executionDelay"`   // delay between tally and execution ofd a proposal
	LeaveCooldownHours    uint64       `json:"leaveCooldown"`    // cooldown for a member to leave the project
	ProposalCost          float64      `json:"proposalCost"`     // minimum transfer for creating a proposal (will go to funds)
	StakeMinAmt           float64      `json:"stakeMinAmount"`   // minimum transfer for a membership in a stake-based project
}

// Voting system & permission
type VotingSystem string

const (
	SystemDemocratic VotingSystem = "democratic" // everyone has an equal stake and vote
	SystemStake      VotingSystem = "stake"      // votes are weighted by stake of the meber
)

// Project - minimal required for proposals
type Project struct {
	ID         uint64                 `json:"id"`
	Owner      sdk.Address            `json:"owner"`
	Name       string                 `json:"name"`
	Config     ProjectConfig          `json:"config"`
	Funds      float64                `json:"funds"`
	FundsAsset sdk.Asset              `json:"fundsAsset"`
	Members    map[sdk.Address]Member `json:"members"`
	Paused     bool                   `json:"paused"`
	Tx         string                 `json:"tx"`
}

// Member represents a project member
type Member struct {
	Address       sdk.Address `json:"address"`
	Stake         float64     `json:"stake"`
	JoinedAt      int64       `json:"joinedAt"`
	LastActionAt  int64       `json:"lastActionAt"`
	ExitRequested int64       `json:"exitRequested"`
	Reputation    int64       `json:"reputation"`
}

type Permission string

const (
	PermCreatorOnly Permission = "creator"
	PermAnyMember   Permission = "member"
)

// -----------------------------------------------------------------------------
// Project operations
// -----------------------------------------------------------------------------
//
//go:wasmexport project_create
func CreateProject(payload *string) *string {
	input := FromJSON[CreateProjectArgs](*payload, "CreateProjectArgs")

	caller := getSenderAddress()

	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow(sdk.GetEnv().Intents)
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}

	// determine required initial stake
	var initialTrasurey float64
	if input.ProjectConfig.VotingSystem == "" {
		input.ProjectConfig.VotingSystem = SystemStake
	}

	if input.ProjectConfig.StakeMinAmt <= 0 {
		input.ProjectConfig.StakeMinAmt = ta.Limit
	} else {
		if input.ProjectConfig.StakeMinAmt > ta.Limit {
			sdk.Abort(fmt.Sprintf("transfer limit %f < StakeMinAmt %f", ta.Limit, input.ProjectConfig.StakeMinAmt))
		} else {
			initialTrasurey = ta.Limit - input.ProjectConfig.StakeMinAmt
		}
	}

	// draw the funds
	mAmount := int64(ta.Limit * 1000)
	sdk.HiveDraw(mAmount, ta.Token)

	// --- create project ---
	id := getCount(ProjectsCount)
	now := time.Now().Unix()

	if input.ProjectConfig.ThresholdPercent <= 0 {
		input.ProjectConfig.ThresholdPercent = 50.001
	}
	if input.ProjectConfig.QuorumPercent <= 0 {
		input.ProjectConfig.QuorumPercent = 50.001
	}
	if input.ProjectConfig.ProposalDurationHours <= 0 {
		input.ProjectConfig.ProposalDurationHours = 24
	}
	if input.ProjectConfig.ExecutionDelayHours <= 0 {
		input.ProjectConfig.ExecutionDelayHours = 4
	}
	if input.ProjectConfig.LeaveCooldownHours <= 0 {
		input.ProjectConfig.LeaveCooldownHours = 24
	}
	if input.ProjectConfig.ProposalCost <= 0 {
		input.ProjectConfig.ProposalCost = 1
	}

	prj := Project{
		ID:         id,
		Owner:      caller,
		Name:       input.Name,
		Config:     input.ProjectConfig,
		Funds:      initialTrasurey,
		FundsAsset: ta.Token,
		Members:    map[sdk.Address]Member{},
		Paused:     false,
		Tx:         *sdk.GetEnvKey("tx.id"),
	}

	prj.Members[caller] = Member{
		Address:      caller,
		Stake:        ta.Limit - initialTrasurey,
		JoinedAt:     now,
		LastActionAt: now,
		Reputation:   0,
	}

	saveProject(&prj)
	setCount(ProjectsCount, id+1)

	emitProjectCreatedEvent(id, caller.String())
	emitFundsAdded(prj.ID, caller.String(), ta.Limit-initialTrasurey, ta.Token.String(), true)
	emitFundsAdded(prj.ID, caller.String(), initialTrasurey, ta.Token.String(), false)
	return strptr(fmt.Sprintf("project %d created", id))
}

// Join project using the first valid transfer intent
//
//go:wasmexport project_join
func JoinProject(projectID *uint64) *string {
	prj := loadProject(*projectID)
	if prj.Paused {
		sdk.Abort("project paused")
	}

	caller := getSenderAddress()
	now := time.Now().Unix()

	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow(sdk.GetEnv().Intents)
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}

	// draw the funds
	mAmount := int64(ta.Limit * 1000)
	sdk.HiveDraw(mAmount, ta.Token)

	if ta.Limit != prj.Config.StakeMinAmt && prj.Config.VotingSystem == SystemDemocratic {
		sdk.Abort(fmt.Sprintf("democratic projects require exactly %f %s", prj.Config.StakeMinAmt, ta.Token.String()))
	}

	if ta.Limit < prj.Config.StakeMinAmt && prj.Config.VotingSystem == SystemStake {
		sdk.Abort(fmt.Sprintf("stake too low, minimum %f %s required", prj.Config.StakeMinAmt, ta.Token.String()))
	}
	prj.Members[caller] = Member{
		Address:      caller,
		Stake:        ta.Limit,
		JoinedAt:     now,
		LastActionAt: now,
	}
	saveProject(prj)
	emitJoinedEvent(prj.ID, caller.String())
	emitFundsAdded(prj.ID, caller.String(), ta.Limit, ta.Token.String(), true)
	return strptr("joined")
}

// Leave project with cooldown
//
//go:wasmexport project_leave
func LeaveProject(projectID *uint64) *string {
	caller := getSenderAddress()
	prj := loadProject(*projectID)
	if prj.Paused {
		sdk.Abort("project paused")
	}

	member, ok := prj.Members[caller]
	if !ok {
		sdk.Abort("not a member")
	}

	now := time.Now().Unix()
	if member.ExitRequested == 0 {
		member.ExitRequested = now
		prj.Members[caller] = member
		saveProject(prj)
		return strptr("exit requested")
	}
	if now-member.ExitRequested < int64(prj.Config.LeaveCooldownHours*3600) {
		sdk.Abort("cooldown not passed")
	}

	// refund stake
	withdraw := member.Stake
	mAmount := int64(withdraw * 1000)
	sdk.HiveTransfer(caller, mAmount, prj.FundsAsset)

	delete(prj.Members, caller)
	saveProject(prj)
	emitLeaveEvent(prj.ID, caller.String())
	emitFundsRemoved(prj.ID, caller.String(), withdraw, prj.FundsAsset.String(), true)
	return strptr("exit finished")
}

type AddFundsArgs struct {
	ProjectId uint64 `json:"id"`
	ToStake   bool   `json:"toStake"`
}

// AddFunds adds funds either to project treasury or to member stake
//
//go:wasmexport project_funds
func AddFunds(payload *string) *string {
	input := FromJSON[AddFundsArgs](*payload, "AddFundsArgs")
	prj := loadProject(input.ProjectId)
	caller := getSenderAddress()

	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow(sdk.GetEnv().Intents)
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}

	// validate intent token matches project treasury asset
	if ta.Token != prj.FundsAsset {
		sdk.Abort(fmt.Sprintf("invalid asset, expected %s", prj.FundsAsset.String()))
	}

	// draw the funds
	mAmount := int64(ta.Limit * 1000)
	sdk.HiveDraw(mAmount, ta.Token)

	if input.ToStake {
		if prj.Config.VotingSystem == SystemDemocratic {
			sdk.Abort("cannot add member stake > StakeMinAmt in democratic systems")
		} else {
			member := getMember(caller, prj.Members)
			member.Stake += ta.Limit
			member.LastActionAt = time.Now().Unix()
			prj.Members[caller] = member
		}

	} else {
		// add to treasury for payouts
		prj.Funds += ta.Limit

	}

	saveProject(prj)
	emitFundsAdded(prj.ID, caller.String(), ta.Limit, ta.Token.String(), input.ToStake)
	return strptr("funds added")
}

func getMember(user sdk.Address, members map[sdk.Address]Member) Member {
	// update member stake
	m, ok := members[user]
	if !ok {
		sdk.Abort(fmt.Sprintf("%s is not a member", user.String()))
	}
	return m
}

// Transfer project ownership
//
//go:wasmexport project_transfer
func TransferProjectOwnership(projectID uint64, newOwner sdk.Address) {
	caller := getSenderAddress()
	prj := loadProject(projectID)
	if caller != prj.Owner {
		sdk.Abort("only owner can transfer")
	}

	if _, ok := prj.Members[newOwner]; !ok {
		sdk.Abort("new owner must be a member")
	}

	prj.Owner = newOwner
	saveProject(prj)
}

// Emergency pause/unpause
//
//go:wasmexport project_pause
func EmergencyPauseImmediate(projectID uint64, pause bool) {
	caller := getSenderAddress()
	prj := loadProject(projectID)
	if caller != prj.Owner {
		sdk.Abort("only owner can pause/unpause")
	}
	prj.Paused = pause
	saveProject(prj)
}

// Save/load
func saveProject(prj *Project) {
	key := projectKey(prj.ID)
	sdk.StateSetObject(key, ToJSON(prj, "project"))
}

func loadProject(id uint64) *Project {
	key := projectKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("project not found")
	}
	return FromJSON[Project](*ptr, "Project")
}
