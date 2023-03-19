package core

import (
	cryptoec "crypto/ecdsa"
	"encoding/hex"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sisu-network/lib/log"
	htypes "github.com/sodiumlabs/dheart/types"
	"github.com/sodiumlabs/dheart/utils"
	"github.com/sodiumlabs/dheart/worker/types"
	libCommon "github.com/sodiumlabs/tss-lib/common"
	"github.com/sodiumlabs/tss-lib/ecdsa/keygen"
	"github.com/sodiumlabs/tss-lib/tss"
)

func (engine *defaultEngine) onEcKeygenFinished(request *types.WorkRequest, output *keygen.LocalPartySaveData) {
	log.Info("Keygen finished for type ", request.KeygenType)

	pkX, pkY := output.ECDSAPub.X(), output.ECDSAPub.Y()
	publicKeyECDSA := cryptoec.PublicKey{
		Curve: tss.EC(tss.EcdsaScheme),
		X:     pkX,
		Y:     pkY,
	}
	publicKeyBytes := crypto.FromECDSAPub(&publicKeyECDSA)

	log.Verbose("publicKeyBytes length = ", len(publicKeyBytes))

	// Make a callback and start next work.
	result := htypes.KeygenResult{
		KeyType:     request.KeygenType,
		PubKeyBytes: publicKeyBytes,
		Outcome:     htypes.OutcomeSuccess,
	}

	engine.callback.OnWorkKeygenFinished(&result)
}

func (engine *defaultEngine) onEcSigningFinished(request *types.WorkRequest, data []*libCommon.ECSignature) {
	log.Infof("%s: Signing finished for Ecdsa workId %s", engine.myPid.Id[len(engine.myPid.Id)-4:],
		request.WorkId)

	signatures := make([][]byte, len(data))
	for i, sig := range data {
		bitSizeInBytes := tss.EC(tss.EcdsaScheme).Params().BitSize / 8
		r := utils.PadToLengthBytesForSignature(data[i].R, bitSizeInBytes)
		s := utils.PadToLengthBytesForSignature(data[i].S, bitSizeInBytes)

		signatures[i] = append(r, s...)
		signatures[i] = append(signatures[i], data[i].SignatureRecovery[0])

		if len(signatures[i]) != 65 {
			log.Error("Signatures length is not 65: hex of R,S,Recovery = ",
				hex.EncodeToString(sig.R),
				hex.EncodeToString(sig.S),
				hex.EncodeToString(data[i].SignatureRecovery),
			)
		}
	}

	result := &htypes.KeysignResult{
		Outcome:    htypes.OutcomeSuccess,
		Signatures: signatures,
	}

	engine.callback.OnWorkSigningFinished(request, result)
}
