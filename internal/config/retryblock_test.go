package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nolint:dupl
func TestRetryBlock_MaxAttemptsValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		block      *RetryBlock
		expected   int
		expectErr  bool
		errContain string
	}{
		{
			name:      "NilReceiver",
			block:     nil,
			expected:  3,
			expectErr: false,
		},
		{
			name: "MaxAttemptsZero",
			block: &RetryBlock{
				MaxAttempts: newInt(0),
			},
			expectErr:  true,
			errContain: "max_attempts must be >= 1",
		},
		{
			name: "MaxAttemptsNegative",
			block: &RetryBlock{
				MaxAttempts: newInt(-1),
			},
			expectErr:  true,
			errContain: "max_attempts must be >= 1",
		},
		{
			name: "MaxAttemptsNil",
			block: &RetryBlock{
				MaxAttempts: nil,
			},
			expected:  3,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.block.MaxAttemptsValue()
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

// nolint:dupl
func TestRetryBlock_InitialIntervalValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		block      *RetryBlock
		expected   time.Duration
		expectErr  bool
		errContain string
	}{
		{
			name:      "NilReceiver",
			block:     nil,
			expected:  DefaultInitialInterval,
			expectErr: false,
		},
		{
			name: "InitialIntervalZero",
			block: &RetryBlock{
				InitialInterval: newDuration(0),
			},
			expectErr:  true,
			errContain: "initial_interval must be > 0",
		},
		{
			name: "InitialIntervalNegative",
			block: &RetryBlock{
				InitialInterval: newDuration(-5 * time.Second),
			},
			expectErr:  true,
			errContain: "initial_interval must be > 0",
		},
		{
			name: "InitialIntervalNil",
			block: &RetryBlock{
				InitialInterval: nil,
			},
			expected:  DefaultInitialInterval,
			expectErr: false,
		},
		{
			name: "InitialIntervalPositive",
			block: &RetryBlock{
				InitialInterval: newDuration(5 * time.Second),
			},
			expected:  5 * time.Second,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.block.InitialIntervalValue()
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

// nolint:dupl
func TestRetryBlock_MaxIntervalValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		block      *RetryBlock
		expected   time.Duration
		expectErr  bool
		errContain string
	}{
		{
			name:      "NilReceiver",
			block:     nil,
			expected:  DefaultMaxInterval,
			expectErr: false,
		},
		{
			name: "MaxIntervalZero",
			block: &RetryBlock{
				MaxInterval: newDuration(0),
			},
			expectErr:  true,
			errContain: "max_interval must be > 0",
		},
		{
			name: "MaxIntervalNegative",
			block: &RetryBlock{
				MaxInterval: newDuration(-10 * time.Second),
			},
			expectErr:  true,
			errContain: "max_interval must be > 0",
		},
		{
			name: "MaxIntervalNil",
			block: &RetryBlock{
				MaxInterval: nil,
			},
			expected:  DefaultMaxInterval,
			expectErr: false,
		},
		{
			name: "MaxIntervalPositive",
			block: &RetryBlock{
				MaxInterval: newDuration(30 * time.Second),
			},
			expected:  30 * time.Second,
			expectErr: false,
		},
		{
			name: "InitialExceedsDefaultMaxInterval",
			block: &RetryBlock{
				InitialInterval: newDuration(60 * time.Second),
				MaxInterval:     nil,
			},
			expectErr:  true,
			errContain: "max_interval (30s) must be >= initial_interval (1m0s)",
		},
		{
			name: "InitialExceedsConfiguredMaxInterval",
			block: &RetryBlock{
				InitialInterval: newDuration(20 * time.Second),
				MaxInterval:     newDuration(10 * time.Second),
			},
			expectErr:  true,
			errContain: "max_interval (10s) must be >= initial_interval (20s)",
		},
		{
			name: "InitialEqualsMax",
			block: &RetryBlock{
				InitialInterval: newDuration(10 * time.Second),
				MaxInterval:     newDuration(10 * time.Second),
			},
			expected:  10 * time.Second,
			expectErr: false,
		},
		{
			name: "InitialLessThanMax",
			block: &RetryBlock{
				InitialInterval: newDuration(5 * time.Second),
				MaxInterval:     newDuration(30 * time.Second),
			},
			expected:  30 * time.Second,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.block.MaxIntervalValue()
			if tt.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func newInt(v int) *int {
	return &v
}

func newDuration(v time.Duration) *time.Duration {
	return &v
}
