package interfaces

import (
	"github.com/sodiumlabs/dheart/types/common"
	"github.com/sodiumlabs/tss-lib/tss"
)

// Dispatch messages to the destination through network.
type MessageDispatcher interface {
	// Broadcast a message to everyone in a list.
	BroadcastMessage(pIDs []*tss.PartyID, tssMessage *common.TssMessage)

	// Send a message to a single destination.
	UnicastMessage(dest *tss.PartyID, tssMessage *common.TssMessage)
}
