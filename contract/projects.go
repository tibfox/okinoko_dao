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
	DemocraticExactAmt   int64        `json:"demAmount"`      // exact amount required to join democratic
	StakeMinAmt          int64        `json:"stateMinAmount"` // min stake for stake-based joining
	ProposalCost         int64        `json:"propCost"`       // fee for creating proposals (goes to project funds)
	EnableSnapshot       bool         `json:"snapshot"`       // snapshot member stakes at proposal start
	RewardEnabled        bool         `json:"reward"`         // rewards enabled
	RewardAmount         int64        `json:"rewardAmount"`   // reward for proposer (from funds)
}

// Project - stored under project:<id>
type Project struct {
	ID           string                 `json:"id"`
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
	ProjectID string `json:"id"`
}

type AddFundsArgs struct {
	ProjectID string `json:"id"`
}

type TransferOwnershipArgs struct {
	ProjectID string      `json:"id"`
	NewOwner  sdk.Address `json:"newOwner"`
}

type LeaveProjectArgs struct {
	ProjectID string `json:"projectID"`
}

// Role constants
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

//go:wasmexport projects_create
func CreateProject(payload *string) *string {

	input, err := FromJSON[CreateProjectArgs](*payload)
	abortOnError(err, "invalid project args")
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
	cfg, err := FromJSON[ProjectConfig](input.ProjectConfig)
	abortOnError(err, "invalid project config")
	if input.Asset != sdk.AssetHive.String() && input.Asset != sdk.AssetHbd.String() {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": input.Asset + " is an invalid asset",
			},
		)

	}
	creator := getSenderAddress()

	sdk.HiveDraw(input.Amount, sdk.Asset(input.Asset)) // TODO: first check balance of calle

	id := generateGUID()
	now := nowUnix()

	prj := Project{
		ID:           id,
		Owner:        creator,
		Name:         input.Name,
		Description:  input.Description,
		JsonMetadata: input.JsonMetadata,
		Config:       *cfg,
		Members:      map[sdk.Address]Member{},
		Funds:        input.Amount,
		FundsAsset:   sdk.Asset(input.Asset),
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
		m.Stake = input.Amount
	}
	prj.Members[creator] = m

	saveProject(&prj)
	AddIDToIndex(idxProjects, id)

	sdk.Log("CreateProject: " + id)

	return returnJsonResponse(
		true, map[string]interface{}{
			"id": id,
		},
	)

}

// GetProject - returns the project object as JSON
//
//go:wasmexport projects_get_one
func GetProject(projectID string) *string {

	prj := loadProject(projectID)

	return returnJsonResponse(
		true, map[string]interface{}{
			"project": prj,
		},
	)
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
	return returnJsonResponse(
		true, map[string]interface{}{
			"projects": projects,
		},
	)
}

// AddFunds - draw funds from caller and add to project's treasury pool
// If the project is a stake based system & the sender is a valid mamber then the stake of the member will get updated accordingly.
// hive assets are always handled x 1000 => 1 HIVE = 1000

func validatetransferAllow(t *TransferAllow, projectAsset sdk.Asset) {
	if t == nil {
		abortCustom("invalid intent")
	}
	if t.Limit <= 0 {
		abortCustom("limit needs to be > 0")
	}
	if t.Token != projectAsset {
		abortCustom(fmt.Sprintf("only %s is allowed", projectAsset.String()))
	}
}

//
//go:wasmexport projects_add_funds
func AddFunds(payload *string) *string {
	input, err := FromJSON[AddFundsArgs](*payload)
	abortOnError(err, "invalid args")
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
	return returnJsonResponse(
		true, map[string]interface{}{
			"added": ta.Limit,
			"asset": prj.FundsAsset.String(),
		},
	)
}

//go:wasmexport projects_join
func JoinProject(payload *string) *string {
	input, err := FromJSON[JoinProjectArgs](*payload)
	abortOnError(err, "invalid args")

	prj := loadProject(input.ProjectID)
	if prj.Paused {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "project is paused",
			},
		)
	}
	ta := getFirstTransferAllow(sdk.GetEnv().Intents)
	validatetransferAllow(ta, prj.FundsAsset)

	if ta.Limit <= 0 {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "amount needs to be > 0",
			},
		)
	}
	if ta.Token != prj.FundsAsset {
		sdk.Log(fmt.Sprintf("JoinProject: asset must match the project main asset: %s", prj.FundsAsset.String()))
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "asset needs to be " + prj.FundsAsset.String(),
			},
		)
	}

	now := nowUnix()
	if prj.Config.VotingSystem == SystemDemocratic {
		if ta.Limit != prj.Config.DemocraticExactAmt {
			abortCustom(fmt.Sprintf("democratic projects need an exact amount to join: %d %s", prj.Config.DemocraticExactAmt, prj.FundsAsset.String()))
		}
		// transfer funds into contract
		sdk.HiveDraw(ta.Limit, ta.Token) // TODO: what if not enough funds?!

		// add member with stake 1
		sender := getSenderAddress()
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
			abortCustom(fmt.Sprintf("JoinProject: the sent amount < than the minimum projects entry fee: %d %s", prj.Config.StakeMinAmt, prj.FundsAsset.String()))
		}
		sender := getSenderAddress()
		_, ok := prj.Members[sender]
		if ok {
			abortCustom("already member")
		} else {
			// transfer funds into contract
			sdk.HiveDraw(ta.Limit, ta.Token)
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

	return returnJsonResponse(
		true, map[string]interface{}{
			"joined": getSenderAddress().String(),
		},
	)
}

//go:wasmexport projects_leave
func LeaveProject(payload *string) *string {
	input, err := FromJSON[LeaveProjectArgs](*payload)
	abortOnError(err, "invalid args")

	caller := getSenderAddress()

	prj := loadProject(input.ProjectID)
	if prj.Paused {
		sdk.Log("LeaveProject: project paused")
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "project is paused",
			},
		)
	}
	if prj.Owner == caller {
		sdk.Log("LeaveProject: project owner can not leave")
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "project owner can not leave the project",
			},
		)
	}
	member, ok := prj.Members[caller]
	if !ok {
		sdk.Log("LeaveProject: not a member")
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": caller + " is not a member",
			},
		)
	}

	now := nowUnix()

	if member.ExitRequested == 0 {
		// set exit requested timestamp
		member.ExitRequested = now
		prj.Members[caller] = member
		saveProject(prj)
		return returnJsonResponse(
			true, map[string]interface{}{
				"details": "exit requested.",
				//TODO: add cooldown info
			},
		)
	} else {
		// if exit requested previously -> try to withdraw
		if now-member.ExitRequested < prj.Config.LeaveCooldownSecs {
			sdk.Log("LeaveProject: cooldown not passed")
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "cooldown of " + caller + " is not passed yet",
					// TODO add remaining time
				},
			)
		}
		// withdraw stake (for stake-based) or refund democratic amount
		if prj.Config.VotingSystem == SystemDemocratic {
			refund := prj.Config.DemocraticExactAmt
			if prj.Funds < refund {
				sdk.Log("LeaveProject: insufficient project funds")
				return returnJsonResponse(
					false, map[string]interface{}{
						"details": "insufficient project funds",
					},
				)
			}
			prj.Funds -= refund
			// transfer back to caller
			sdk.HiveTransfer(sdk.Address(caller), refund, prj.FundsAsset)
			delete(prj.Members, caller)
			projectIds := GetIDsFromIndex(idxProjects)
			// remove votes
			for _, pid := range projectIds {
				removeVote(input.ProjectID, pid, caller) // TODO: should "deactivate" votes - not remove them
			}
			saveProject(prj)
			sdk.Log("LeaveProject: democratic refunded")
			return returnJsonResponse(
				true, map[string]interface{}{
					"details": "democratic refunded",
				},
			)
		}
		// stake-based
		withdraw := member.Stake
		if withdraw <= 0 { // should never happen(?)
			sdk.Log("LeaveProject: nothing to withdraw")
			// return returnJsonResponse(
			// 	"projects_leave", true, map[string]interface{}{
			// 		"details": "project left - nothing to withdraw",
			// 	},
			// )
		}
		if prj.Funds < withdraw {
			sdk.Log("LeaveProject: insufficient project funds")
			return returnJsonResponse(
				false, map[string]interface{}{
					"details": "insufficient project funds",
				},
			)
		}
		prj.Funds -= withdraw
		sdk.HiveTransfer(sdk.Address(caller), withdraw, prj.FundsAsset)
		delete(prj.Members, caller)
		openProposalsIds := GetIDsFromIndex(idxProjectProposalsOpen + prj.ID)
		for _, pid := range openProposalsIds {
			removeVote(input.ProjectID, pid, caller)
		}
		saveProject(prj)
		sdk.Log("LeaveProject: withdrew stake")
		return returnJsonResponse(
			true, map[string]interface{}{
				"details": "project left - stake refunded",
			},
		)
	}

}

//go:wasmexport projects_transfer_ownership
func TransferProjectOwnership(payload *string) *string {
	input, err := FromJSON[TransferOwnershipArgs](*payload)
	abortOnError(err, "invalid args")

	sender := getSenderAddress()

	prj := loadProject(input.ProjectID)

	if sender != prj.Owner {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": sender + " is not owner of the project",
			},
		)
	}
	prj.Owner = input.NewOwner
	// ensure new owner exists as member
	if _, ok := prj.Members[input.NewOwner]; !ok {
		prj.Members[input.NewOwner] = Member{
			Address:      input.NewOwner,
			Stake:        0,
			Role:         RoleMember,
			JoinedAt:     nowUnix(),
			LastActionAt: nowUnix(),
		}
	}
	saveProject(prj)

	return returnJsonResponse(
		true, map[string]interface{}{
			"to": input.NewOwner,
		},
	)

}

//go:wasmexport projects_pause
func EmergencyPauseImmediate(projectID string, pause bool) *string {

	caller := getSenderAddress()
	prj := loadProject(projectID)

	if caller != prj.Owner {
		return returnJsonResponse(
			false, map[string]interface{}{
				"details": "only the project owner can pause / unpause without dedicated meta proposal",
			},
		)

	}
	prj.Paused = pause
	saveProject(prj)
	sdk.Log("EmergencyPauseImmediate: set paused=" + strconv.FormatBool(pause))
	return returnJsonResponse(
		true, map[string]interface{}{
			"details": "pause switched",
			"value":   pause,
		},
	)
}

func saveProject(pro *Project) {
	key := projectKey(pro.ID)
	b, err := json.Marshal(pro)
	abortOnError(err, "failed to marshal")
	sdk.StateSetObject(key, string(b))
}

func loadProject(id string) *Project {
	key := projectKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil {
		abortCustom("loading failed for project")
	}
	var pro Project
	if err := json.Unmarshal([]byte(*ptr), &pro); err != nil {
		abortCustom(fmt.Sprintf("failed unmarshal project %s: %v", id, err))
	}
	return &pro
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
