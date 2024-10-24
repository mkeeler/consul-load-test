package load_test

import (
	"testing"

	"github.com/mkeeler/consul-load-test/load"
	"github.com/mkeeler/consul-load-test/load/kv"
	"github.com/mkeeler/consul-load-test/load/peering"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestConfig_Normalize(t *testing.T) {
	type testcase struct {
		config    load.Config
		expectErr bool
	}

	testcases := map[string]testcase{
		"returns error when config.KV is invalid": testcase{
			config: load.Config{
				KV: &kv.UserConfig{NumKeys: 100},
			},
			expectErr: true,
		},
		"returns error when config.Peering is invalid": testcase{
			config: load.Config{
				Peering: &peering.UserConfig{},
			},
			expectErr: true,
		},
		"config.Target is set to TargetKV when KV config set": testcase{
			config: load.Config{
				KV: &kv.UserConfig{
					UpdateRate: rate.Limit(1.0),
				},
			},
		},
		"config.Target is set to TargetPeering when Peering config set": testcase{
			config: load.Config{
				Peering: &peering.UserConfig{
					RegisterLimit: rate.Limit(1.0),
					NumServices:   10,
				},
			},
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
