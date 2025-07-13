package tests

import (
	"errors"
	"testing"
	"tunnox-core/internal/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisposeErrorHandling(t *testing.T) {
	t.Run("Single Error Handler", func(t *testing.T) {
		dispose := &utils.Dispose{}

		testError := errors.New("test cleanup error")
		dispose.AddCleanHandler(func() error {
			return testError
		})

		result := dispose.Close()
		require.True(t, result.HasErrors())
		assert.Len(t, result.Errors, 1)
		assert.Equal(t, testError, result.Errors[0].Err)
		assert.Equal(t, 0, result.Errors[0].HandlerIndex)
	})

	t.Run("Backward Compatibility", func(t *testing.T) {
		dispose := &utils.Dispose{}

		dispose.AddCleanHandler(func() error {
			return errors.New("compatibility test error")
		})

		err := dispose.CloseWithError()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compatibility test error")
	})
}
