package main

import (
	"fmt"
	"okinoko_dao/contract/dao"
	"okinoko_dao/sdk"
	"strconv"
	"strings"
)

const (
	FallbackNFTContract                 = "TODO: set default nft contract"
	FallbackNFTFunction                 = "nft_hasNFTEdition"
	FallbackThresholdPercent            = 50.001
	FallbackQuorumPercent               = 50.001
	FallbackProposalDurationHours       = 24
	FallbackExecutionDelayHours         = 4
	FallbackLeaveCooldownHours          = 24
	FallbackProposalCost                = 1
	FallbackProposalCreatorsMembersOnly = true
	FallbackMembershipPayloadFormat     = "{nft}|{caller}"
)

// Permission defines access rights for actions within a project.
type Permission string

const (
	// PermCreatorOnly restricts an action to the project creator/owner.
	PermCreatorOnly Permission = "creator"

	// PermAnyMember allows any project member to perform the action.
	PermAnyMember Permission = "member"
)

// -----------------------------------------------------------------------------
// Project operations
// -----------------------------------------------------------------------------

// CreateProject bootstraps a DAO, pulling stake + treasury in one wonky transfer.allow parsing.
// Example payload: CreateProject(strptr("My DAO|desc|..."))
//
//go:wasmexport project_create
func CreateProject(payload *string) *string {
	input := decodeCreateProjectArgs(payload)

	caller := getSenderAddress()
	callerAddr := dao.Address(caller)
	callerStr := caller.String()

	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow()
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}
	tokenStr := ta.Token.String()

	// determine required initial stake
	stakeLimit := ta.Limit
	stakeMin := stakeLimit
	if input.ProjectConfig.StakeMinAmt > 0 {
		stakeMin = input.ProjectConfig.StakeMinAmt
		if stakeMin > stakeLimit {
			sdk.Abort(fmt.Sprintf("transfer limit %f < StakeMinAmt %f", stakeLimit, stakeMin))
		}
	} else {
		input.ProjectConfig.StakeMinAmt = stakeLimit
	}
	initialTrasurey := stakeLimit - stakeMin

	// --- create project ---
	id := getCount(ProjectsCount)
	now := nowUnix()
	txPtr := sdk.GetEnvKey("tx.id")
	txID := ""
	if txPtr != nil {
		txID = *txPtr
	}

	stakeAmount := dao.FloatToAmount(ta.Limit - initialTrasurey)
	treasuryAmount := dao.FloatToAmount(initialTrasurey)
	depositAmount := stakeAmount + treasuryAmount

	prj := dao.Project{
		ID:          id,
		Owner:       callerAddr,
		Name:        input.Name,
		Description: input.Description,
		Metadata:    input.Metadata,
		Funds:       treasuryAmount,
		FundsAsset:  dao.Asset(ta.Token),
		Paused:      false,
		Tx:          txID,
		StakeTotal:  stakeAmount,
		MemberCount: 1,
	}
	prj.Config = input.ProjectConfig

	creatorMember := dao.Member{
		Address:      callerAddr,
		Stake:        stakeAmount,
		JoinedAt:     now,
		LastActionAt: now,
		Reputation:   0,
	}
	// draw the funds
	mAmount := dao.AmountToInt64(depositAmount)
	sdk.HiveDraw(mAmount, ta.Token)
	saveMember(prj.ID, &creatorMember)
	// save project
	saveProject(&prj)
	setCount(ProjectsCount, id+1)

	emitProjectCreatedEvent(id, callerStr)
	stakeAmountFloat := dao.AmountToFloat(stakeAmount)
	treasuryAmountFloat := dao.AmountToFloat(treasuryAmount)
	emitFundsAdded(prj.ID, callerStr, stakeAmountFloat, tokenStr, true)
	emitFundsAdded(prj.ID, callerStr, treasuryAmountFloat, tokenStr, false)
	result := strconv.FormatUint(id, 10)
	return &result
}

// JoinProject lets someone stake into a DAO, optionally proving NFT membership before funds move.
// Example payload: JoinProject(strptr("123"))
//
//go:wasmexport project_join
func JoinProject(projectID *string) *string {
	rawID := unwrapPayload(projectID, "project ID is required")
	id, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil {
		sdk.Abort("invalid project ID")
	}
	prj := loadProject(id)
	if prj.Paused {
		sdk.Abort("project paused")
	}
	caller := getSenderAddress()
	callerAddr := dao.AddressFromString(caller.String())

	if _, exists := loadMember(prj.ID, callerAddr); exists {
		sdk.Abort("already a member")
	}

	if prj.Config.MembershipNFT != nil {
		// check if caller is owner of any edition of the membership nft
		// GetNFTOwnedEditionsArgs specifies the arguments to query editions owned by an address.

		nftContract := prj.Config.MembershipNFTContract
		if nftContract == nil {
			nftContract = strptr(FallbackNFTContract)
		}

		nftFunction := prj.Config.MembershipNFTContractFunction
		if nftFunction == nil {
			nftFunction = strptr(FallbackNFTFunction)
		}
		payload := formatMembershipPayload(prj.Config.MembershipNftPayloadFormat, UInt64ToString(*prj.Config.MembershipNFT), dao.AddressToString(callerAddr))

		editions := sdk.ContractCall(
			*nftContract,
			*nftFunction,
			payload,
			nil)
		if editions == nil || *editions == "[]" || *editions == "" {
			sdk.Abort("membership nft not owned")
		}
	}
	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow()
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}
	intentAsset := dao.AssetFromString(ta.Token.String())
	if intentAsset != prj.FundsAsset {
		sdk.Abort(fmt.Sprintf("invalid asset, expected %s", dao.AssetToString(prj.FundsAsset)))
	}

	if ta.Limit != prj.Config.StakeMinAmt && prj.Config.VotingSystem == dao.VotingSystemDemocratic {
		sdk.Abort(fmt.Sprintf("democratic projects require exactly %f %s", prj.Config.StakeMinAmt, ta.Token.String()))
	}

	if ta.Limit < prj.Config.StakeMinAmt && prj.Config.VotingSystem == dao.VotingSystemStake {
		sdk.Abort(fmt.Sprintf("stake too low, minimum %f %s required", prj.Config.StakeMinAmt, ta.Token.String()))
	}
	now := nowUnix()

	depositAmount := dao.FloatToAmount(ta.Limit)
	newMember := dao.Member{
		Address:      callerAddr,
		Stake:        depositAmount,
		JoinedAt:     now,
		LastActionAt: now,
	}
	saveMember(prj.ID, &newMember)
	prj.MemberCount++
	prj.StakeTotal += depositAmount
	// draw the funds
	mAmount := dao.AmountToInt64(depositAmount)
	sdk.HiveDraw(mAmount, ta.Token)
	saveProjectFinance(prj)
	emitJoinedEvent(prj.ID, dao.AddressToString(callerAddr))
	emitFundsAdded(prj.ID, dao.AddressToString(callerAddr), dao.AmountToFloat(depositAmount), ta.Token.String(), true)
	return strptr("joined")
}

// LeaveProject either schedules or finalizes an exit, blocking folks with active payout locks.
// Example payload: LeaveProject(strptr("123"))
//
//go:wasmexport project_leave
func LeaveProject(projectID *string) *string {
	raw := unwrapPayload(projectID, "project ID is required")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		sdk.Abort("invalid project ID")
	}
	caller := getSenderAddress()
	callerAddr := dao.AddressFromString(caller.String())
	prj := loadProject(id)
	if prj.Paused {
		sdk.Abort("project paused")
	}

	member := getMember(prj.ID, callerAddr)

	now := nowUnix()
	if hasActivePayout(prj.ID, callerAddr) {
		sdk.Abort("active proposal requesting funds")
	}
	if member.ExitRequested == 0 {
		member.ExitRequested = now
		saveMember(prj.ID, &member)
		return strptr("exit requested")
	}
	if now-member.ExitRequested < int64(prj.Config.LeaveCooldownHours*3600) {
		sdk.Abort("cooldown not passed")
	}

	// refund stake
	withdraw := member.Stake
	mAmount := dao.AmountToInt64(withdraw)
	sdk.HiveTransfer(caller, mAmount, sdk.Asset(dao.AssetToString(prj.FundsAsset)))

	deleteMember(prj.ID, callerAddr)
	if prj.MemberCount > 0 {
		prj.MemberCount--
	}
	prj.StakeTotal -= withdraw
	saveProjectFinance(prj)
	emitLeaveEvent(prj.ID, dao.AddressToString(callerAddr))
	emitFundsRemoved(prj.ID, dao.AddressToString(callerAddr), dao.AmountToFloat(withdraw), dao.AssetToString(prj.FundsAsset), true)
	return strptr("exit finished")
}

// hasActivePayout checks payout locks to avoid releasing stake while funds are still promised.
func hasActivePayout(projectID uint64, member dao.Address) bool {
	return getPayoutLockCount(projectID, member) > 0
}

// AddFunds handles the double-path deposit (treasury vs stake) and still validates asset + transfer.allow.
// Example payload: AddFunds(strptr("7|1"))
//
//go:wasmexport project_funds
func AddFunds(payload *string) *string {
	input := decodeAddFundsArgs(payload)
	prj := loadProject(input.ProjectID)
	caller := getSenderAddress()
	callerAddr := dao.AddressFromString(caller.String())
	var stakingMember *dao.Member

	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow()
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}

	// validate intent token matches project treasury asset
	asset := dao.AssetFromString(ta.Token.String())
	if asset != prj.FundsAsset {
		sdk.Abort(fmt.Sprintf("invalid asset, expected %s", dao.AssetToString(prj.FundsAsset)))
	}

	if input.ToStake {
		if prj.Config.VotingSystem == dao.VotingSystemDemocratic {
			sdk.Abort("cannot add member stake > StakeMinAmt in democratic systems")
		}
		member := getMember(prj.ID, callerAddr)
		stakingMember = &member
	}

	// draw the funds
	depositAmount := dao.FloatToAmount(ta.Limit)
	mAmount := dao.AmountToInt64(depositAmount)
	sdk.HiveDraw(mAmount, ta.Token)

	if input.ToStake {
		member := stakingMember
		member.Stake += depositAmount
		member.LastActionAt = nowUnix()
		saveMember(prj.ID, member)
		prj.StakeTotal += depositAmount
	} else {
		// add to treasury for payouts
		prj.Funds += depositAmount

	}

	saveProjectFinance(prj)
	emitFundsAdded(prj.ID, dao.AddressToString(callerAddr), dao.AmountToFloat(depositAmount), ta.Token.String(), input.ToStake)
	return strptr("funds added")
}

// getMember fetches the cached member or aborts with a readable adress on failure.
func getMember(projectID uint64, user dao.Address) dao.Member {
	member, ok := loadMember(projectID, user)
	if !ok {
		sdk.Abort(fmt.Sprintf("%s is not a member", dao.AddressToString(user)))
	}
	return *member
}

// TransferProjectOwnership lets the owner hand over control, but we enforce the target is still a member.
// Example payload: TransferProjectOwnership(strptr("5|hive:alice"))
//
//go:wasmexport project_transfer
func TransferProjectOwnership(payload *string) *string {
	raw := unwrapPayload(payload, "transfer payload required")
	parts := strings.Split(raw, "|")
	if len(parts) < 2 {
		sdk.Abort("transfer payload requires projectId|newOwner")
	}
	idStr := strings.TrimSpace(parts[0])
	if idStr == "" {
		sdk.Abort("project ID is required")
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		sdk.Abort("invalid project ID")
	}
	newOwnerStr := strings.TrimSpace(parts[1])
	if newOwnerStr == "" {
		sdk.Abort("new owner address required")
	}
	caller := getSenderAddress()
	callerAddr := dao.AddressFromString(caller.String())
	newOwnerAddr := dao.AddressFromString(newOwnerStr)
	prj := loadProject(id)
	if callerAddr != prj.Owner {
		sdk.Abort("only owner can transfer")
	}

	if _, exists := loadMember(prj.ID, newOwnerAddr); !exists {
		sdk.Abort("new owner must be a member")
	}

	prj.Owner = newOwnerAddr
	saveProjectMeta(prj)
	return strptr("ownership transferred")
}

// EmergencyPauseImmediate is the safety valve so owners can halt stuff without waiting for proposals.
// Example payload: EmergencyPauseImmediate(strptr("5|false"))
//
//go:wasmexport project_pause
func EmergencyPauseImmediate(payload *string) *string {
	raw := unwrapPayload(payload, "pause payload required")
	parts := strings.Split(raw, "|")
	if len(parts) == 0 {
		sdk.Abort("project ID is required")
	}
	idStr := strings.TrimSpace(parts[0])
	if idStr == "" {
		sdk.Abort("project ID is required")
	}
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		sdk.Abort("invalid project ID")
	}
	pause := true
	if len(parts) > 1 {
		pause = parseBoolField(parts[1])
	}
	caller := getSenderAddress()
	callerAddr := dao.AddressFromString(caller.String())
	prj := loadProject(id)
	if callerAddr != prj.Owner {
		sdk.Abort("only owner can pause/unpause")
	}
	prj.Paused = pause
	saveProjectMeta(prj)
	if pause {
		return strptr("paused")
	}
	return strptr("unpaused")
}

// saveProject persists all project parts (meta, config, finance).
func saveProject(prj *dao.Project) {
	saveProjectMeta(prj)
	saveProjectConfig(prj)
	saveProjectFinance(prj)
}

// loadProject retrieves a project from contract storage by ID.
// Aborts if the project does not exist.
func loadProject(id uint64) *dao.Project {
	meta := loadProjectMeta(id)
	cfg := loadProjectConfig(id)
	fin := loadProjectFinance(id)
	return &dao.Project{
		ID:          id,
		Owner:       meta.Owner,
		Name:        meta.Name,
		Description: meta.Description,
		Config:      *cfg,
		Metadata:    meta.Metadata,
		Funds:       fin.Funds,
		FundsAsset:  fin.FundsAsset,
		Paused:      meta.Paused,
		Tx:          meta.Tx,
		StakeTotal:  fin.StakeTotal,
		MemberCount: fin.MemberCount,
	}
}

func saveProjectMeta(prj *dao.Project) {
	meta := dao.ProjectMeta{
		Owner:       prj.Owner,
		Name:        prj.Name,
		Description: prj.Description,
		Paused:      prj.Paused,
		Tx:          prj.Tx,
		Metadata:    prj.Metadata,
	}
	data := dao.EncodeProjectMeta(&meta)
	stateSetIfChanged(projectKey(prj.ID), string(data))
}

func loadProjectMeta(id uint64) *dao.ProjectMeta {
	key := projectKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort(fmt.Sprintf("project %d not found", id))
	}
	meta, err := dao.DecodeProjectMeta([]byte(*ptr))
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to decode project meta: %v", err))
	}
	return meta
}

func saveProjectConfig(prj *dao.Project) {
	data := dao.EncodeProjectConfig(&prj.Config)
	stateSetIfChanged(projectConfigKey(prj.ID), string(data))
}

func loadProjectConfig(id uint64) *dao.ProjectConfig {
	key := projectConfigKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("project config not found")
	}
	cfg, err := dao.DecodeProjectConfig([]byte(*ptr))
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to decode project config: %v", err))
	}
	return cfg
}

func saveProjectFinance(prj *dao.Project) {
	fin := dao.ProjectFinance{
		Funds:       prj.Funds,
		FundsAsset:  prj.FundsAsset,
		StakeTotal:  prj.StakeTotal,
		MemberCount: prj.MemberCount,
	}
	data := dao.EncodeProjectFinance(&fin)
	stateSetIfChanged(projectFinanceKey(prj.ID), string(data))
}

func loadProjectFinance(id uint64) *dao.ProjectFinance {
	key := projectFinanceKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("project finance not found")
	}
	fin, err := dao.DecodeProjectFinance([]byte(*ptr))
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to decode project finance: %v", err))
	}
	return fin
}
