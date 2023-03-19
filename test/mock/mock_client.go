package mock

import (
	"github.com/sodiumlabs/dheart/types"
)

// TODO: Use mock gen instead
type MockClient struct {
	TryDialFunc           func()
	PostKeygenResultFunc  func(result *types.KeygenResult) error
	PostPresignResultFunc func(result *types.PresignResult) error
	PostKeysignResultFunc func(result *types.KeysignResult) error
}

func (m *MockClient) TryDial() {
	if m.TryDialFunc != nil {
		m.TryDialFunc()
	}
}

func (m *MockClient) PostKeygenResult(result *types.KeygenResult) error {
	if m.PostKeygenResultFunc != nil {
		return m.PostKeygenResultFunc(result)
	}

	return nil
}

func (m *MockClient) PostKeysignResult(result *types.KeysignResult) error {
	if m.PostKeysignResultFunc != nil {
		return m.PostKeysignResultFunc(result)
	}

	return nil
}

func (m *MockClient) PostPresignResult(result *types.PresignResult) error {
	if m.PostPresignResultFunc != nil {
		return m.PostPresignResultFunc(result)
	}

	return nil
}
