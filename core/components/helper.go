package components

import (
	"github.com/sodiumlabs/dheart/db"
	ecsigning "github.com/sodiumlabs/tss-lib/ecdsa/signing"
)

func GetMokDbForAvailManager(presignPids, pids []string) db.Database {
	return &db.MockDatabase{
		GetAvailablePresignShortFormFunc: func() ([]string, []string, error) {
			return presignPids, pids, nil
		},

		LoadPresignFunc: func(presignIds []string) ([]*ecsigning.SignatureData_OneRoundData, error) {
			return make([]*ecsigning.SignatureData_OneRoundData, len(presignIds)), nil
		},
	}
}
