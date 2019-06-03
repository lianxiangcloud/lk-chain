package commands

import (
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/types"
	cmn "github.com/tendermint/tmlibs/common"
)

func init() {
	InitFilesCmd.Flags().String("chain_id", config.ChainID, "Blockchain id")
	InitFilesCmd.Flags().String("genesis", config.Genesis, "Blockchain genesis file")
}

// InitFilesCmd initialises a fresh Tendermint Core instance.
var InitFilesCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Tendermint",
	Run:   initFiles,
}

func initFiles(cmd *cobra.Command, args []string) {
	// private validator
	privValFile := config.PrivValidatorFile()
	var privValidator *types.PrivValidatorFS
	if cmn.FileExists(privValFile) {
		privValidator = types.LoadPrivValidatorFS(privValFile)
		logger.Info("Found private validator", "path", privValFile)
	} else {
		privValidator = types.GenPrivValidatorFS(privValFile)
		privValidator.LastHeight = config.BlockStartNum
		privValidator.Save()
		logger.Info("Genetated private validator", "path", privValFile)
	}

	// genesis file
	genFile := config.GenesisFile()
	if cmn.FileExists(genFile) {
		logger.Info("Found genesis file", "path", genFile)
	} else {
		genDoc := types.GenesisDoc{
			ChainID: cmn.Fmt("test-chain-%v", cmn.RandStr(6)),
		}
		genDoc.Validators = []types.GenesisValidator{
			{
				PubKey: privValidator.GetPubKey(),
				Power:  10,
			},
		}
		genDoc.BlockStartNum = config.BlockStartNum
		if len(config.ChainID) > 0 {
			genDoc.ChainID = config.ChainID
		}

		if err := genDoc.SaveAs(genFile); err != nil {
			panic(err)
		}
		logger.Info("Genetated genesis file", "path", genFile)
	}
}
