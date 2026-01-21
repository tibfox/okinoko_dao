package main

import (
	"fmt"
	"math"
	"okinoko_dao/sdk"
	"strconv"
	"strings"
	"time"
)

// CreateProposal builds a proposal payload, enforces membership + costs, and snapshots stake + member counts.
// Example payload: CreateProposal(strptr("5|Add funds|Desc|..."))
//
//go:wasmexport proposal_create
func CreateProposal(payload *string) *string {
	input := decodeCreateProposalArgs(payload)

	caller := getSenderAddress()
	callerStr := caller.String()
	callerAddr := caller
	prj := loadProject(input.ProjectID)
	if prj.Paused {
		if input.ProposalOutcome == nil || input.ProposalOutcome.Meta == nil {
			sdk.Abort("project is paused")
		}
		if !allowsPauseMeta(input.ProposalOutcome.Meta) {
			sdk.Abort("project is paused")
		}
	}

	// Only members can create unless config allows public proposals
	if prj.Config.ProposalsMembersOnly {
		if _, exists := loadMember(prj.ID, callerAddr); !exists {
			sdk.Abort("only members can create proposals")
		}
	}

	isPoll := input.ForcePoll
	if len(input.OptionsList) == 0 {
		input.OptionsList = []ProposalOptionInput{
			{Text: "no", URL: ""},
			{Text: "yes", URL: ""},
		}
		if !input.ForcePoll {
			isPoll = false
		}
	} else if !input.ForcePoll {
		isPoll = true
	}

	id := getCount(ProposalsCount)

	var duration uint64
	if input.ProposalDuration > 0 {
		if input.ProposalDuration < prj.Config.ProposalDurationHours {
			sdk.Abort("Duration must be higher or equal to project defined proposal duration")
		}
		duration = input.ProposalDuration
	} else {
		duration = prj.Config.ProposalDurationHours
	}

	now := nowUnix()

	// Count members / sum stakes from aggregates
	memberSnap := uint(prj.MemberCount)
	stakeSnap := prj.StakeTotal

	// Prevent proposals when there are no stakes (stake-based voting would be meaningless)
	if prj.Config.VotingSystem == VotingSystemStake && stakeSnap == 0 {
		sdk.Abort("cannot create proposal with zero total stake in stake-based project")
	}

	txID := ""
	if txPtr := sdk.GetEnvKey("tx.id"); txPtr != nil {
		txID = *txPtr
	}

	prpsl := &Proposal{
		ID:                  id,
		ProjectID:           input.ProjectID,
		Creator:             callerAddr,
		Name:                input.Name,
		Description:         input.Description,
		Metadata:            input.Metadata,
		URL:                 input.URL,
		Outcome:             input.ProposalOutcome,
		CreatedAt:           now,
		DurationHours:       duration,
		State:               ProposalActive,
		Tx:                  txID,
		MemberCountSnapshot: memberSnap,
		StakeSnapshot:       stakeSnap,
		IsPoll:              isPoll,
		OptionCount:         uint32(len(input.OptionsList)),
		ExecutableAt:        0,
	}

	if prj.Config.ProposalCost > 0 {
		ta := getFirstTransferAllow()
		if ta == nil {
			sdk.Abort("no valid transfer intent provided")
		}
		if ta.Token != prj.FundsAsset {
			sdk.Abort(fmt.Sprintf("invalid asset, expected %s", AssetToString(prj.FundsAsset)))
		}
		costAmount := FloatToAmount(prj.Config.ProposalCost)
		providedAmount := FloatToAmount(ta.Limit)
		if providedAmount < costAmount {
			sdk.Abort(fmt.Sprintf("proposal cost requires at least %f %s", prj.Config.ProposalCost, ta.Token.String()))
		}
		mAmount := AmountToInt64(costAmount)
		sdk.HiveDraw(mAmount, ta.Token)
		prj.Funds += costAmount
		saveProjectFinance(prj)
		emitFundsAdded(prj.ID, callerStr, AmountToFloat(costAmount), ta.Token.String(), false)
	}

	saveProposal(prpsl)
	if prpsl.Outcome != nil && prpsl.Outcome.Payout != nil {
		incrementPayoutLocks(prpsl.ProjectID, prpsl.Outcome.Payout)
	}
	for i, optInput := range input.OptionsList {
		opt := ProposalOption{
			Text: optInput.Text,
			URL:  optInput.URL,
		}
		saveProposalOption(prpsl.ID, uint32(i), &opt)
	}
	setCount(ProposalsCount, id+1)

	emitProposalCreatedEvent(prpsl, prj.ID, AddressToString(callerAddr), input.OptionsList)
	emitProposalStateChangedEvent(id, ProposalActive)
	result := strconv.FormatUint(id, 10)
	return &result
}

// -----------------------------------------------------------------------------
// Tally
// -----------------------------------------------------------------------------

// TallyProposal crunches the weight totals, checks quorum/threshold and sets the final state.
// Example payload: TallyProposal(strptr("12"))
//
//go:wasmexport proposal_tally
func TallyProposal(proposalId *string) *string {
	raw := unwrapPayload(proposalId, "proposal ID is required")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		sdk.Abort("invalid proposal ID")
	}
	prpsl := loadProposal(id)
	prj := loadProject(prpsl.ProjectID)

	if prpsl.State != ProposalActive {
		sdk.Abort("proposal not active")
	}
	deadline := prpsl.CreatedAt + int64(prpsl.DurationHours)*3600
	if nowUnix() < deadline {
		sdk.Abort(fmt.Sprintf("proposal still running until %s", time.Unix(deadline, 0).UTC().Format(time.RFC3339)))
	}

	// find best option
	var totalVotes float64
	var voterCount uint64
	highestOptionId := -1
	highestOptionValue := float64(0)

	opts := loadProposalOptions(prpsl.ID, prpsl.OptionCount)
	for i, opt := range opts {
		weight := AmountToFloat(opt.WeightTotal)
		totalVotes += weight
		voterCount += opt.VoterCount

		if weight > highestOptionValue {
			highestOptionValue = weight
			highestOptionId = i
		}
	}

	// default to failed
	prpsl.State = ProposalFailed
	prpsl.ExecutableAt = 0

	if highestOptionId >= 0 && highestOptionValue > 0 {
		// calculate quorum threshold (round up)
		quorumThreshold := uint64(math.Ceil(percentageOf(float64(prpsl.MemberCountSnapshot), prj.Config.QuorumPercent)))
		// Check quorum
		quorumMet := voterCount >= quorumThreshold
		// Check threshold (fraction of total stake at creation)
		denom := AmountToFloat(prpsl.StakeSnapshot)
		thresholdMet := denom > 0 && (highestOptionValue/denom) >= (prj.Config.ThresholdPercent/100)

		if quorumMet && thresholdMet {
			prpsl.ResultOptionID = int32(highestOptionId)
			if prpsl.IsPoll {
				prpsl.State = ProposalClosed // polls remain advisory even if yes wins
			} else {
				prpsl.State = ProposalPassed // non-polls become executable actions
				execReady := prpsl.CreatedAt + int64(prpsl.DurationHours+prj.Config.ExecutionDelayHours)*3600
				prpsl.ExecutableAt = execReady
				emitProposalExecutionDelayEvent(prpsl.ProjectID, prpsl.ID, execReady)
			}
		}
	}
	if prpsl.Outcome != nil && prpsl.Outcome.Payout != nil {
		decrementPayoutLocks(prpsl.ProjectID, prpsl.Outcome.Payout)
	}

	saveProposal(prpsl)
	emitProposalStateChangedEvent(prpsl.ID, prpsl.State)
	return strptr("tallied")
}

// -----------------------------------------------------------------------------
// Execute
// -----------------------------------------------------------------------------

// ExecuteProposal enforces execution delays, drains treasury if needed, and applies config/meta changes.
// Example payload: ExecuteProposal(strptr("77"))
//
//go:wasmexport proposal_execute
func ExecuteProposal(proposalID *string) *string {
	raw := unwrapPayload(proposalID, "proposal ID is required")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		sdk.Abort("invalid proposal ID")
	}
	prpsl := loadProposal(id)
	prj := loadProject(prpsl.ProjectID)
	if prj.Paused && !proposalAllowsExecutionWhilePaused(prpsl) {
		sdk.Abort("project is paused")
	}
	if prpsl.State == ProposalExecuted {
		sdk.Abort("proposal already executed")
	}
	if prpsl.State != ProposalPassed {
		sdk.Abort(fmt.Sprintf("proposal is %s", prpsl.State))
	}

	// If proposal has ICC calls, only creator can execute
	if prpsl.Outcome != nil && len(prpsl.Outcome.ICC) > 0 {
		caller := getSenderAddress()
		if caller != prpsl.Creator {
			sdk.Abort("only proposal creator can execute proposals with inter-contract calls")
		}
	}

	requiredReady := prpsl.CreatedAt + int64(prpsl.DurationHours+prj.Config.ExecutionDelayHours)*3600
	if prpsl.ExecutableAt > requiredReady {
		requiredReady = prpsl.ExecutableAt
	}
	if nowUnix() < requiredReady {
		sdk.Abort(fmt.Sprintf("execution delay until %s", time.Unix(requiredReady, 0).UTC().Format(time.RFC3339)))
	}

	fundsTransferred := false
	configChanged := false
	stateChanged := false
	metaChanged := false
	if prpsl.Outcome != nil {
		if prpsl.Outcome.Payout != nil {
			// Transfer each payout with its specified asset
			for addr, entry := range prpsl.Outcome.Payout {
				asset := entry.Asset
				// If no asset specified (legacy), use project's original asset
				if asset.String() == "" {
					asset = prj.FundsAsset
				}
				// Check treasury balance for this asset
				treasuryBalance := getTreasuryBalance(prj.ID, asset)
				if treasuryBalance < entry.Amount {
					sdk.Abort(fmt.Sprintf("insufficient %s funds in treasury", AssetToString(asset)))
				}
				// Remove from treasury
				if !removeTreasuryFunds(prj.ID, asset, entry.Amount) {
					sdk.Abort(fmt.Sprintf("failed to remove %s from treasury", AssetToString(asset)))
				}
				// Transfer to recipient
				mAmount := AmountToInt64(entry.Amount)
				sdk.HiveTransfer(addr, mAmount, asset)
				emitFundsRemoved(prj.ID, AddressToString(addr), AmountToFloat(entry.Amount), AssetToString(asset), false)
				fundsTransferred = true
			}
		}
		if prpsl.Outcome.Meta != nil {
			// meta change
			for action, value := range prpsl.Outcome.Meta {
				switch action {
				// todo: add more
				case "update_threshold":
					v, err := strconv.ParseFloat(value, 64)
					if err != nil {
						sdk.Abort("invalid threshold update")
					}
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "threshold", fmt.Sprintf("%f", prj.Config.ThresholdPercent), fmt.Sprintf("%f", v))
					prj.Config.ThresholdPercent = v
					metaChanged = true
					configChanged = true
				case "update_quorum":
					v, err := strconv.ParseFloat(value, 64)
					if err != nil {
						sdk.Abort("invalid quorum update")
					}
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "quorum", fmt.Sprintf("%f", prj.Config.QuorumPercent), fmt.Sprintf("%f", v))
					prj.Config.QuorumPercent = v
					metaChanged = true
					configChanged = true
				case "update_proposalDuration":
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						sdk.Abort("invalid proposal duration update")
					}
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "proposalDuration", fmt.Sprintf("%d", prj.Config.ProposalDurationHours), value)
					prj.Config.ProposalDurationHours = v
					metaChanged = true
					configChanged = true
				case "update_executionDelay":
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						sdk.Abort("invalid execution delay update")
					}
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "executionDelay", fmt.Sprintf("%d", prj.Config.ExecutionDelayHours), value)
					prj.Config.ExecutionDelayHours = v
					metaChanged = true
					configChanged = true
				case "update_leaveCooldown":
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						sdk.Abort("invalid leave cooldown update")
					}
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "leaveCooldown", fmt.Sprintf("%d", prj.Config.LeaveCooldownHours), value)
					prj.Config.LeaveCooldownHours = v
					metaChanged = true
					configChanged = true
				case "update_proposalCost":
					v, err := strconv.ParseFloat(value, 64)
					if err != nil {
						sdk.Abort("invalid proposal cost update")
					}
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "proposalCost", fmt.Sprintf("%f", prj.Config.ProposalCost), fmt.Sprintf("%f", v))
					prj.Config.ProposalCost = v
					metaChanged = true
					configChanged = true
				case "update_membershipNFT":
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						sdk.Abort("invalid membership nft update")
					}
					prev := ""
					if prj.Config.MembershipNFT != nil {
						prev = fmt.Sprintf("%d", *prj.Config.MembershipNFT)
					}
					prj.Config.MembershipNFT = &v
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "membershipNFT", prev, value)
					metaChanged = true
					configChanged = true
				case "update_membershipNFTContract":
					v := value
					prev := ""
					if prj.Config.MembershipNFTContract != nil {
						prev = *prj.Config.MembershipNFTContract
					}
					prj.Config.MembershipNFTContract = &v
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "membershipNFTContract", prev, value)
					metaChanged = true
					configChanged = true
				case "update_membershipNFTContractFunction":
					v := value
					prev := ""
					if prj.Config.MembershipNFTContractFunction != nil {
						prev = *prj.Config.MembershipNFTContractFunction
					}
					prj.Config.MembershipNFTContractFunction = &v
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "membershipNFTContractFunction", prev, value)
					metaChanged = true
					configChanged = true
				case "update_membershipNFTPayload":
					prev := prj.Config.MembershipNftPayloadFormat
					prj.Config.MembershipNftPayloadFormat = normalizeMembershipPayloadFormat(value)
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "membershipNftPayload", prev, prj.Config.MembershipNftPayloadFormat)
					metaChanged = true
					configChanged = true
				case "update_proposalCreatorRestriction":
					prev := prj.Config.ProposalsMembersOnly
					prj.Config.ProposalsMembersOnly = parseCreatorRestrictionField(value)
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "proposalCreatorRestriction", strconv.FormatBool(prev), strconv.FormatBool(prj.Config.ProposalsMembersOnly))
					metaChanged = true
					configChanged = true
				case "toggle_pause":
					prev := prj.Paused
					prj.Paused = !prj.Paused
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "paused", strconv.FormatBool(prev), strconv.FormatBool(prj.Paused))
					metaChanged = true
					stateChanged = true
				case "update_owner":
					newOwnerAddr := AddressFromString(value)
					if _, exists := loadMember(prj.ID, newOwnerAddr); !exists {
						sdk.Abort("new owner must be a member")
					}
					oldOwner := prj.Owner
					prj.Owner = newOwnerAddr
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "owner", AddressToString(oldOwner), AddressToString(newOwnerAddr))
					metaChanged = true
					stateChanged = true
				case "update_url":
					prev := prj.URL
					prj.URL = normalizeOptionalField(value)
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "url", prev, prj.URL)
					metaChanged = true
					stateChanged = true
				case "update_whitelistOnly":
					val := strings.ToLower(strings.TrimSpace(value))
					var newVal bool
					switch val {
					case "1", "true", "yes":
						newVal = true
					case "0", "false", "no":
						newVal = false
					default:
						sdk.Abort("invalid whitelist flag")
					}
					prev := prj.Config.WhitelistOnly
					prj.Config.WhitelistOnly = newVal
					emitProposalConfigUpdatedEvent(prj.ID, prpsl.ID, "whitelistOnly", strconv.FormatBool(prev), strconv.FormatBool(newVal))
					metaChanged = true
					configChanged = true
				case "whitelist_add":
					addresses := parseAddressList(value)
					if len(addresses) == 0 {
						sdk.Abort("whitelist_add metadata requires addresses")
					}
					added := addWhitelistEntries(prj.ID, addresses)
					if len(added) > 0 {
						emitWhitelistEvent(prj.ID, "add", added)
						metaChanged = true
					}
				case "whitelist_remove":
					addresses := parseAddressList(value)
					if len(addresses) == 0 {
						sdk.Abort("whitelist_remove metadata requires addresses")
					}
					removed := removeWhitelistEntries(prj.ID, addresses)
					if len(removed) > 0 {
						emitWhitelistEvent(prj.ID, "remove", removed)
						metaChanged = true
					}
				}
			}
		}
		if len(prpsl.Outcome.ICC) > 0 {
			// Execute inter-contract calls
			for _, icc := range prpsl.Outcome.ICC {
				// Build intents for asset transfers
				var opts *sdk.ContractCallOptions
				if len(icc.Assets) > 0 {
					intents := make([]sdk.Intent, 0, len(icc.Assets))
					for asset, amount := range icc.Assets {
						// Check treasury balance
						treasuryBalance := getTreasuryBalance(prj.ID, asset)
						if treasuryBalance < amount {
							sdk.Abort(fmt.Sprintf("insufficient %s funds in treasury for ICC", AssetToString(asset)))
						}
						// Remove from treasury
						if !removeTreasuryFunds(prj.ID, asset, amount) {
							sdk.Abort(fmt.Sprintf("failed to remove %s from treasury for ICC", AssetToString(asset)))
						}

						// Create transfer intent
						intents = append(intents, sdk.Intent{
							Type: "transfer.allow",
							Args: map[string]string{
								"to":     icc.ContractAddress,
								"tk":     AssetToString(asset),
								"amount": fmt.Sprintf("%d", AmountToInt64(amount)),
							},
						})
						fundsTransferred = true
					}
					opts = &sdk.ContractCallOptions{
						Intents: intents,
					}
				}

				// Execute the contract call
				sdk.ContractCall(icc.ContractAddress, icc.Function, icc.Payload, opts)
				emitProposalResultEvent(prj.ID, prpsl.ID, fmt.Sprintf("ICC executed: %s.%s", icc.ContractAddress, icc.Function))
			}
		}
	}

	prpsl.State = ProposalExecuted
	saveProposal(prpsl)
	if fundsTransferred {
		saveProjectFinance(prj)
	}
	if configChanged {
		saveProjectConfig(prj)
	}
	if stateChanged {
		saveProjectMeta(prj)
	}
	emitProposalStateChangedEvent(prpsl.ID, prpsl.State)
	if metaChanged {
		emitProposalResultEvent(prj.ID, prpsl.ID, "meta changed")
	}
	if fundsTransferred {
		emitProposalResultEvent(prj.ID, prpsl.ID, "funds transferred")
	}
	return strptr("executed")
}

// -----------------------------------------------------------------------------
// Save/Load
// -----------------------------------------------------------------------------

// saveProposal persists a proposal in contract state.
func saveProposal(prpsl *Proposal) {
	key := proposalKey(prpsl.ID)
	data := EncodeProposal(prpsl)
	sdk.StateSetObject(key, string(data))
}

// loadProposal retrieves a proposal from contract state by ID.
// Aborts if not found or if unmarshalling fails.
func loadProposal(id uint64) *Proposal {
	key := proposalKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort(fmt.Sprintf("proposal %d not found", id))
	}
	prpsl, err := DecodeProposal([]byte(*ptr))
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to decode proposal: %v", err))
	}
	return prpsl
}

// CancelProposal lets either creator or owner abort an active proposal and optionally refund the cost.
// Example payload: CancelProposal(strptr("42"))
//
//go:wasmexport proposal_cancel
func CancelProposal(payload *string) *string {
	raw := unwrapPayload(payload, "proposal ID is required")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		sdk.Abort("invalid proposal ID")
	}
	prpsl := loadProposal(id)
	if prpsl.State != ProposalActive {
		sdk.Abort("proposal not active")
	}
	prj := loadProject(prpsl.ProjectID)

	caller := getSenderAddress()
	callerAddr := caller
	if callerAddr != prpsl.Creator && callerAddr != prj.Owner {
		sdk.Abort("only creator or owner can cancel")
	}

	refund := callerAddr != prpsl.Creator && prj.Config.ProposalCost > 0
	var refundAmount Amount
	if refund {
		refundAmount = FloatToAmount(prj.Config.ProposalCost)
		if prj.Funds < refundAmount {
			// Treasury has insufficient funds for refund - proposal cost remains with project
			refund = false
		} else {
			prj.Funds -= refundAmount
			saveProjectFinance(prj)
			mAmount := AmountToInt64(refundAmount)
			sdk.HiveTransfer(prpsl.Creator, mAmount, prj.FundsAsset)
			emitFundsRemoved(prj.ID, AddressToString(prpsl.Creator), AmountToFloat(refundAmount), AssetToString(prj.FundsAsset), false)
		}
	}

	prpsl.State = ProposalCancelled
	prpsl.ResultOptionID = -1
	prpsl.ExecutableAt = 0

	if prpsl.Outcome != nil && prpsl.Outcome.Payout != nil {
		decrementPayoutLocks(prpsl.ProjectID, prpsl.Outcome.Payout)
	}

	saveProposal(prpsl)
	emitProposalStateChangedEvent(prpsl.ID, prpsl.State)
	return strptr("cancelled")
}

// -----------------------------------------------------------------------------
// Local helpers
// -----------------------------------------------------------------------------

// percentageOf calculates the percentage of a value.
// Example: percentageOf(100, 50.5) returns 50.5
func percentageOf(value, percent float64) float64 {
	return value * (percent / 100.0)
}

// allowsPauseMeta checks whether the meta payload only toggles pause state or transfers ownership.
func allowsPauseMeta(meta map[string]string) bool {
	if meta == nil {
		return false
	}
	if len(meta) == 1 {
		if _, ok := meta["toggle_pause"]; ok {
			return true
		}
		if _, ok := meta["update_owner"]; ok {
			return true
		}
	}
	return false
}

// proposalAllowsExecutionWhilePaused reuses the meta helper so pause votes can execute safely.
func proposalAllowsExecutionWhilePaused(prpsl *Proposal) bool {
	if prpsl == nil || prpsl.Outcome == nil || prpsl.Outcome.Meta == nil {
		return false
	}
	return allowsPauseMeta(prpsl.Outcome.Meta)
}
