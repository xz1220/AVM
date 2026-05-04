package config

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

const agentIDPrefix = "agt_"

// NewAgentID returns a path-safe stable identity for an agent profile.
func NewAgentID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return agentIDPrefix + hex.EncodeToString(raw[:])
	}
	return agentIDPrefix + strconv.FormatInt(time.Now().UnixNano(), 16)
}
