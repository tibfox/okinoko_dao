////////////////////////////////////////////////////////////////////////////////
// Okinoko DAO: A universal DAO for the vsc network
// created by tibfox 2025-08-12
////////////////////////////////////////////////////////////////////////////////

package main

// TODO: removing projects only via meta proposals?
// // RemoveProject - deletes project and all its proposals and votes (creator only)
// // projects_remove
// func RemoveProject(projectID string) error {
// 	env := sdk.GetEnv()
// 	caller := env.Sender.Address.String()

// 	prj, err := loadProject(projectID)
// 	if err != nil {
// 		return err
// 	}
// 	if caller != prj.Owner {
// 		return fmt.Errorf("only owner can remove")
// 	}
// 	// remove index entry
// 	ids := listAllProjectIDs()
// 	newIds := make([]string, 0, len(ids))
// 	for _, id := range ids {
// 		if id != projectID {
// 			newIds = append(newIds, id)
// 		}
// 	}
// 	nb, _ := json.Marshal(newIds)
// 	sdk.StateSetObject(projectsIndexKey, string(nb))

// 	// delete proposals & votes
// 	for _, pid := range listProposalIDsForProject(projectID) {
// 		// delete votes
// 		votersKey := fmt.Sprintf("proposal:%s:%s:voters", projectID, pid)
// 		ptr := sdk.StateGetObject(votersKey)
// 		if ptr != nil {
// 			var voters []string
// 			json.Unmarshal([]byte(*ptr), &voters)
// 			for _, v := range voters {
// 				sdk.StateDeleteObject(voteKey(projectID, pid, v))
// 			}
// 			sdk.StateDeleteObject(votersKey)
// 		}
// 		// delete proposal
// 		sdk.StateDeleteObject(proposalKey(pid))
// 	}
// 	// delete proposals index
// 	sdk.StateDeleteObject(projectProposalsIndexKey(projectID))
//  	// TODO: refund project members

// 	// delete project
// 	sdk.StateDeleteObject(projectKey(projectID))
// 	sdk.Log("RemoveProject: removed " + projectID)
// 	return nil
// }
