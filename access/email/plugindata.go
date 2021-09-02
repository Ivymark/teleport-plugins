package main

import (
	"fmt"
	"strings"

	"github.com/gravitational/teleport/api/types"
)

// PluginData is a data associated with access request that we store in Teleport using UpdatePluginData API.
type PluginData struct {
	RequestData
	EmailThreads []EmailThread
}

// Resolution is a review request resolution result
type Resolution struct {
	Tag    ResolutionTag
	Reason string
}
type ResolutionTag string

const Unresolved = ResolutionTag("")
const ResolvedApproved = ResolutionTag("APPROVED")
const ResolvedDenied = ResolutionTag("DENIED")
const ResolvedExpired = ResolutionTag("EXPIRED")

// RequestData represents part of plugin data responsible for review request
type RequestData struct {
	User          string
	Roles         []string
	RequestReason string
	ReviewsCount  int
	Resolution    Resolution
}

// EmailThread stores value about particular original message
type EmailThread struct {
	Email     string
	MessageID string
	Timestamp string
}

// NewRequestData converts types.AccessRequest to RequestData
func NewRequestData(req types.AccessRequest) (string, RequestData) {
	id := req.GetName()
	data := RequestData{User: req.GetUser(), Roles: req.GetRoles(), RequestReason: req.GetRequestReason()}

	return id, data
}

// DecodePluginData deserializes a string map to PluginData struct.
func DecodePluginData(dataMap map[string]string) (data PluginData) {
	data.User = dataMap["user"]
	if str := dataMap["roles"]; str != "" {
		data.Roles = strings.Split(str, ",")
	}
	data.RequestReason = dataMap["request_reason"]
	if str := dataMap["reviews_count"]; str != "" {
		fmt.Sscanf(str, "%d", &data.ReviewsCount)
	}
	data.Resolution.Tag = ResolutionTag(dataMap["resolution"])
	data.Resolution.Reason = dataMap["resolve_reason"]
	if email, timestamp := dataMap["email"], dataMap["timestamp"]; email != "" && timestamp != "" {
		data.EmailThreads = append(data.EmailThreads, EmailThread{Email: email, Timestamp: timestamp})
	}
	if str := dataMap["email_threads"]; str != "" {
		for _, encodedThread := range strings.Split(str, ",") {
			if parts := strings.Split(encodedThread, "/"); len(parts) == 3 {
				data.EmailThreads = append(data.EmailThreads, EmailThread{Email: parts[0], Timestamp: parts[1], MessageID: parts[2]})
			}
		}
	}
	return
}

// EncodePluginData serializes a PluginData struct into a string map.
func EncodePluginData(data PluginData) map[string]string {
	result := make(map[string]string)

	result["user"] = data.User
	result["roles"] = strings.Join(data.Roles, ",")
	result["request_reason"] = data.RequestReason
	var reviewsCountStr string
	if data.ReviewsCount > 0 {
		reviewsCountStr = fmt.Sprintf("%d", data.ReviewsCount)
	}
	result["reviews_count"] = reviewsCountStr
	result["resolution"] = string(data.Resolution.Tag)
	result["resolve_reason"] = data.Resolution.Reason
	var emailThreads []string
	for _, t := range data.EmailThreads {
		emailThreads = append(emailThreads, fmt.Sprintf("%s/%s/%s", t.Email, t.Timestamp, t.MessageID))
	}
	result["email_threads"] = strings.Join(emailThreads, ",")

	return result
}
