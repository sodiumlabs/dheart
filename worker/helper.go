package worker

import (
	libCommon "github.com/sodiumlabs/tss-lib/common"
	"github.com/sodiumlabs/tss-lib/ecdsa/keygen"
	ecsigning "github.com/sodiumlabs/tss-lib/ecdsa/signing"
	edkeygen "github.com/sodiumlabs/tss-lib/eddsa/keygen"
	edsigning "github.com/sodiumlabs/tss-lib/eddsa/signing"
)

func GetEcKeygenOutputs(results []*JobResult) []*keygen.LocalPartySaveData {
	outputs := make([]*keygen.LocalPartySaveData, len(results))
	for i := range results {
		outputs[i] = results[i].EcKeygen
	}

	return outputs
}

func GetEcPresignOutputs(results []*JobResult) []*ecsigning.SignatureData_OneRoundData {
	outputs := make([]*ecsigning.SignatureData_OneRoundData, len(results))
	for i := range results {
		outputs[i] = results[i].EcPresign
	}

	return outputs
}

func GetEcSigningOutputs(results []*JobResult) []*libCommon.ECSignature {
	outputs := make([]*libCommon.ECSignature, len(results))
	for i := range results {
		outputs[i] = results[i].EcSigning.Signature
	}

	return outputs
}

func GetEdKeygenOutputs(results []*JobResult) []*edkeygen.LocalPartySaveData {
	outputs := make([]*edkeygen.LocalPartySaveData, len(results))
	for i := range results {
		outputs[i] = results[i].EdKeygen
	}

	return outputs
}

func GetEdSigningOutputs(results []*JobResult) []*edsigning.SignatureData {
	outputs := make([]*edsigning.SignatureData, len(results))
	for i := range results {
		outputs[i] = results[i].EdSigning
	}

	return outputs
}
