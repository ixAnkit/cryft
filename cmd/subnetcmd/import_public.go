// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package subnetcmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/MetalBlockchain/coreth/core"
	"github.com/ixAnkit/cryft/pkg/constants"
	"github.com/ixAnkit/cryft/pkg/models"
	"github.com/ixAnkit/cryft/pkg/networkoptions"
	"github.com/ixAnkit/cryft/pkg/utils"
	"github.com/ixAnkit/cryft/pkg/ux"
	"github.com/ixAnkit/cryft/pkg/vm"
	"github.com/MetalBlockchain/metalgo/api/info"
	"github.com/MetalBlockchain/metalgo/ids"
	"github.com/MetalBlockchain/metalgo/utils/rpc"
	"github.com/MetalBlockchain/metalgo/vms/platformvm"
	"github.com/MetalBlockchain/metalgo/vms/platformvm/txs"
	"github.com/spf13/cobra"
)

var (
	importPublicSupportedNetworkOptions = []networkoptions.NetworkOption{networkoptions.Tahoe, networkoptions.Mainnet}
	genesisFilePath                     string
	blockchainIDstr                     string
	nodeURL                             string
)

// avalanche subnet import public
func newImportPublicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "public [subnetPath]",
		Short:        "Import an existing subnet config from running subnets on a public network",
		RunE:         importPublic,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		Long: `The subnet import public command imports a Subnet configuration from a running network.

The genesis file should be available from the disk for this to work. By default, an imported Subnet
doesn't overwrite an existing Subnet with the same name. To allow overwrites, provide the --force
flag.`,
	}

	networkoptions.AddNetworkFlagsToCmd(cmd, &globalNetworkFlags, false, importPublicSupportedNetworkOptions)

	cmd.Flags().StringVar(&nodeURL, "node-url", "", "[optional] URL of an already running subnet validator")

	cmd.Flags().BoolVar(&useSubnetEvm, "evm", false, "import a subnet-evm")
	cmd.Flags().BoolVar(&useCustom, "custom", false, "use a custom VM template")
	cmd.Flags().BoolVar(
		&overwriteImport,
		"force",
		false,
		"overwrite the existing configuration if one exists",
	)
	cmd.Flags().StringVar(
		&genesisFilePath,
		"genesis-file-path",
		"",
		"path to the genesis file",
	)
	cmd.Flags().StringVar(
		&blockchainIDstr,
		"blockchain-id",
		"",
		"the blockchain ID",
	)
	return cmd
}

func importPublic(*cobra.Command, []string) error {
	network, err := networkoptions.GetNetworkFromCmdLineFlags(
		app,
		globalNetworkFlags,
		false,
		importPublicSupportedNetworkOptions,
		"",
	)
	if err != nil {
		return err
	}

	if genesisFilePath == "" {
		genesisFilePath, err = app.Prompt.CaptureExistingFilepath("Provide the path to the genesis file")
		if err != nil {
			return err
		}
	}

	var reply *info.GetNodeVersionReply

	if nodeURL == "" {
		yes, err := app.Prompt.CaptureYesNo("Have nodes already been deployed to this subnet?")
		if err != nil {
			return err
		}
		if yes {
			nodeURL, err = app.Prompt.CaptureString(
				"Please provide an API URL of such a node so we can query its VM version (e.g. http://111.22.33.44:5555)")
			if err != nil {
				return err
			}
			ctx, cancel := utils.GetAPIContext()
			defer cancel()
			infoAPI := info.NewClient(nodeURL)
			options := []rpc.Option{}
			reply, err = infoAPI.GetNodeVersion(ctx, options...)
			if err != nil {
				return fmt.Errorf("failed to query node - is it running and reachable? %w", err)
			}
		}
	}

	var blockchainID ids.ID
	if blockchainIDstr == "" {
		blockchainID, err = app.Prompt.CaptureID("What is the ID of the blockchain?")
		if err != nil {
			return err
		}
	} else {
		blockchainID, err = ids.FromString(blockchainIDstr)
		if err != nil {
			return err
		}
	}

	client := platformvm.NewClient(network.Endpoint)
	ctx, cancel := utils.GetAPIContext()
	defer cancel()
	options := []rpc.Option{}

	ux.Logger.PrintToUser("Getting information from the %s network...", network.Name())

	txBytes, err := client.GetTx(ctx, blockchainID, options...)
	if err != nil {
		return err
	}

	var (
		vmID, subnetID ids.ID
		tx             txs.Tx
		subnetName     string
	)

	_, err = txs.Codec.Unmarshal(txBytes, &tx)
	if err != nil {
		return fmt.Errorf("failed unmarshaling the createChainTx: %w", err)
	}

	createChainTx, ok := tx.Unsigned.(*txs.CreateChainTx)
	if !ok {
		return fmt.Errorf("expected a CreateChainTx, got %T", tx.Unsigned)
	}

	vmID = createChainTx.VMID
	subnetID = createChainTx.SubnetID
	subnetName = createChainTx.ChainName

	ux.Logger.PrintToUser("Retrieved information. BlockchainID: %s, SubnetID: %s, Name: %s, VMID: %s",
		blockchainID.String(),
		subnetID.String(),
		subnetName,
		vmID.String(),
	)
	// TODO: it's probably possible to deploy VMs with the same name on a public network
	// In this case, an import could clash because the tool supports unique names only

	genBytes, err := os.ReadFile(genesisFilePath)
	if err != nil {
		return err
	}

	if err = app.WriteGenesisFile(subnetName, genBytes); err != nil {
		return err
	}

	vmType := getVMFromFlag()
	if vmType == "" {
		subnetTypeStr, err := app.Prompt.CaptureList(
			"What's this VM's type?",
			[]string{models.SubnetEvm, models.CustomVM},
		)
		if err != nil {
			return err
		}
		vmType = models.VMTypeFromString(subnetTypeStr)
	}

	vmIDstr := vmID.String()

	sc := &models.Sidecar{
		Name: subnetName,
		VM:   vmType,
		Networks: map[string]models.NetworkData{
			network.Name(): {
				SubnetID:     subnetID,
				BlockchainID: blockchainID,
			},
		},
		Subnet:       subnetName,
		Version:      constants.SidecarVersion,
		TokenName:    constants.DefaultTokenName,
		TokenSymbol:  constants.DefaultTokenSymbol,
		ImportedVMID: vmIDstr,
		// signals that the VMID wasn't derived from the subnet name but through import
		ImportedFromAPM: true,
	}

	var versions []string

	if reply != nil {
		// a node was queried
		for _, v := range reply.VMVersions {
			if v == vmIDstr {
				sc.VMVersion = v
				break
			}
		}
		sc.RPCVersion = int(reply.RPCProtocolVersion)
	} else {
		// no node was queried, ask the user
		switch vmType {
		case models.SubnetEvm:
			versions, err = app.Downloader.GetAllReleasesForRepo(constants.AvaLabsOrg, constants.SubnetEVMRepoName)
			if err != nil {
				return err
			}
			sc.VMVersion, err = app.Prompt.CaptureList("Pick the version for this VM", versions)
		case models.CustomVM:
			return fmt.Errorf("importing custom VMs is not yet implemented, but will be available soon")
		default:
			return fmt.Errorf("unexpected VM type: %v", vmType)
		}
		if err != nil {
			return err
		}
		sc.RPCVersion, err = vm.GetRPCProtocolVersion(app, vmType, sc.VMVersion)
		if err != nil {
			return fmt.Errorf("failed getting RPCVersion for VM type %s with version %s", vmType, sc.VMVersion)
		}
	}
	if vmType == models.SubnetEvm {
		var genesis core.Genesis
		if err := json.Unmarshal(genBytes, &genesis); err != nil {
			return err
		}
		sc.ChainID = genesis.Config.ChainID.String()
	}

	if err := app.CreateSidecar(sc); err != nil {
		return fmt.Errorf("failed creating the sidecar for import: %w", err)
	}

	ux.Logger.PrintToUser("Subnet %q imported successfully", sc.Name)

	return nil
}
