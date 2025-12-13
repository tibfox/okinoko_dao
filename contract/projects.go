package main

import (
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
	"strings"
)

const (
	FallbackNFTContract                 = ""
	FallbackNFTFunction                 = ""
	FallbackThresholdPercent            = 50.001
	FallbackQuorumPercent               = 50.001
	FallbackProposalDurationHours       = 24
	FallbackExecutionDelayHours         = 4
	FallbackLeaveCooldownHours          = 24
	FallbackProposalCost                = 1
	FallbackProposalCreatorsMembersOnly = true
	FallbackMembershipPayloadFormat     = "{nft}|{caller}"
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
	callerAddr := caller
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
		if FloatToAmount(stakeMin) > FloatToAmount(stakeLimit) {
			sdk.Abort(fmt.Sprintf("transfer limit %f < StakeMinAmt %f", stakeLimit, stakeMin))
		}
	} else {
		input.ProjectConfig.StakeMinAmt = stakeLimit
	}

	// --- create project ---
	id := getCount(ProjectsCount)
	now := nowUnix()
	txPtr := sdk.GetEnvKey("tx.id")
	txID := ""
	if txPtr != nil {
		txID = *txPtr
	}

	// Calculate amounts to avoid rounding errors
	stakeAmount := FloatToAmount(stakeMin)
	depositAmount := FloatToAmount(ta.Limit)
	treasuryAmount := depositAmount - stakeAmount

	prj := Project{
		ID:          id,
		Owner:       callerAddr,
		Name:        input.Name,
		Description: input.Description,
		URL:         input.URL,
		Metadata:    input.Metadata,
		Funds:       treasuryAmount,
		FundsAsset:  ta.Token,
		Paused:      false,
		Tx:          txID,
		StakeTotal:  stakeAmount,
		MemberCount: 1,
	}
	prj.Config = input.ProjectConfig

	creatorMember := Member{
		Address:      callerAddr,
		Stake:        stakeAmount,
		JoinedAt:     now,
		LastActionAt: now,
		Reputation:   0,
	}
	// draw the funds
	mAmount := AmountToInt64(depositAmount)
	sdk.HiveDraw(mAmount, ta.Token)
	saveMember(prj.ID, &creatorMember)
	// save project
	saveProject(&prj)
	setCount(ProjectsCount, id+1)

	emitProjectCreatedEvent(&prj, callerStr)
	stakeAmountFloat := AmountToFloat(stakeAmount)
	treasuryAmountFloat := AmountToFloat(treasuryAmount)
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
	callerAddr := caller

	if _, exists := loadMember(prj.ID, callerAddr); exists {
		sdk.Abort("already a member")
	}

	// Check NFT membership requirement
	if !checkNFTMembership(prj, callerAddr) {
		sdk.Abort("membership nft not owned")
	}
	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow()
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}
	if ta.Token != prj.FundsAsset {
		sdk.Abort(fmt.Sprintf("invalid asset, expected %s", AssetToString(prj.FundsAsset)))
	}

	if prj.Config.VotingSystem == VotingSystemDemocratic {
		expectedStake := FloatToAmount(prj.Config.StakeMinAmt)
		providedStake := FloatToAmount(ta.Limit)
		if providedStake != expectedStake {
			sdk.Abort(fmt.Sprintf("democratic projects require exactly %f %s", prj.Config.StakeMinAmt, ta.Token.String()))
		}
	}

	if prj.Config.VotingSystem == VotingSystemStake {
		requiredStake := FloatToAmount(prj.Config.StakeMinAmt)
		providedStake := FloatToAmount(ta.Limit)
		if providedStake < requiredStake {
			sdk.Abort(fmt.Sprintf("stake too low, minimum %f %s required", prj.Config.StakeMinAmt, ta.Token.String()))
		}
	}
	now := nowUnix()

	depositAmount := FloatToAmount(ta.Limit)
	newMember := Member{
		Address:      callerAddr,
		Stake:        depositAmount,
		JoinedAt:     now,
		LastActionAt: now,
	}
	saveMember(prj.ID, &newMember)
	prj.MemberCount++
	prj.StakeTotal += depositAmount
	// draw the funds
	mAmount := AmountToInt64(depositAmount)
	sdk.HiveDraw(mAmount, ta.Token)
	saveProjectFinance(prj)
	emitJoinedEvent(prj.ID, AddressToString(callerAddr))
	emitFundsAdded(prj.ID, AddressToString(callerAddr), AmountToFloat(depositAmount), ta.Token.String(), true)
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
	callerAddr := caller
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
	mAmount := AmountToInt64(withdraw)
	sdk.HiveTransfer(caller, mAmount, prj.FundsAsset)

	deleteMember(prj.ID, callerAddr)
	if prj.MemberCount > 0 {
		prj.MemberCount--
	}
	prj.StakeTotal -= withdraw
	saveProjectFinance(prj)
	emitLeaveEvent(prj.ID, AddressToString(callerAddr))
	emitFundsRemoved(prj.ID, AddressToString(callerAddr), AmountToFloat(withdraw), AssetToString(prj.FundsAsset), true)
	return strptr("exit finished")
}

// hasActivePayout checks payout locks to avoid releasing stake while funds are still promised.
func hasActivePayout(projectID uint64, member sdk.Address) bool {
	return getPayoutLockCount(projectID, member) > 0
}

// checkNFTMembership verifies if caller owns the required NFT for membership.
// Returns true if NFT check passes or if no NFT is required.
func checkNFTMembership(prj *Project, callerAddr sdk.Address) bool {
	if prj.Config.MembershipNFT == nil ||
		prj.Config.MembershipNFTContract == nil ||
		prj.Config.MembershipNFTContractFunction == nil {
		return true
	}

	contract := prj.Config.MembershipNFTContract
	function := prj.Config.MembershipNFTContractFunction
	if contract == nil || function == nil {
		return true
	}

	contractName := strings.TrimSpace(*contract)
	functionName := strings.TrimSpace(*function)
	if contractName == "" || functionName == "" {
		return true
	}

	payload := formatMembershipPayload(
		prj.Config.MembershipNftPayloadFormat,
		UInt64ToString(*prj.Config.MembershipNFT),
		AddressToString(callerAddr),
	)

	editions := sdk.ContractCall(contractName, functionName, payload, nil)
	return editions != nil && *editions != "[]" && *editions != ""
}

// AddFunds handles the double-path deposit (treasury vs stake) and still validates asset + transfer.allow.
// Example payload: AddFunds(strptr("7|1"))
//
//go:wasmexport project_funds
func AddFunds(payload *string) *string {
	input := decodeAddFundsArgs(payload)
	prj := loadProject(input.ProjectID)
	caller := getSenderAddress()
	callerAddr := caller
	var stakingMember *Member

	// --- get first valid transfer intent ---
	ta := getFirstTransferAllow()
	if ta == nil {
		sdk.Abort("no valid transfer intent provided")
	}

	// validate intent token matches project treasury asset
	if ta.Token != prj.FundsAsset {
		sdk.Abort(fmt.Sprintf("invalid asset, expected %s", AssetToString(prj.FundsAsset)))
	}

	if input.ToStake {
		if prj.Config.VotingSystem == VotingSystemDemocratic {
			sdk.Abort("cannot add member stake > StakeMinAmt in democratic systems")
		}
		member := getMember(prj.ID, callerAddr)
		stakingMember = &member
	}

	// draw the funds
	depositAmount := FloatToAmount(ta.Limit)
	mAmount := AmountToInt64(depositAmount)
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
	emitFundsAdded(prj.ID, AddressToString(callerAddr), AmountToFloat(depositAmount), ta.Token.String(), input.ToStake)
	return strptr("funds added")
}

// getMember fetches the cached member or aborts with a readable adress on failure.
func getMember(projectID uint64, user sdk.Address) Member {
	member, ok := loadMember(projectID, user)
	if !ok {
		sdk.Abort(fmt.Sprintf("%s is not a member", AddressToString(user)))
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
	callerAddr := caller
	newOwnerAddr := AddressFromString(newOwnerStr)
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
	callerAddr := caller
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
func saveProject(prj *Project) {
	saveProjectMeta(prj)
	saveProjectConfig(prj)
	saveProjectFinance(prj)
}

// loadProject retrieves a project from contract storage by ID.
// Aborts if the project does not exist.
func loadProject(id uint64) *Project {
	meta := loadProjectMeta(id)
	cfg := loadProjectConfig(id)
	fin := loadProjectFinance(id)
	return &Project{
		ID:          id,
		Owner:       meta.Owner,
		Name:        meta.Name,
		Description: meta.Description,
		URL:         meta.URL,
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

func saveProjectMeta(prj *Project) {
	meta := ProjectMeta{
		Owner:       prj.Owner,
		Name:        prj.Name,
		Description: prj.Description,
		Paused:      prj.Paused,
		Tx:          prj.Tx,
		Metadata:    prj.Metadata,
		URL:         prj.URL,
	}
	data := EncodeProjectMeta(&meta)
	stateSetIfChanged(projectKey(prj.ID), string(data))
}

func loadProjectMeta(id uint64) *ProjectMeta {
	key := projectKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort(fmt.Sprintf("project %d not found", id))
	}
	meta, err := DecodeProjectMeta([]byte(*ptr))
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to decode project meta: %v", err))
	}
	return meta
}

func saveProjectConfig(prj *Project) {
	data := EncodeProjectConfig(&prj.Config)
	stateSetIfChanged(projectConfigKey(prj.ID), string(data))
}

func loadProjectConfig(id uint64) *ProjectConfig {
	key := projectConfigKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("project config not found")
	}
	cfg, err := DecodeProjectConfig([]byte(*ptr))
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to decode project config: %v", err))
	}
	return cfg
}

func saveProjectFinance(prj *Project) {
	fin := ProjectFinance{
		Funds:       prj.Funds,
		FundsAsset:  prj.FundsAsset,
		StakeTotal:  prj.StakeTotal,
		MemberCount: prj.MemberCount,
	}
	data := EncodeProjectFinance(&fin)
	stateSetIfChanged(projectFinanceKey(prj.ID), string(data))
}

func loadProjectFinance(id uint64) *ProjectFinance {
	key := projectFinanceKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort("project finance not found")
	}
	fin, err := DecodeProjectFinance([]byte(*ptr))
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to decode project finance: %v", err))
	}
	return fin
}
