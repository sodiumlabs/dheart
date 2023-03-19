package helper

import "github.com/sodiumlabs/tss-lib/tss"

func GetPidFromString(fromString string, pids []*tss.PartyID) *tss.PartyID {
	for _, pid := range pids {
		if pid.Id == fromString {
			return pid
		}
	}

	return nil
}
