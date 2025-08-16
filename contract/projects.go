package main

import "okinoko_dao/sdk"

//go:wasmexport projects_create
func CreateProject(name, description, jsonMetadata string, cfg ProjectConfig, amount int64, asset sdk.Asset) string {
	if amount <= 0 {
		sdk.Log("CreateProject: amount must be > 1")
		return "XXX" // TODO: correct return
	}
	// TODO: add more asset checks here

	creator := getSenderAddress()

	sdk.HiveDraw(amount, sdk.Asset(asset))

	id := generateGUID()
	now := nowUnix()

	prj := Project{
		ID:           id,
		Owner:        creator,
		Name:         name,
		Description:  description,
		JsonMetadata: jsonMetadata,
		Config:       cfg,
		Members:      map[string]Member{},
		Funds:        amount,
		FundsAsset:   asset,
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
	// if it is stake based - add stake of the project fee as stake
	if cfg.VotingSystem == SystemStake {
		m.Stake = amount
	}

	prj.Members[creator] = m
	saveProject(&prj)
	addProjectToIndex(id)
	sdk.Log("CreateProject: " + id)
	return id
}

// GetProject - returns the project object (no proposals included)
//
//go:wasmexport projects_get_one
func GetProject(projectID string) *Project {
	prj, err := loadProject(projectID)
	if err != nil {
		return nil
	}
	return prj
}

// GetAllProjects - returns all projects (IDs then loads each)
//
//go:wasmexport projects_get_all
func GetAllProjects() []*Project {
	ids := listAllProjectIDs()
	out := make([]*Project, 0, len(ids))
	for _, id := range ids {
		if prj, err := loadProject(id); err == nil {
			out = append(out, prj)
		}
	}
	return out
}
