package main

// maintaining index keys for querying data in various ways

import (
	"encoding/json"
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
)

// index key prefixes
const (
	maxChunkSize              = 2500                 // all indexes are split into chunks of X entries to avoid overflowing the max size of a key/value in the contract state
	idxProjects               = "proj:"              // 		// holds all projects
	idxMembers                = "mem:proj:"          // + member		// holds all open projects a user is member of
	idxProjectProposalsOpen   = "proj:props:open:"   // + projectId		// holds all open proposals for a given project
	idxProjectProposalsClosed = "proj:props:closed:" // + projectId		// holds all closed proposals for a given project
	idxProposalVotes          = "prop:v:"            // + proposalId		// holds all votes for a given proposal
	VotesCount                = "count:v"            // 					// holds a int counter for votes (to create new ids)
	ProposalsCount            = "count:props"        // 					// holds a int counter for proposals (to create new ids)
	ProjectsCount             = "count:proj"         // 					// holds a int counter for projects (to create new ids)

)

// oss stores number of chunks for a base index
func chunkCounterKey(base string) string {
	return base + ":chunks"
}

func chunkKey(base string, chunk int) string {
	return base + ":" + strconv.Itoa(chunk)
}

// get number of chunks for an index
func getChunkCount(baseKey string) int {
	ptr := sdk.StateGetObject(chunkCounterKey(baseKey))
	if ptr == nil || *ptr == "" {
		return 0
	}
	n, _ := strconv.Atoi(*ptr)
	return n
}

// set number of chunks
func setChunkCount(baseKey string, n int) {
	sdk.StateSetObject(chunkCounterKey(baseKey), strconv.Itoa(n))
}

// AddIDToIndex ensures id exists across all chunks (no duplicates).
func AddIDToIndex(baseKey string, id int64) {
	chunks := getChunkCount(baseKey)
	// search existing chunks for duplicates or free space
	for i := 0; i < chunks; i++ {
		key := chunkKey(baseKey, i)
		ptr := sdk.StateGetObject(key)
		var ids []int64
		if ptr != nil && *ptr != "" {
			if err := json.Unmarshal([]byte(*ptr), &ids); err != nil {
				sdk.Abort(fmt.Sprintf("unmarshal index %s: %w", key, err))

			}
			// duplicate check
			for _, e := range ids {
				if e == id {
					return // already present
				}
			}
			// append if space
			if len(ids) < maxChunkSize {
				ids = append(ids, id)
				b, err := json.Marshal(ids)
				if err != nil {
					sdk.Abort(fmt.Sprintf("marshal index %s: %w", key, err))
				}
				sdk.StateSetObject(key, string(b))
				return
			}
		}
	}
	// not found / no space -> create new chunk
	key := chunkKey(baseKey, chunks)
	ids := []int64{id}
	b, err := json.Marshal(ids)
	if err != nil {
		sdk.Abort(fmt.Sprintf("marshal index %s: %w", key, err))
	}
	sdk.StateSetObject(key, string(b))
	setChunkCount(baseKey, chunks+1)
	return
}

// RemoveIDFromIndex removes id from whichever chunk it’s in.
func RemoveIDFromIndex(baseKey string, id int64) {
	chunks := getChunkCount(baseKey)
	for i := 0; i < chunks; i++ {
		key := chunkKey(baseKey, i)
		ptr := sdk.StateGetObject(key)
		if ptr == nil || *ptr == "" {
			continue
		}
		var ids []int64
		if err := json.Unmarshal([]byte(*ptr), &ids); err != nil {
			sdk.Abort(fmt.Sprintf("unmarshal index %s: %w", key, err))

		}
		newIds := ids[:0]
		found := false
		for _, e := range ids {
			if e == id {
				found = true
				continue
			}
			newIds = append(newIds, e)
		}
		if found {
			// save updated chunk
			b, err := json.Marshal(newIds)
			if err != nil {
				sdk.Abort(fmt.Sprintf("marshal index %s: %w", key, err))
			}
			sdk.StateSetObject(key, string(b))

		}
	}

}

// GetIDsFromIndex collects all IDs across all chunks.
func GetIDsFromIndex(baseKey string) []int64 {
	all := []int64{}
	chunks := getChunkCount(baseKey)
	for i := 0; i < chunks; i++ {
		key := chunkKey(baseKey, i)
		ptr := sdk.StateGetObject(key)
		if ptr == nil || *ptr == "" {
			continue
		}
		var ids []int64
		if err := json.Unmarshal([]byte(*ptr), &ids); err != nil {
			sdk.Abort(fmt.Sprintf("unmarshal index %s: %w", key, err))
			return nil // will not happen because of error
		}
		all = append(all, ids...)
	}
	return all
}

// GetOneIDFromIndex checks all chunks for a specific id.
func GetOneIDFromIndex(baseKey string, id int64) (*int64, error) {
	chunks := getChunkCount(baseKey)
	for i := 0; i < chunks; i++ {
		key := chunkKey(baseKey, i)
		ptr := sdk.StateGetObject(key)
		if ptr == nil || *ptr == "" {
			continue
		}
		var ids []int64
		if err := json.Unmarshal([]byte(*ptr), &ids); err != nil {
			return nil, fmt.Errorf("unmarshal index %s: %w", key, err)
		}
		for _, v := range ids {
			if v == id {
				return &id, nil
			}
		}
	}
	return nil, nil
}

// updateBoolIndex ensures the objectId is in the correct boolean index chunk
func updateBoolIndex(baseKey string, objectId int64, targetBool bool) {
	// remove from the opposite boolean index
	oppositeKey := baseKey + strconv.FormatBool(!targetBool)
	RemoveIDFromIndex(oppositeKey, objectId)
	// add to the correct boolean index
	correctKey := baseKey + strconv.FormatBool(targetBool)
	AddIDToIndex(correctKey, objectId)
}

func getCount(key string) int64 {
	ptr := sdk.StateGetObject(key)
	if ptr == nil || *ptr == "" {
		return 0
	}
	n, _ := strconv.ParseInt(*ptr, 10, 64)
	return n
}

func setCount(key string, n int64) {
	sdk.StateSetObject(key, strconv.FormatInt(n, 10))
}
