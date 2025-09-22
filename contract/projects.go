package main

import (
	"fmt"
	"okinoko_dao/sdk"
	"time"
)

// ProjectConfig contains toggles & params for a project
type ProjectConfig struct {
	VotingSystem         VotingSystem `json:"votingSystem"`
	ThresholdPercent     int          `json:"threshold"`
	QuorumPercent        int          `json:"quorum"`
	ProposalDurationSecs int64        `json:"proposalDuration"`
	ExecutionDelaySecs   int64        `json:"executionDelay"`
	LeaveCooldownSecs    int64        `json:"leaveCooldown"`
	ProposalCost         float64      `json:"proposalCost"`
	DemocraticExactAmt   float64      `json:"democraticAmount"`
	StakeMinAmt          float64      `json:"stakeMinAmount"`
}

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

// Voting system & permission
type VotingSystem string

const (
	SystemDemocratic VotingSystem = "democratic"
	SystemStake      VotingSystem = "stake"
)

type Permission string

const (
	PermCreatorOnly Permission = "creator"
	PermAnyMember   Permission = "member"
)

// CreateProjectArgs defines the JSON payload for creating a project
type CreateProjectArgs struct {
	Name          string            `json:"name"`
	ProjectConfig ProjectConfig     `json:"config"` // JSON string representing ProjectConfig
	JsonMetadata  map[string]string `json:"meta,omitempty"`
}

// -----------------------------------------------------------------------------
// Project operations
// -----------------------------------------------------------------------------
//
//go:wasmexport project_create
func CreateProject(payload *string) *string {
	input := FromJSON[CreateProjectArgs](*payload, "CreateProjectArgs")

	caller := getSenderAddress()
	bal := sdk.GetBalance(caller, sdk.AssetHive)
	sdk.Log(fmt.Sprintf("bal: %d", bal))

	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow(sdk.GetEnv().Intents)
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}

	// determine required initial stake
	var initialStake float64
	if input.ProjectConfig.VotingSystem == SystemDemocratic {
		initialStake = input.ProjectConfig.DemocraticExactAmt
	} else {
		initialStake = input.ProjectConfig.StakeMinAmt
	}

	// check that the transfer is enough
	if ta.Limit < initialStake {
		sdk.Abort(fmt.Sprintf("transfer limit %f < required initial stake %f", ta.Limit, initialStake))
	}

	// draw the funds
	mAmount := int64(ta.Limit * 1000)
	sdk.HiveDraw(mAmount, ta.Token)

	// --- create project ---
	id := getCount(ProjectsCount)
	now := time.Now().Unix()

	prj := Project{
		ID:         id,
		Owner:      caller,
		Name:       input.Name,
		Config:     input.ProjectConfig,
		Funds:      0,
		FundsAsset: ta.Token,
		Members:    map[sdk.Address]Member{},
		Paused:     false,
		Tx:         *sdk.GetEnvKey("tx.id"),
	}

	// add creator as member
	creatorStake := 1.0
	if input.ProjectConfig.VotingSystem == SystemStake {
		creatorStake = ta.Limit
	}
	prj.Members[caller] = Member{
		Address:      caller,
		Stake:        creatorStake,
		JoinedAt:     now,
		LastActionAt: now,
		Reputation:   0,
	}

	saveProject(&prj)
	setCount(ProjectsCount, id+1)

	emitProjectCreatedEvent(id, caller.String())
	emitFundsAdded(prj.ID, caller.String(), *ta, true)
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

	switch prj.Config.VotingSystem {
	case SystemDemocratic:
		if ta.Limit != prj.Config.DemocraticExactAmt {
			sdk.Abort(fmt.Sprintf("democratic projects require exactly %f %s", prj.Config.DemocraticExactAmt, ta.Token.String()))
		}
		prj.Members[caller] = Member{
			Address:      caller,
			Stake:        1,
			JoinedAt:     now,
			LastActionAt: now,
			Reputation:   0,
		}
		prj.Funds += ta.Limit

	case SystemStake:
		if ta.Limit < prj.Config.StakeMinAmt {
			sdk.Abort(fmt.Sprintf("stake too low, minimum %f required", prj.Config.StakeMinAmt))
		}
		prj.Members[caller] = Member{
			Address:      caller,
			Stake:        ta.Limit,
			JoinedAt:     now,
			LastActionAt: now,
		}
		prj.Funds += ta.Limit
	}

	saveProject(prj)
	emitJoinedEvent(prj.ID, caller.String())
	emitFundsAdded(prj.ID, caller.String(), *ta, true)
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
	if now-member.ExitRequested < prj.Config.LeaveCooldownSecs {
		sdk.Abort("cooldown not passed")
	}

	// refund stake or democratic amount
	withdraw := member.Stake
	if prj.Config.VotingSystem == SystemDemocratic {
		withdraw = prj.Config.DemocraticExactAmt
	}

	if prj.Funds < withdraw {
		sdk.Abort("insufficient funds")
	}
	prj.Funds -= withdraw
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
		// Only stake-based projects can add member funds
		if prj.Config.VotingSystem != SystemStake {
			sdk.Abort("cannot add member funds in democratic projects")
		}
		// update member stake
		m, ok := prj.Members[caller]
		if !ok {
			sdk.Abort("caller is not a member")
		}
		m.Stake += ta.Limit
		m.LastActionAt = time.Now().Unix()
		prj.Members[caller] = m
	} else {
		// add to treasury for payouts
		prj.Funds += ta.Limit
	}

	saveProject(prj)
	emitFundsAdded(prj.ID, caller.String(), *ta, input.ToStake)
	return strptr("funds added")
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
