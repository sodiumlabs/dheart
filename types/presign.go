package types

import "github.com/sodiumlabs/tss-lib/tss"

type PresignResult struct {
	Outcome OutcomeType

	PubKeyBytes []byte
	Address     string
	Culprits    []*tss.PartyID
}
