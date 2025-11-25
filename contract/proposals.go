package main

import (
	"fmt"
	"math"
	"okinoko_dao/contract/dao"
	"okinoko_dao/sdk"
	"strconv"
	"time"
)

// CreateProposal creates a new proposal within a project.
// Only members may create proposals. If no options are provided,
// defaults to a binary yes/no vote.
//
//go:wasmexport proposal_create
func CreateProposal(payload *string) *string {
	input := decodeCreateProposalArgs(payload)

	caller := getSenderAddress()
	callerAddr := dao.AddressFromString(caller.String())
	prj := loadProject(input.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}

	// Only members can create
	if _, exists := loadMember(prj.ID, callerAddr); !exists {
		sdk.Abort("only members can create proposals")
	}

	isPoll := input.ForcePoll
	if len(input.OptionsList) == 0 {
		input.OptionsList = []string{"no", "yes"}
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

	prpsl := &dao.Proposal{
		ID:                  id,
		ProjectID:           input.ProjectID,
		Creator:             callerAddr,
		Name:                input.Name,
		Description:         input.Description,
		Metadata:            input.Metadata,
		Outcome:             input.ProposalOutcome,
		CreatedAt:           now,
		DurationHours:       duration,
		State:               dao.ProposalActive,
		Tx:                  *sdk.GetEnvKey("tx.id"),
		MemberCountSnapshot: memberSnap,
		StakeSnapshot:       stakeSnap,
		IsPoll:              isPoll,
		OptionCount:         uint32(len(input.OptionsList)),
	}

	saveProposal(prpsl)
	for i, txt := range input.OptionsList {
		opt := dao.ProposalOption{Text: txt}
		saveProposalOption(prpsl.ID, uint32(i), &opt)
	}
	setCount(ProposalsCount, id+1)
	emitProposalCreatedEvent(id, dao.AddressToString(callerAddr))
	emitProposalStateChangedEvent(id, dao.ProposalActive)
	result := strconv.FormatUint(id, 10)
	return &result
}

// -----------------------------------------------------------------------------
// Tally
// -----------------------------------------------------------------------------

// TallyProposal closes voting on a proposal and determines its result.
// Checks quorum and threshold rules, then marks the proposal as passed,
// closed, or failed.
//
//go:wasmexport proposal_tally
func TallyProposal(proposalId *string) *string {
	if proposalId == nil || *proposalId == "" {
		sdk.Abort("proposal ID is required")
	}
	id, err := strconv.ParseUint(*proposalId, 10, 64)
	if err != nil {
		sdk.Abort("invalid proposal ID")
	}
	prpsl := loadProposal(id)
	prj := loadProject(prpsl.ProjectID)

	if prpsl.State != dao.ProposalActive {
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
		totalVotes += dao.AmountToFloat(opt.WeightTotal)
		voterCount += opt.VoterCount

		if dao.AmountToFloat(opt.WeightTotal) > highestOptionValue {
			highestOptionValue = dao.AmountToFloat(opt.WeightTotal)
			highestOptionId = i
		}
	}

	// default to failed
	prpsl.State = dao.ProposalFailed

	if highestOptionId >= 0 && highestOptionValue > 0 {
		// calculate quorum threshold (round up)
		quorumThreshold := uint64(math.Ceil(float64(prpsl.MemberCountSnapshot) * (prj.Config.QuorumPercent / 100)))
		// Check quorum
		quorumMet := voterCount >= quorumThreshold
		// Check threshold (fraction of total stake at creation)
		denom := dao.AmountToFloat(prpsl.StakeSnapshot)
		thresholdMet := denom > 0 && (highestOptionValue/denom) >= prj.Config.ThresholdPercent/100

		if quorumMet && thresholdMet {
			prpsl.ResultOptionID = int32(highestOptionId)
			if prpsl.IsPoll && highestOptionId == 1 {
				prpsl.State = dao.ProposalPassed // make it executable
			} else {
				prpsl.State = dao.ProposalClosed // just close
			}
		}
	}
	saveProposal(prpsl)
	emitProposalStateChangedEvent(prpsl.ID, prpsl.State)
	return strptr("tallied")
}

// -----------------------------------------------------------------------------
// Execute
// -----------------------------------------------------------------------------

// ExecuteProposal executes a previously passed proposal.
// It can transfer funds or update project metadata as defined
// in the proposal outcome.
//
//go:wasmexport proposal_execute
func ExecuteProposal(proposalID *uint64) *string {
	prpsl := loadProposal(*proposalID)
	prj := loadProject(prpsl.ProjectID)
	if prj.Paused {
		sdk.Abort("project is paused")
	}
	if prpsl.State != dao.ProposalPassed {
		sdk.Abort(fmt.Sprintf("proposal is %s", prpsl.State))
	}
	fundsTransferred := false
	configChanged := false
	stateChanged := false
	metaChanged := false
	if prpsl.Outcome != nil {
		if prpsl.Outcome.Payout != nil {
			totalAsked := float64(0)
			for _, stake := range prpsl.Outcome.Payout {
				totalAsked += dao.AmountToFloat(stake)
			}
			if dao.AmountToFloat(prj.Funds) < totalAsked {
				sdk.Abort("insufficient funds")
			}
			for addr, amount := range prpsl.Outcome.Payout {
				mAmount := dao.AmountToInt64(amount)
				prj.Funds -= amount
				sdk.HiveTransfer(sdk.Address(dao.AddressToString(addr)), mAmount, sdk.Asset(dao.AssetToString(prj.FundsAsset)))
				emitFundsRemoved(prj.ID, dao.AddressToString(addr), dao.AmountToFloat(amount), dao.AssetToString(prj.FundsAsset), false)
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
					prj.Config.ThresholdPercent = v
					metaChanged = true
					configChanged = true
				case "update_quorum":
					v, err := strconv.ParseFloat(value, 64)
					if err != nil {
						sdk.Abort("invalid quorum update")
					}
					prj.Config.QuorumPercent = v
					metaChanged = true
					configChanged = true
				case "update_proposalDuration":
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						sdk.Abort("invalid proposal duration update")
					}
					prj.Config.ProposalDurationHours = v
					metaChanged = true
					configChanged = true
				case "update_executionDelay":
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						sdk.Abort("invalid execution delay update")
					}
					prj.Config.ExecutionDelayHours = v
					metaChanged = true
					configChanged = true
				case "update_leaveCooldown":
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						sdk.Abort("invalid leave cooldown update")
					}
					prj.Config.LeaveCooldownHours = v
					metaChanged = true
					configChanged = true
				case "update_proposalCost":
					v, err := strconv.ParseFloat(value, 64)
					if err != nil {
						sdk.Abort("invalid proposal cost update")
					}
					prj.Config.ProposalCost = v
					metaChanged = true
					configChanged = true
				case "update_membershipNFT":
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						sdk.Abort("invalid membership nft update")
					}
					prj.Config.MembershipNFT = &v
					metaChanged = true
					configChanged = true
				case "update_membershipNFTContract":
					v := value
					prj.Config.MembershipNFTContract = &v
					metaChanged = true
					configChanged = true
				case "update_membershipNFTContractFunction":
					v := value
					prj.Config.MembershipNFTContractFunction = &v
					metaChanged = true
					configChanged = true
				case "toggle_pause":
					prj.Paused = !prj.Paused
					metaChanged = true
					stateChanged = true
				}
			}
		}
	}

	prpsl.State = dao.ProposalExecuted
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
func saveProposal(prpsl *dao.Proposal) {
	key := proposalKey(prpsl.ID)
	data := dao.EncodeProposal(prpsl)
	sdk.StateSetObject(key, string(data))
}

// loadProposal retrieves a proposal from contract state by ID.
// Aborts if not found or if unmarshalling fails.
func loadProposal(id uint64) *dao.Proposal {
	key := proposalKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		sdk.Abort(fmt.Sprintf("proposal %d not found", id))
	}
	prpsl, err := dao.DecodeProposal([]byte(*ptr))
	if err != nil {
		sdk.Abort(fmt.Sprintf("failed to decode proposal: %v", err))
	}
	return prpsl
}
