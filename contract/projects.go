package contract

import (
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
)

//go:wasmexport projects_create
func CreateProject(payload string) *string {
	args, err := ParseJSONFunctionArgs[CreateProjectArgs](payload)
	if err != nil {
		return returnJsonResponse(
			"projects_create", false, map[string]interface{}{
				"details": "arguments do not match",
			},
		)
	}

	if args.Amount <= 0 {
		sdkInterface.Log("CreateProject: amount must be > 1")
		return returnJsonResponse(
			"projects_create", false, map[string]interface{}{
				"details": "amount must be > 1",
			},
		)

	}

	// Parse JSON params
	cfg, err := ProjectConfigFromJSON(args.ProjectConfig)
	if err != nil {
		return returnJsonResponse(
			"projects_create", false, map[string]interface{}{
				"details": "invalid project config\n" + err.Error(),
			},
		)

	}
	if args.Asset != sdk.AssetHive.String() && args.Asset != sdk.AssetHbd.String() {
		return returnJsonResponse(
			"projects_create", false, map[string]interface{}{
				"details": args.Asset + " is an invalid asset",
			},
		)

	}

	state := getState()
	creator := getSenderAddress()

	sdk.HiveDraw(args.Amount, sdk.Asset(args.Asset)) // TODO: first check balance of calle

	id := generateGUID()
	now := nowUnix()

	prj := Project{
		ID:           id,
		Owner:        creator,
		Name:         args.Name,
		Description:  args.Description,
		JsonMetadata: args.JsonMetadata,
		Config:       *cfg,
		Members:      map[string]Member{},
		Funds:        args.Amount,
		FundsAsset:   sdk.Asset(args.Asset),
		CreatedAt:    now,
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
		m.Stake = args.Amount
	}
	prj.Members[creator] = m

	saveProject(state, &prj)
	addProjectToIndex(state, id)

	sdkInterface.Log("CreateProject: " + id)

	return returnJsonResponse(
		"projects_create", true, map[string]interface{}{
			"id": id,
		},
	)

}

// GetProject - returns the project object as JSON
//
//go:wasmexport projects_get_one
func GetProject(projectID string) *string {
	state := getState()
	prj, err := loadProject(state, projectID)
	if err != nil {
		return returnJsonResponse(
			"projects_get_one", false, map[string]interface{}{
				"details": "project not found",
			},
		)
	}
	return returnJsonResponse(
		"projects_get_one", true, map[string]interface{}{
			"project": prj,
		},
	)
}

// GetAllProjects - returns all projects as JSON array
//
//go:wasmexport projects_get_all
func GetAllProjects() *string {
	state := getState()
	ids := listAllProjectIDs(state)
	projects := make([]*Project, 0, len(ids))
	for _, id := range ids {
		if prj, err := loadProject(state, id); err == nil {
			projects = append(projects, prj)
		}
	}
	return returnJsonResponse(
		"projects_get_all", true, map[string]interface{}{
			"projects": projects,
		},
	)
}

// AddFunds - draw funds from caller and add to project's treasury pool
// If the project is a stake based system & the sender is a valid mamber then the stake of the member will get updated accordingly.
// hive assets are always handled x 1000 => 1 HIVE = 1000
//
//go:wasmexport projects_add_funds
func AddFunds(payload string) *string {
	args, err := ParseJSONFunctionArgs[AddFundsArgs](payload)
	if err != nil {
		return returnJsonResponse(
			"projects_add_funds", false, map[string]interface{}{
				"details": "arguments do not match",
			},
		)
	}
	state := getState()
	if args.Amount <= 0 {
		return returnJsonResponse(
			"projects_add_funds", false, map[string]interface{}{
				"projects": "amount needs to be > 0",
			},
		)
	}
	prj, err := loadProject(state, args.ProjectID)
	if err != nil {
		return returnJsonResponse(
			"projects_add_funds", false, map[string]interface{}{
				"details": "project not found",
			},
		)
	}
	if args.Asset != prj.FundsAsset.String() {
		return returnJsonResponse(
			"projects_add_funds", false, map[string]interface{}{
				"details": "asset needs to be " + prj.FundsAsset.String(),
			},
		)
	}
	caller := getSenderAddress()

	sdk.HiveDraw(args.Amount, sdk.Asset(args.Asset))
	prj.Funds += args.Amount

	// if stake based
	if prj.Config.VotingSystem == SystemStake {
		// check if member
		m, ismember := prj.Members[caller]
		if ismember {
			now := nowUnix()
			m.Stake = m.Stake + args.Amount
			m.LastActionAt = now
			// add member with exact stake
			prj.Members[caller] = m
		}
	}

	saveProject(state, prj)
	sdkInterface.Log("AddFunds: added " + strconv.FormatInt(args.Amount, 10))
	return returnJsonResponse(
		"projects_add_funds", true, map[string]interface{}{
			"added": args.Amount,
			"asset": prj.FundsAsset.String(),
		},
	)
}

//go:wasmexport projects_join
func JoinProject(payload string) *string {
	args, err := ParseJSONFunctionArgs[JoinProjectArgs](payload)
	if err != nil {
		return returnJsonResponse(
			"projects_join", false, map[string]interface{}{
				"details": "arguments do not match",
			},
		)
	}
	state := getState()
	caller := getSenderAddress()

	prj, err := loadProject(state, args.ProjectID)
	asset := sdk.Asset(args.Asset)

	if err != nil {
		return returnJsonResponse(
			"projects_join", false, map[string]interface{}{
				"details": "project not found",
			},
		)
	}
	if prj.Paused {
		return returnJsonResponse(
			"projects_join", false, map[string]interface{}{
				"details": "project is paused",
			},
		)
	}
	if args.Amount <= 0 {
		return returnJsonResponse(
			"projects_join", false, map[string]interface{}{
				"details": "amount needs to be > 0",
			},
		)
	}
	if asset != prj.FundsAsset {
		sdkInterface.Log(fmt.Sprintf("JoinProject: asset must match the project main asset: %s", prj.FundsAsset.String()))
		return returnJsonResponse(
			"projects_join", false, map[string]interface{}{
				"details": "asset needs to be " + prj.FundsAsset.String(),
			},
		)
	}

	now := nowUnix()
	if prj.Config.VotingSystem == SystemDemocratic {
		if args.Amount != prj.Config.DemocraticExactAmt {
			sdkInterface.Log(fmt.Sprintf("JoinProject: democratic projects need an exact amount to join: %d %s", prj.Config.DemocraticExactAmt, prj.FundsAsset.String()))
			return returnJsonResponse(
				"projects_join", false, map[string]interface{}{
					"details": fmt.Sprintf("democratic projects need an exact amount to join: %d %s", prj.Config.DemocraticExactAmt, prj.FundsAsset.String()),
				},
			)
		}
		// transfer funds into contract
		sdk.HiveDraw(args.Amount, sdk.Asset(asset)) // TODO: what if not enough funds?!

		// add member with stake 1
		prj.Members[caller] = Member{
			Address:      caller,
			Stake:        1,
			Role:         RoleMember,
			JoinedAt:     now,
			LastActionAt: now,
			Reputation:   0,
		}
		prj.Funds += args.Amount
	} else { // if the project is a stake based system
		if args.Amount < prj.Config.StakeMinAmt {
			sdkInterface.Log(fmt.Sprintf("JoinProject: the sent amount < than the minimum projects entry fee: %d %s", prj.Config.StakeMinAmt, prj.FundsAsset.String()))

			return returnJsonResponse(
				"projects_join", false, map[string]interface{}{
					"details": fmt.Sprintf("the sent amount < than the minimum projects entry fee: %d %s", prj.Config.StakeMinAmt, prj.FundsAsset.String()),
				},
			)
		}
		_, ok := prj.Members[caller]
		if ok {
			sdkInterface.Log("JoinProject: already member")
			return returnJsonResponse(
				"projects_join", false, map[string]interface{}{
					"details": "already member",
				},
			)
		} else {
			// transfer funds into contract
			sdk.HiveDraw(args.Amount, sdk.Asset(asset)) // TODO: what if not enough funds?!
			// add member with exact stake
			prj.Members[caller] = Member{
				Address:      caller,
				Stake:        args.Amount,
				Role:         RoleMember,
				JoinedAt:     now,
				LastActionAt: now,
			}
		}
		prj.Funds += args.Amount
	}
	saveProject(state, prj)
	sdkInterface.Log("JoinProject: " + args.ProjectID + " by " + caller)
	return returnJsonResponse(
		"projects_join", true, map[string]interface{}{
			"joined": caller,
		},
	)
}

//go:wasmexport projects_leave
func LeaveProject(payload string) *string {
	args, err := ParseJSONFunctionArgs[LeaveProjectArgs](payload)
	if err != nil {
		return returnJsonResponse(
			"projects_leave", false, map[string]interface{}{
				"details": "arguments do not match",
			},
		)
	}
	state := getState()
	caller := getSenderAddress()

	prj, err := loadProject(state, args.ProjectID)
	if err != nil {
		return returnJsonResponse(
			"projects_leave", false, map[string]interface{}{
				"details": "project not found",
			},
		)
	}
	if prj.Paused {
		sdkInterface.Log("LeaveProject: project paused")
		return returnJsonResponse(
			"projects_leave", false, map[string]interface{}{
				"details": "project is paused",
			},
		)
	}
	if prj.Owner == caller {
		sdkInterface.Log("LeaveProject: project owner can not leave")
		return returnJsonResponse(
			"projects_leave", false, map[string]interface{}{
				"details": "project owner can not leave the project",
			},
		)
	}
	member, ok := prj.Members[caller]
	if !ok {
		sdkInterface.Log("LeaveProject: not a member")
		return returnJsonResponse(
			"projects_leave", false, map[string]interface{}{
				"details": caller + " is not a member",
			},
		)
	}

	now := nowUnix()
	// if exit requested previously -> try to withdraw
	if member.ExitRequested > 0 {
		if now-member.ExitRequested < prj.Config.LeaveCooldownSecs {
			sdkInterface.Log("LeaveProject: cooldown not passed")
			return returnJsonResponse(
				"projects_leave", false, map[string]interface{}{
					"details": "cooldown of " + caller + " is not passed yet",
					// TODO add remaining time
				},
			)
		}
		// withdraw stake (for stake-based) or refund democratic amount
		if prj.Config.VotingSystem == SystemDemocratic {
			refund := prj.Config.DemocraticExactAmt
			if prj.Funds < refund {
				sdkInterface.Log("LeaveProject: insufficient project funds")
				return returnJsonResponse(
					"projects_leave", false, map[string]interface{}{
						"details": "insufficient project funds",
					},
				)
			}
			prj.Funds -= refund
			// transfer back to caller
			sdk.HiveTransfer(sdk.Address(caller), refund, prj.FundsAsset)
			delete(prj.Members, caller)
			// remove votes
			for _, pid := range listProposalIDsForProject(state, args.ProjectID) {
				removeVote(state, args.ProjectID, pid, caller) // TODO: should "deactivate" votes - not remove them
			}
			saveProject(state, prj)
			sdkInterface.Log("LeaveProject: democratic refunded")
			return returnJsonResponse(
				"projects_leave", true, map[string]interface{}{
					"details": "democratic refunded",
				},
			)
		}
		// stake-based
		withdraw := member.Stake
		if withdraw <= 0 { // should never happen(?)
			sdkInterface.Log("LeaveProject: nothing to withdraw")
			// return returnJsonResponse(
			// 	"projects_leave", true, map[string]interface{}{
			// 		"details": "project left - nothing to withdraw",
			// 	},
			// )
		}
		if prj.Funds < withdraw {
			sdkInterface.Log("LeaveProject: insufficient project funds")
			return returnJsonResponse(
				"projects_leave", false, map[string]interface{}{
					"details": "insufficient project funds",
				},
			)
		}
		prj.Funds -= withdraw
		sdk.HiveTransfer(sdk.Address(caller), withdraw, prj.FundsAsset)
		delete(prj.Members, caller)
		for _, pid := range listProposalIDsForProject(state, args.ProjectID) {
			removeVote(state, args.ProjectID, pid, caller)
		}
		saveProject(state, prj)
		sdkInterface.Log("LeaveProject: withdrew stake")
		return returnJsonResponse(
			"projects_leave", true, map[string]interface{}{
				"details": "project left - stake refunded",
			},
		)
	}

	// otherwise set exit requested timestamp
	member.ExitRequested = now
	prj.Members[caller] = member
	saveProject(state, prj)
	return returnJsonResponse(
		"projects_leave", true, map[string]interface{}{
			"details": "exit requested.",
			//TODO: add cooldown info
		},
	)
}

//go:wasmexport projects_transfer_ownership
func TransferProjectOwnership(payload string) *string {
	args, err := ParseJSONFunctionArgs[TransferOwnershipArgs](payload)
	if err != nil {
		return returnJsonResponse(
			"projects_transfer_ownership", false, map[string]interface{}{
				"details": "arguments do not match",
			},
		)
	}
	state := getState()
	caller := getSenderAddress()

	prj, err := loadProject(state, args.ProjectID)
	if err != nil {
		return returnJsonResponse(
			"projects_transfer_ownership", false, map[string]interface{}{
				"details": "project not found",
			},
		)
	}
	if caller != prj.Owner {
		return returnJsonResponse(
			"projects_transfer_ownership", false, map[string]interface{}{
				"details": caller + " is not owner of the project",
			},
		)
	}
	prj.Owner = args.NewOwner
	// ensure new owner exists as member
	if _, ok := prj.Members[args.NewOwner]; !ok {
		prj.Members[args.NewOwner] = Member{
			Address:      args.NewOwner,
			Stake:        0,
			Role:         RoleMember,
			JoinedAt:     nowUnix(),
			LastActionAt: nowUnix(),
		}
	}
	saveProject(state, prj)
	sdkInterface.Log("TransferProjectOwnership: " + args.ProjectID + " -> " + args.NewOwner)
	return returnJsonResponse(
		"projects_transfer_ownership", true, map[string]interface{}{
			"to": args.NewOwner,
		},
	)

}

//go:wasmexport projects_pause
func EmergencyPauseImmediate(projectID string, pause bool) *string {
	state := getState()
	caller := getSenderAddress()
	prj, err := loadProject(state, projectID)
	if err != nil {
		return returnJsonResponse(
			"projects_pause", false, map[string]interface{}{
				"details": "project not found",
			},
		)
	}
	if caller != prj.Owner {
		return returnJsonResponse(
			"projects_pause", false, map[string]interface{}{
				"details": "only the project owner can pause / unpause without dedicated meta proposal",
			},
		)

	}
	prj.Paused = pause
	saveProject(state, prj)
	sdkInterface.Log("EmergencyPauseImmediate: set paused=" + strconv.FormatBool(pause))
	return returnJsonResponse(
		"projects_pause", true, map[string]interface{}{
			"details": "pause switched",
			"value":   pause,
		},
	)
}
