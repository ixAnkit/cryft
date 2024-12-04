// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vm

import (
	"errors"
	"io"
	"testing"

	"github.com/ixAnkit/cryft/internal/mocks"
	"github.com/ixAnkit/cryft/pkg/application"
	"github.com/ixAnkit/cryft/pkg/ux"
	"github.com/MetalBlockchain/metalgo/utils/logging"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testToken = "TEST"

func setupTest(t *testing.T) *require.Assertions {
	// use io.Discard to not print anything
	ux.NewUserLog(logging.NoLog{}, io.Discard)
	return require.New(t)
}

func Test_getChainId(t *testing.T) {
	require := setupTest(t)
	app := application.New()
	mockPrompt := &mocks.Prompter{}
	app.Prompt = mockPrompt

	mockPrompt.On("CaptureString", mock.Anything).Return(testToken, nil)

	token, err := getTokenSymbol(app, "")
	require.NoError(err)
	require.Equal(testToken, token)
}

func Test_getChainId_Err(t *testing.T) {
	require := setupTest(t)
	app := application.New()
	mockPrompt := &mocks.Prompter{}
	app.Prompt = mockPrompt

	testErr := errors.New("Bad prompt")
	mockPrompt.On("CaptureString", mock.Anything).Return("", testErr)

	_, err := getTokenSymbol(app, "")
	require.ErrorIs(testErr, err)
}
