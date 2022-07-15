package swap

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSwapId(t *testing.T) {
	swapId := NewSwapId()
	assert.Equal(t, len(swapId), 32)

	// Create new and compare if different
	swapId2 := NewSwapId()
	assert.NotEqual(t, swapId[:], swapId2[:])
}

func TestSwapId_String(t *testing.T) {
	swapId := NewSwapId()
	swapId2 := NewSwapId()
	assert.NotEqual(t, swapId.String(), swapId2.String())
}

func TestSwapId_FromString(t *testing.T) {
	swapId := NewSwapId()
	var goodSwapId *SwapId = new(SwapId)
	err := goodSwapId.FromString(swapId.String())
	assert.NoError(t, err)
	assert.Equal(t, swapId[:], goodSwapId[:])

	// Check wrong sizes
	var largeSwapId *SwapId = new(SwapId)
	var largeId [33]byte
	rand.Read(largeId[:])
	largeStr := base64.StdEncoding.EncodeToString(largeId[:])
	err = largeSwapId.FromString(largeStr)
	assert.Error(t, err)

	var smallSwapId *SwapId = new(SwapId)
	var smallId [31]byte
	rand.Read(smallId[:])
	smallStr := base64.StdEncoding.EncodeToString(smallId[:])
	err = smallSwapId.FromString(smallStr)
	assert.Error(t, err)
}

func TestSwapIdJson(t *testing.T) {
	sid := NewSwapId()
	b, err := json.Marshal(sid)
	assert.NoError(t, err)

	var sid2 *SwapId
	err = json.Unmarshal(b, &sid2)
	assert.NoError(t, err)
	assert.Equal(t, sid, sid2)
}
