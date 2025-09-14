package main

import (
	"encoding/json"
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
)

const prjCreationMinHIVE = 10
const prjCreationMinHBD = 1

// ProjectConfig contains toggles & params for a project
type ProjectConfig struct {
	ProposalPermission   Permission   `json:"propPerm"`       // who may create proposals
	ExecutePermission    Permission   `json:"execPerm"`       // who may execute transfers
	VotingSystem         VotingSystem `json:"v"`              // democratic or stake_based
	ThresholdPercent     int          `json:"threshold"`      // percent required to pass (0-100)
	QuorumPercent        int          `json:"quorum"`         // percent of voting power that must participate (0-100)
	ProposalDurationSecs int64        `json:"propDur"`        // default duration
	ExecutionDelaySecs   int64        `json:"execDelay"`      // delay after pass before exec allowed
	LeaveCooldownSecs    int64        `json:"leaveCd"`        // cooldown for leaving/withdrawing
	DemocraticExactAmt   float64      `json:"demAmount"`      // exact amount required to join democratic
	StakeMinAmt          float64      `json:"stateMinAmount"` // min stake for stake-based joining
	ProposalCost         float64      `json:"propCost"`       // fee for creating proposals (goes to project funds)
	// TODO: add snapshot
	// EnableSnapshot       bool         `json:"snapshot"`       // snapshot member stakes at proposal start
}

// Project - stored under project:<id>
type Project struct {
	ID           int64                  `json:"id"`
	Owner        sdk.Address            `json:"owner"`
	Name         string                 `json:"name"`
	Description  string                 `json:"desc"`
	JsonMetadata map[string]string      `json:"meta,omitempty"`
	Config       ProjectConfig          `json:"cfg"`
	Members      map[sdk.Address]Member `json:"members,omitempty"` // key: address string
	Funds        float64                `json:"funds"`             // pool in minimal unit
	FundsAsset   sdk.Asset              `json:"funds_asset"`
	CreationTxID string                 `json:"txID"`
	Paused       bool                   `json:"paused"`
}

// Member represents a project member
type Member struct {
	Address       sdk.Address `json:"a"`
	Stake         float64     `json:"stake"`
	Role          string      `json:"role"` // "admin" or "member"
	JoinedTxID    string      `json:"txID"`
	JoinedAt      int64       `json:"joined_at"` // unix ts
	LastActionAt  int64       `json:"action_at"` // last stake/join/withdraw time (for cooldown)
	ExitRequested int64       `json:"exit_req"`  // 0 if not requested
	Reputation    int64       `json:"rep"`       // initially 0 | every vote += 1 | every passed proposal += 5
}

// Voting system for a project
type VotingSystem string

const (
	SystemDemocratic VotingSystem = "dem"   // every member has an equal vote
	SystemStake      VotingSystem = "stake" // ever member has a different vote weight - based on the stake in the project treasury fund
)

// Permission for who may create/execute proposals
type Permission string

const (
	PermCreatorOnly Permission = "creator"
	PermAnyMember   Permission = "member"
	PermAnyone      Permission = "any"
)

// function arguments
type CreateProjectArgs struct {
	Name          string            `json:"name"`
	Description   string            `json:"desc"`
	JsonMetadata  map[string]string `json:"meta,omitempty"`
	ProjectConfig string            `json:"cfg"`
}

type JoinProjectArgs struct {
	ProjectID int64 `json:"id"`
}

type AddFundsArgs struct {
	ProjectID int64 `json:"id"`
}

type TransferOwnershipArgs struct {
	ProjectID int64       `json:"id"`
	NewOwner  sdk.Address `json:"newOwner"`
}

type LeaveProjectArgs struct {
	ProjectID int64 `json:"projectID"`
}

// Role constants
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

//go:wasmexport projects_create
func CreateProject(payload *string) *string {

	input := FromJSON[CreateProjectArgs](*payload, "CreateProjectArgs")
	env := sdk.GetEnv()
	ta := getFirstTransferAllow(env.Intents)
	if ta == nil {
		sdk.Abort("no intents set")
	}
	if ta.Token == sdk.AssetHive && ta.Limit < prjCreationMinHIVE {
		sdk.Abort(fmt.Sprintf("project stake must be at least %f HIVE", prjCreationMinHIVE))
	}
	if ta.Token == sdk.AssetHbd && ta.Limit < prjCreationMinHBD {
		sdk.Abort(fmt.Sprintf("project stake must be at least %f HBD", prjCreationMinHBD))
	}
	// Parse JSON params
	cfg := FromJSON[ProjectConfig](input.ProjectConfig, "ProjectConfig")

	creator := getSenderAddress()

	mTaLimit := int64(ta.Limit * 1000)
	sdk.HiveDraw(mTaLimit, sdk.Asset(ta.Token)) // TODO: first check balance of calle

	id := getCount(ProjectsCount)
	now := nowUnix()

	prj := Project{
		ID:           id,
		Owner:        creator,
		Name:         input.Name,
		Description:  input.Description,
		JsonMetadata: input.JsonMetadata,
		Config:       *cfg,
		Members:      map[sdk.Address]Member{},
		Funds:        ta.Limit,
		FundsAsset:   sdk.Asset(ta.Token),
		CreationTxID: getTxID(),
		Paused:       false,
	}
	// Add creator as admin
	m := Member{
		Address:      creator,
		Stake:        1,
		Role:         RoleAdmin,
		JoinedAt:     now,
		LastActionAt: now,
		Reputation:   0,
	}
	// If it is stake-based, add full stake
	if cfg.VotingSystem == SystemStake {
		m.Stake = ta.Limit
	}
	prj.Members[creator] = m
	saveProject(&prj)
	AddIDToIndex(idxProjects, id)
	setCount(ProjectsCount, id+1)
	return strptr(fmt.Sprintf("project created: %d", id))

}

// GetProject - returns the project object as JSON
//
//go:wasmexport projects_get_one
func GetProject(projectID *int64) *string {
	prj := loadProject(*projectID)
	prjJson := ToJSON(prj, "loaded project")
	return strptr(prjJson)
}

// GetAllProjects - returns all projects as JSON array
//
//go:wasmexport projects_get_all
func GetAllProjects() *string {
	ids := GetIDsFromIndex(idxProjects)
	projects := make([]*Project, 0, len(ids))
	for _, id := range ids {
		prj := loadProject(id)
		projects = append(projects, prj)
	}
	projectsJson := ToJSON(projects, "loaded project")
	return strptr(projectsJson)

}

// AddFunds - draw funds from caller and add to project's treasury pool
// If the project is a stake based system & the sender is a valid mamber then the stake of the member will get updated accordingly.
// hive assets are always handled x 1000 => 1 HIVE = 1000

func validatetransferAllow(ta *TransferAllow, projectAsset sdk.Asset) {
	if ta == nil {
		sdk.Abort("invalid intent")
	}
	if ta.Limit <= 0 {
		sdk.Abort("limit needs to be > 0")
	}
	if ta.Token != projectAsset {
		sdk.Abort(fmt.Sprintf("only %s is allowed", projectAsset.String()))
	}
}

//
//go:wasmexport projects_add_funds
func AddFunds(payload *string) *string {
	input := FromJSON[AddFundsArgs](*payload, "AddFundsArgs")
	prj := loadProject(input.ProjectID)

	ta := getFirstTransferAllow(sdk.GetEnv().Intents)
	validatetransferAllow(ta, prj.FundsAsset)
	sender := getSenderAddress()

	mTaLimit := int64(ta.Limit * 1000)
	sdk.HiveDraw(mTaLimit, ta.Token)
	prj.Funds += ta.Limit

	// if stake based
	if prj.Config.VotingSystem == SystemStake {
		// check if member
		m, ismember := prj.Members[sender]
		if ismember {
			now := nowUnix()
			m.Stake = m.Stake + ta.Limit
			m.LastActionAt = now
			// add member with exact stake
			prj.Members[sender] = m
		}
	}

	saveProject(prj)
	return strptr("fund added")
}

//go:wasmexport projects_join
func JoinProject(ProjectID *int64) *string {

	prj := loadProject(*ProjectID)

	if prj.Paused {
		sdk.Abort("project is paused")
	}
	ta := getFirstTransferAllow(sdk.GetEnv().Intents)
	validatetransferAllow(ta, prj.FundsAsset)

	if ta.Limit <= 0 {
		sdk.Abort("amount needs to be > 0")
	}

	if ta.Token != prj.FundsAsset {
		sdk.Abort("asset needs to be " + prj.FundsAsset.String())
	}
	mTaLimit := int64(ta.Limit * 1000)

	now := nowUnix()
	sender := getSenderAddress()
	if prj.Config.VotingSystem == SystemDemocratic {
		if ta.Limit != prj.Config.DemocraticExactAmt {
			sdk.Abort(fmt.Sprintf("democratic projects need an exact amount to join: %d %s", prj.Config.DemocraticExactAmt, prj.FundsAsset.String()))
		}
		// transfer funds into contract
		sdk.HiveDraw(mTaLimit, ta.Token)

		// add member with stake 1
		prj.Members[sender] = Member{
			Address:      sender,
			Stake:        1,
			Role:         RoleMember,
			JoinedAt:     now,
			LastActionAt: now,
			Reputation:   0,
		}
		prj.Funds += ta.Limit
	} else { // if the project is a stake based system
		if ta.Limit < prj.Config.StakeMinAmt {
			sdk.Abort(fmt.Sprintf("JoinProject: the sent amount < than the minimum projects entry fee: %d %s", prj.Config.StakeMinAmt, prj.FundsAsset.String()))
		}

		_, ok := prj.Members[sender]
		if ok {
			sdk.Abort("already member")
		} else {
			// transfer funds into contract
			sdk.HiveDraw(mTaLimit, ta.Token)
			// add member with exact stake
			prj.Members[sender] = Member{
				Address:      sender,
				Stake:        ta.Limit,
				Role:         RoleMember,
				JoinedAt:     now,
				LastActionAt: now,
			}
		}
		prj.Funds += ta.Limit
	}
	saveProject(prj)
	AddIDToIndex(idxMembers+sender.String(), *ProjectID)

	return strptr(sender.String() + " joined")
}

//go:wasmexport projects_leave
func LeaveProject(ProjectID *int64) *string {

	caller := getSenderAddress()
	prj := loadProject(*ProjectID)
	if prj.Paused {
		sdk.Abort("project paused")
	}
	if prj.Owner == caller {
		sdk.Abort("project owner can not leave")
	}

	member, ok := prj.Members[caller]
	if !ok {
		sdk.Abort("not a member")
	}

	// TODO: check if no active vote

	now := nowUnix()

	if member.ExitRequested == 0 {
		// set exit requested timestamp
		member.ExitRequested = now
		prj.Members[caller] = member
		saveProject(prj)
		return strptr("exit requested") //TODO: add cooldown info
	} else {
		// if exit requested previously -> try to withdraw
		if now-member.ExitRequested < prj.Config.LeaveCooldownSecs {
			sdk.Abort("cooldown not passed")
		}
		// withdraw stake (for stake-based) or refund democratic amount
		if prj.Config.VotingSystem == SystemDemocratic {
			refund := prj.Config.DemocraticExactAmt
			if prj.Funds < refund {
				sdk.Abort("insufficient project funds")
			}
			prj.Funds -= refund
			// transfer back to caller
			mRefund := int64(refund * 1000)
			sdk.HiveTransfer(sdk.Address(caller), mRefund, prj.FundsAsset)
			delete(prj.Members, caller)
			projectIds := GetIDsFromIndex(idxProjects)
			// remove votes
			for _, pid := range projectIds {
				removeVote(input.ProjectID, pid, caller) // TODO: should "deactivate" votes - not remove them
			}
			saveProject(prj)
			return strptr("democratic refunded")
		}
		// stake-based
		withdraw := member.Stake
		if withdraw <= 0 { // should never happen(?)
			sdk.Log("LeaveProject: nothing to withdraw")
		}
		if prj.Funds < withdraw {
			sdk.Abort("insufficient project funds")
		}
		prj.Funds -= withdraw
		mWithdraw := int64(withdraw * 1000)
		sdk.HiveTransfer(sdk.Address(caller), mWithdraw, prj.FundsAsset)
		delete(prj.Members, caller)
		openProposalsIds := GetIDsFromIndex(idxProjectProposalsOpen + prj.ID)
		for _, pid := range openProposalsIds {
			removeVote(input.ProjectID, pid, caller)
		}
		saveProject(prj)
		// remove project from member index
		RemoveIDFromIndex(idxMembers+caller.String(), *ProjectID)

		sdk.Log("LeaveProject: withdrew stake")
		return strptr("project left - stake refunded")
	}
}

//go:wasmexport projects_transfer_ownership
func TransferProjectOwnership(payload *string) *string {
	input := FromJSON[TransferOwnershipArgs](*payload, "TransferOwnershipArgs")
	sender := getSenderAddress()
	prj := loadProject(input.ProjectID)
	if sender != prj.Owner {
		sdk.Abort(sender.String() + " is not owner of the project")
	}
	prj.Owner = input.NewOwner
	// ensure new owner exists as member
	_, ok := prj.Members[input.NewOwner]
	if !ok {
		sdk.Abort("new owner needs to be a member")
	}

	saveProject(prj)
	return strptr(input.NewOwner.String() + " is te new owner")
}

// TODO: use payload instead of arguments
//
//go:wasmexport projects_pause
func EmergencyPauseImmediate(projectID int64, pause bool) *string {
	caller := getSenderAddress()
	prj := loadProject(projectID)
	if caller != prj.Owner {
		sdk.Abort("only the project owner can pause / unpause without dedicated meta proposal")
	}
	prj.Paused = pause
	saveProject(prj)
	return strptr("pause switched to " + strconv.FormatBool(pause))
}

func saveProject(pro *Project) {
	key := projectKey(pro.ID)
	b := ToJSON(pro, "Project")
	sdk.StateSetObject(key, string(b))
}

func loadProject(id int64) *Project {
	key := projectKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("loading failed for project")
	}
	pro := FromJSON[Project](*ptr, "Project")
	return pro
}

func allProjectIDs() []string {
	ptr := sdk.StateGetObject(idxProjects)
	if ptr == nil {
		return []string{}
	}
	var ids []string
	if err := json.Unmarshal([]byte(*ptr), &ids); err != nil {
		return []string{}
	}
	return ids
}
