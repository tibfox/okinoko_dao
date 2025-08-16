package main

import (
	"encoding/json"
	"fmt"
	"okinoko_dao/sdk"
)

////////////////////////////////////////////////////////////////////////////////
// Contract State Persistence helpers
////////////////////////////////////////////////////////////////////////////////

func saveProject(pro *Project) {
	key := projectKey(pro.ID)
	b, _ := json.Marshal(pro)
	sdk.StateSetObject(key, string(b))
}

func loadProject(id string) (*Project, error) {
	key := projectKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil {
		return nil, fmt.Errorf("project %s not found", id)
	}
	var pro Project
	if err := json.Unmarshal([]byte(*ptr), &pro); err != nil {
		return nil, fmt.Errorf("failed unmarshal project %s: %v", id, err)
	}
	return &pro, nil
}

func addProjectToIndex(id string) {
	ptr := sdk.StateGetObject(projectsIndexKey)
	var ids []string
	if ptr != nil {
		json.Unmarshal([]byte(*ptr), &ids)
	}
	// prevent duplicates
	for _, v := range ids {
		if v == id {
			return
		}
	}
	ids = append(ids, id)
	b, _ := json.Marshal(ids)
	sdk.StateSetObject(projectsIndexKey, string(b))
}

func listAllProjectIDs() []string {
	ptr := sdk.StateGetObject(projectsIndexKey)
	if ptr == nil {
		return []string{}
	}
	var ids []string
	if err := json.Unmarshal([]byte(*ptr), &ids); err != nil {
		return []string{}
	}
	return ids
}

func saveProposal(prpsl *Proposal) {
	key := proposalKey(prpsl.ID)
	b, _ := json.Marshal(prpsl)
	sdk.StateSetObject(key, string(b))
}

func loadProposal(id string) (*Proposal, error) {
	key := proposalKey(id)
	ptr := sdk.StateGetObject(key)
	if ptr == nil {
		return nil, fmt.Errorf("proposal %s not found", id)
	}
	var prpsl Proposal
	if err := json.Unmarshal([]byte(*ptr), &prpsl); err != nil {
		return nil, fmt.Errorf("failed unmarshal proposal %s: %v", id, err)
	}
	return &prpsl, nil
}

func addProposalToProjectIndex(projectID, proposalID string) {
	key := projectProposalsIndexKey(projectID)
	ptr := sdk.StateGetObject(key)
	var ids []string
	if ptr != nil {
		json.Unmarshal([]byte(*ptr), &ids)
	}
	// TODO: avoid dublicates
	ids = append(ids, proposalID)
	b, _ := json.Marshal(ids)
	sdk.StateSetObject(key, string(b))
}

func listProposalIDsForProject(projectID string) []string {
	key := projectProposalsIndexKey(projectID)
	ptr := sdk.StateGetObject(key)
	if ptr == nil {
		return []string{}
	}
	var ids []string
	if err := json.Unmarshal([]byte(*ptr), &ids); err != nil {
		return []string{}
	}
	return ids
}

func saveVote(vote *VoteRecord) {
	key := voteKey(vote.ProjectID, vote.ProposalID, vote.Voter)
	b, _ := json.Marshal(vote)
	sdk.StateSetObject(key, string(b))

	// ensure voter listed in index for iteration (store list under project:proposal:voters)
	votersKey := fmt.Sprintf("proposal:%s:%s:voters", vote.ProjectID, vote.ProposalID)
	ptr := sdk.StateGetObject(votersKey)
	var voters []string
	if ptr != nil {
		json.Unmarshal([]byte(*ptr), &voters)
	}
	seen := false
	for _, a := range voters {
		if a == vote.Voter {
			seen = true
			break
		}
	}
	if !seen {
		voters = append(voters, vote.Voter)
		nb, _ := json.Marshal(voters)
		sdk.StateSetObject(votersKey, string(nb))
	}
}

func loadVotesForProposal(projectID, proposalID string) []VoteRecord {
	votersKey := fmt.Sprintf("proposal:%s:%s:voters", projectID, proposalID)
	ptr := sdk.StateGetObject(votersKey)
	if ptr == nil {
		return []VoteRecord{}
	}
	var voters []string
	if err := json.Unmarshal([]byte(*ptr), &voters); err != nil {
		return []VoteRecord{}
	}
	out := make([]VoteRecord, 0, len(voters))
	for _, v := range voters {
		vk := voteKey(projectID, proposalID, v)
		vp := sdk.StateGetObject(vk)
		if vp == nil {
			continue
		}
		var vr VoteRecord
		if err := json.Unmarshal([]byte(*vp), &vr); err == nil {
			out = append(out, vr)
		}
	}
	return out
}

// remove vote only needed if member leaves project while still voted on an active proposal
func removeVote(projectID, proposalID, voter string) {
	key := voteKey(projectID, proposalID, voter)
	sdk.StateDeleteObject(key)
	// remove from voter list
	votersKey := fmt.Sprintf("proposal:%s:%s:voters", projectID, proposalID)
	ptr := sdk.StateGetObject(votersKey)
	if ptr == nil {
		return
	}
	var voters []string
	json.Unmarshal([]byte(*ptr), &voters)
	newV := make([]string, 0, len(voters))
	for _, a := range voters {
		if a != voter {
			newV = append(newV, a)
		}
	}
	nb, _ := json.Marshal(newV)
	sdk.StateSetObject(votersKey, string(nb))
}
