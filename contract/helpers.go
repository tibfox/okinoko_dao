package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"okinoko_dao/sdk"
	"strconv"
	"strings"
	"time"
)

////////////////////////////////////////////////////////////////////////////////
// Helpers: keys, guids, time
////////////////////////////////////////////////////////////////////////////////

// New struct for transfer.allow args
type TransferAllow struct {
	Limit int64
	Token sdk.Asset
}

// Helper function to validate token
func isValidAsset(token string) bool {
	for _, a := range validAssets {
		if token == a {
			return true
		}
	}
	return false
}

var validAssets = []string{sdk.AssetHbd.String(), sdk.AssetHive.String()}

// Helper function to get the first transfer.allow intent
func getFirstTransferAllow(intents []sdk.Intent) *TransferAllow {
	for _, intent := range intents {
		if intent.Type == "transfer.allow" {
			token := intent.Args["token"]
			if !isValidAsset(token) {
				abortCustom("invalid intent")
			}
			limitStr := intent.Args["limit"]
			limit, err := strconv.ParseInt(limitStr, 10, 64)
			if err != nil {
				abortCustom("invalid intent")
			}
			ta := &TransferAllow{
				Limit: limit,
				Token: sdk.Asset(token),
			}
			return ta

		}
	}
	return nil
}

func getSenderAddress() sdk.Address {
	return sdk.GetEnv().Sender.Address
}

func projectKey(id string) string {
	return "project:" + id
}

func proposalKey(id string) string {
	return "proposal:" + id
}

func voteKey(projectID, proposalID, voter string) string {
	return fmt.Sprintf("vote:%s:%s:%s", projectID, proposalID, voter)
}

// generateGUID returns a 16-byte hex string
func generateGUID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return fmt.Sprintf("g_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func nowUnix() int64 {
	// try chain timestamp via env key
	if tsPtr := sdk.GetEnvKey("block.timestamp"); tsPtr != nil && *tsPtr != "" {
		// try parse as integer seconds
		if v, err := strconv.ParseInt(*tsPtr, 10, 64); err == nil {
			return v
		}
		// try RFC3339
		if t, err := time.Parse(time.RFC3339, *tsPtr); err == nil {
			return t.Unix()
		}
	}
	return time.Now().Unix()
}

func getTxID() string {
	if t := sdk.GetEnvKey("tx.id"); t != nil {
		return *t
	}
	return ""
}

///////////////////////////////////////////////////
// Conversions from/to json strings
///////////////////////////////////////////////////

func ToJSON[T any](v T) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func FromJSON[T any](data string) (*T, error) {
	data = strings.TrimSpace(data)
	var v T
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func returnJsonResponse(success bool, data map[string]interface{}) *string {

	data["success"] = success

	jsonBytes, _ := json.Marshal(data)
	jsonStr := string(jsonBytes)

	return &jsonStr
}

func abortOnError(err error, message string) {
	if err != nil {
		abortCustom(fmt.Sprintf("%s: %v", message, err))
	}
}

func abortCustom(abortMessage string) {
	sdk.Abort(abortMessage)
}
