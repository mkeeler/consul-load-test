package peering

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestPeeringConfig_Normalize(t *testing.T) {
	type testcase struct {
		config    UserConfig
		expectErr bool
	}
	testcases := map[string]testcase{
		"RegisterLimit must be non-zero": {
			config: UserConfig{
				RegisterLimit: rate.Limit(0.0),
				NumServices:   1,
			},
			expectErr: true,
		},
		"NumServices must be non-zero": {
			config: UserConfig{
				RegisterLimit: rate.Limit(1.0),
				NumServices:   0,
			},
			expectErr: true,
		},
		"Valid config": {
			config: UserConfig{
				RegisterLimit: rate.Limit(1.0),
				NumServices:   1,
			},
			expectErr: false,
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			if tc.expectErr {
				require.Error(t, tc.config.Normalize())
				return
			}
			require.NoError(t, tc.config.Normalize())
		})
	}
}
