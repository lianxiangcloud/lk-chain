package commands

import (
	"os"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tmlibs/cli"
	cfg "github.com/tendermint/tendermint/config"
)

var (
	defaultRoot = os.ExpandEnv("$HOME/.some/test/dir")
)

const (
	rootName = "root"
)

// isolate provides a clean setup and returns a copy of RootCmd you can
// modify in the test cases.
// NOTE: it unsets all TM* env variables.
func isolate(cmds ...*cobra.Command) cli.Executable {
	if err := os.Unsetenv("TMHOME"); err != nil {
		panic(err)
	}
	if err := os.Unsetenv("TM_HOME"); err != nil {
		panic(err)
	}
	if err := os.RemoveAll(defaultRoot); err != nil {
		panic(err)
	}

	viper.Reset()
	config = cfg.DefaultConfig()
	r := &cobra.Command{
		Use:               rootName,
		PersistentPreRunE: RootCmd.PersistentPreRunE,
	}
	r.AddCommand(cmds...)
	wr := cli.PrepareBaseCmd(r, "TM", defaultRoot)
	return wr
}

func TestRootConfig(t *testing.T) {
	assert, require := assert.New(t), require.New(t)

	// we pre-create a config file we can refer to in the rest of
	// the test cases.
	cvals := map[string]string{
		"moniker":   "monkey",
		"fast_sync": "false",
	}
	// proper types of the above settings
	cfast := false
	conf, err := cli.WriteDemoConfig(cvals)
	require.Nil(err)

	defaults := cfg.DefaultConfig()
	dmax := defaults.P2P.MaxNumPeers

	cases := []struct {
		args     []string
		env      map[string]string
		root     string
		moniker  string
		fastSync bool
		maxPeer  int
	}{
		{nil, nil, defaultRoot, defaults.Moniker, defaults.FastSync, dmax},
		// try multiple ways of setting root (two flags, cli vs. env)
		{[]string{"--home", conf}, nil, conf, cvals["moniker"], cfast, dmax},
		{nil, map[string]string{"TMHOME": conf}, conf, cvals["moniker"], cfast, dmax},
		// check setting p2p subflags two different ways
		{[]string{"--p2p.max_num_peers", "420"}, nil, defaultRoot, defaults.Moniker, defaults.FastSync, 420},
		{nil, map[string]string{"TM_P2P_MAX_NUM_PEERS": "17"}, defaultRoot, defaults.Moniker, defaults.FastSync, 17},
		// try to set env that have no flags attached...
		{[]string{"--home", conf}, map[string]string{"TM_MONIKER": "funny"}, conf, "funny", cfast, dmax},
	}

	for idx, tc := range cases {
		i := strconv.Itoa(idx)
		// test command that does nothing, except trigger unmarshalling in root
		noop := &cobra.Command{
			Use: "noop",
			RunE: func(cmd *cobra.Command, args []string) error {
				return nil
			},
		}
		noop.Flags().Int("p2p.max_num_peers", defaults.P2P.MaxNumPeers, "")
		cmd := isolate(noop)

		args := append([]string{rootName, noop.Use}, tc.args...)
		err := cli.RunWithArgs(cmd, args, tc.env)
		require.Nil(err, i)
		assert.Equal(tc.root, config.RootDir, i)
		assert.Equal(tc.root, config.P2P.RootDir, i)
		assert.Equal(tc.root, config.Consensus.RootDir, i)
		assert.Equal(tc.root, config.Mempool.RootDir, i)
		assert.Equal(tc.moniker, config.Moniker, i)
		assert.Equal(tc.fastSync, config.FastSync, i)
		assert.Equal(tc.maxPeer, config.P2P.MaxNumPeers, i)
	}
}
