package load_test

import (
	"testing"

	"github.com/mkeeler/consul-load-test/load"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestConfig_Normalize(t *testing.T) {
	type testcase struct {
		config       load.Config
		expectTarget load.Target
		expectErr    bool
	}

	testcases := map[string]testcase{
		"config.KV and config.Peering are mutually exclusive": testcase{
			config: load.Config{
				KV:      &load.KVConfig{},
				Peering: &load.PeeringConfig{},
			},
			expectErr: true,
		},
		"returns error when config.KV is invalid": testcase{
			config: load.Config{
				KV: &load.KVConfig{NumKeys: 100},
			},
			expectErr: true,
		},
		"returns error when config.Peering is invalid": testcase{
			config: load.Config{
				Peering: &load.PeeringConfig{},
			},
			expectErr: true,
		},
		"config.Target is set to TargetKV when KV config set": testcase{
			config: load.Config{
				KV: &load.KVConfig{
					UpdateRate: rate.Limit(1.0),
				},
			},
			expectTarget: load.TargetKV,
		},
		"config.Target is set to TargetPeering when Peering config set": testcase{
			config: load.Config{
				Peering: &load.PeeringConfig{
					RegisterLimit: rate.Limit(1.0),
					NumServices:   10,
				},
			},
			expectTarget: load.TargetPeering,
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			if tc.expectErr {
				require.Error(t, tc.config.Normalize())
				return
			}
			require.NoError(t, tc.config.Normalize())
			require.Equal(t, tc.expectTarget, tc.config.Target)
		})
	}
}
