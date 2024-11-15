// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package subnetcmd

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"

	"github.com/ixAnkit/cryft/pkg/constants"
	"github.com/ixAnkit/cryft/pkg/key"
	"github.com/ixAnkit/cryft/pkg/models"
	"github.com/ixAnkit/cryft/pkg/networkoptions"
	"github.com/ixAnkit/cryft/pkg/subnet"
	"github.com/ixAnkit/cryft/pkg/utils"
	"github.com/ixAnkit/cryft/pkg/ux"
	"github.com/ixAnkit/cryft/pkg/vm"
	anr_utils "github.com/MetalBlockchain/metal-network-runner/utils"
	"github.com/MetalBlockchain/metalgo/ids"
	"github.com/MetalBlockchain/metalgo/utils/logging"
	"github.com/MetalBlockchain/subnet-evm/core"
	"github.com/MetalBlockchain/subnet-evm/params"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/deployerallowlist"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/feemanager"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/nativeminter"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/rewardmanager"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/txallowlist"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/warp"
	"github.com/ethereum/go-ethereum/common"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var printGenesisOnly bool

// avalanche subnet describe
func newDescribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe [subnetName]",
		Short: "Print a summary of the subnet’s configuration",
		Long: `The subnet describe command prints the details of a Subnet configuration to the console.
By default, the command prints a summary of the configuration. By providing the --genesis
flag, the command instead prints out the raw genesis file.`,
		RunE: readGenesis,
		Args: cobra.ExactArgs(1),
	}
	cmd.Flags().BoolVarP(
		&printGenesisOnly,
		"genesis",
		"g",
		false,
		"Print the genesis to the console directly instead of the summary",
	)
	return cmd
}

func printGenesis(sc models.Sidecar, subnetName string) error {
	genesisFile := app.GetGenesisPath(subnetName)
	gen, err := os.ReadFile(genesisFile)
	if err != nil {
		return err
	}
	fmt.Println(string(gen))
	if sc.SubnetEVMMainnetChainID != 0 {
		fmt.Printf("Genesis is set to be deployed to Mainnet with Chain Id %d\n", sc.SubnetEVMMainnetChainID)
	}
	return nil
}

func printDetails(genesis core.Genesis, sc models.Sidecar) error {
	const art = `
 _____       _        _ _
|  __ \     | |      (_) |
| |  | | ___| |_ __ _ _| |___
| |  | |/ _ \ __/ _` + `  | | / __|
| |__| |  __/ || (_| | | \__ \
|_____/ \___|\__\__,_|_|_|___/
`
	fmt.Print(logging.LightBlue.Wrap(art))
	table := tablewriter.NewWriter(os.Stdout)
	header := []string{"Parameter", "Value"}
	table.SetHeader(header)
	table.SetRowLine(true)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoMergeCellsByColumnIndex([]int{0})

	table.Append([]string{"Subnet Name", sc.Subnet})
	table.Append([]string{"ChainID", genesis.Config.ChainID.String()})
	if sc.SubnetEVMMainnetChainID != 0 {
		table.Append([]string{"Mainnet ChainID", fmt.Sprint(sc.SubnetEVMMainnetChainID)})
	}
	table.Append([]string{"Token Name", app.GetTokenName(sc.Subnet)})
	table.Append([]string{"Token Symbol", app.GetTokenSymbol(sc.Subnet)})
	table.Append([]string{"VM Version", sc.VMVersion})
	if sc.ImportedVMID != "" {
		table.Append([]string{"VM ID", sc.ImportedVMID})
	} else {
		id := constants.NotAvailableLabel
		vmID, err := anr_utils.VMID(sc.Name)
		if err == nil {
			id = vmID.String()
		}
		table.Append([]string{"VM ID", id})
	}

	for net, data := range sc.Networks {
		network, err := networkoptions.GetNetworkFromSidecarNetworkName(app, net)
		if err != nil {
			return err
		}
		if data.SubnetID != ids.Empty {
			table.Append([]string{fmt.Sprintf("%s SubnetID", net), data.SubnetID.String()})
		}
		if data.BlockchainID != ids.Empty {
			table.Append([]string{fmt.Sprintf("%s RPC URL", net), network.BlockchainEndpoint(data.BlockchainID.String())})
			if network.Kind == models.Local {
				codespaceURL, err := utils.GetCodespaceURL(network.BlockchainEndpoint(data.BlockchainID.String()))
				if err != nil {
					return err
				}
				if codespaceURL != "" {
					table.Append([]string{"Codespace RPC URL", codespaceURL})
				}
			}
			hexEncoding := "0x" + hex.EncodeToString(data.BlockchainID[:])
			table.Append([]string{fmt.Sprintf("%s BlockchainID", net), data.BlockchainID.String()})
			table.Append([]string{fmt.Sprintf("%s BlockchainID", net), hexEncoding})
		}
		if data.TeleporterMessengerAddress != "" {
			table.Append([]string{fmt.Sprintf("%s Teleporter Messenger Address", net), data.TeleporterMessengerAddress})
		}
		if data.TeleporterRegistryAddress != "" {
			table.Append([]string{fmt.Sprintf("%s Teleporter Registry Address", net), data.TeleporterRegistryAddress})
		}
	}
	table.Render()
	return nil
}

func printGasTable(genesis core.Genesis) {
	// Generated here with BIG font
	// https://patorjk.com/software/taag/#p=display&f=Big&t=Precompiles
	const art = `
  _____              _____             __ _
 / ____|            / ____|           / _(_)
| |  __  __ _ ___  | |     ___  _ __ | |_ _  __ _
| | |_ |/ _` + `  / __| | |    / _ \| '_ \|  _| |/ _` + `  |
| |__| | (_| \__ \ | |___| (_) | | | | | | | (_| |
 \_____|\__,_|___/  \_____\___/|_| |_|_| |_|\__, |
                                             __/ |
                                            |___/
`

	fmt.Print(logging.LightBlue.Wrap(art))
	table := tablewriter.NewWriter(os.Stdout)
	header := []string{"Gas Parameter", "Value"}
	table.SetHeader(header)
	table.SetRowLine(true)

	table.Append([]string{"GasLimit", genesis.Config.FeeConfig.GasLimit.String()})
	table.Append([]string{"MinBaseFee", genesis.Config.FeeConfig.MinBaseFee.String()})
	table.Append([]string{"TargetGas (per 10s)", genesis.Config.FeeConfig.TargetGas.String()})
	table.Append([]string{"BaseFeeChangeDenominator", genesis.Config.FeeConfig.BaseFeeChangeDenominator.String()})
	table.Append([]string{"MinBlockGasCost", genesis.Config.FeeConfig.MinBlockGasCost.String()})
	table.Append([]string{"MaxBlockGasCost", genesis.Config.FeeConfig.MaxBlockGasCost.String()})
	table.Append([]string{"TargetBlockRate", strconv.FormatUint(genesis.Config.FeeConfig.TargetBlockRate, 10)})
	table.Append([]string{"BlockGasCostStep", genesis.Config.FeeConfig.BlockGasCostStep.String()})

	table.Render()
}

func printAirdropTable(genesis core.Genesis, sc models.Sidecar) error {
	const art = `
          _         _
    /\   (_)       | |
   /  \   _ _ __ __| |_ __ ___  _ __
  / /\ \ | | '__/ _` + `  | '__/ _ \| '_ \
 / ____ \| | | | (_| | | | (_) | |_) |
/_/    \_\_|_|  \__,_|_|  \___/| .__/
                               | |
                               |_|
`
	fmt.Print(logging.LightBlue.Wrap(art))
	teleporterKeyAddress := ""
	teleporterPrivKey := ""
	if sc.TeleporterReady {
		k, err := key.LoadSoft(models.NewLocalNetwork().ID, app.GetKeyPath(sc.TeleporterKey))
		if err != nil {
			return err
		}
		teleporterKeyAddress = k.C()
		teleporterPrivKey = hex.EncodeToString(k.Raw())
	}
	subnetAirdropKeyName, subnetAirdropAddress, subnetAirdropPrivKey, err := subnet.GetSubnetAirdropKeyInfo(app, sc.Name)
	if err != nil {
		return err
	}
	if len(genesis.Alloc) > 0 {
		table := tablewriter.NewWriter(os.Stdout)
		header := []string{"Description", "Address", "Airdrop Amount (10^18)", "Airdrop Amount (wei)", "Private Key"}
		table.SetHeader(header)
		table.SetRowLine(true)

		for address := range genesis.Alloc {
			amount := genesis.Alloc[address].Balance
			formattedAmount := new(big.Int).Div(amount, big.NewInt(params.Ether))
			description := ""
			privKey := ""
			switch address.Hex() {
			case teleporterKeyAddress:
				description = fmt.Sprintf("Teleporter deploys %s", sc.TeleporterKey)
				privKey = teleporterPrivKey
			case subnetAirdropAddress:
				description = fmt.Sprintf("Main funded account %s", subnetAirdropKeyName)
				privKey = subnetAirdropPrivKey
			case vm.PrefundedEwoqAddress.Hex():
				description = "Main funded account EWOQ"
				privKey = vm.PrefundedEwoqPrivate
			}
			table.Append([]string{description, address.Hex(), formattedAmount.String(), amount.String(), privKey})
		}

		table.Render()
	} else {
		fmt.Printf("No airdrops allocated")
	}
	return nil
}

func printPrecompileTable(genesis core.Genesis) {
	const art = `

  _____                                    _ _
 |  __ \                                  (_) |
 | |__) | __ ___  ___ ___  _ __ ___  _ __  _| | ___  ___
 |  ___/ '__/ _ \/ __/ _ \| '_ ` + `  _ \| '_ \| | |/ _ \/ __|
 | |   | | |  __/ (_| (_) | | | | | | |_) | | |  __/\__ \
 |_|   |_|  \___|\___\___/|_| |_| |_| .__/|_|_|\___||___/
                                    | |
                                    |_|

`
	fmt.Print(logging.LightBlue.Wrap(art))

	table := tablewriter.NewWriter(os.Stdout)
	header := []string{"Precompile", "Admin Addresses", "Enabled Addresses"}
	table.SetHeader(header)
	table.SetAutoMergeCellsByColumnIndex([]int{0, 1, 2})
	table.SetRowLine(true)

	precompileSet := false

	// Warp
	if genesis.Config.GenesisPrecompiles[warp.ConfigKey] != nil {
		table.Append([]string{"Warp", "n/a", "n/a"})
		precompileSet = true
	}

	// Native Minting
	if genesis.Config.GenesisPrecompiles[nativeminter.ConfigKey] != nil {
		cfg := genesis.Config.GenesisPrecompiles[nativeminter.ConfigKey].(*nativeminter.Config)
		appendToAddressTable(table, "Native Minter", cfg.AdminAddresses, cfg.EnabledAddresses)
		precompileSet = true
	}

	// Contract allow list
	if genesis.Config.GenesisPrecompiles[deployerallowlist.ConfigKey] != nil {
		cfg := genesis.Config.GenesisPrecompiles[deployerallowlist.ConfigKey].(*deployerallowlist.Config)
		appendToAddressTable(table, "Contract Allow List", cfg.AdminAddresses, cfg.EnabledAddresses)
		precompileSet = true
	}

	// TX allow list
	if genesis.Config.GenesisPrecompiles[txallowlist.ConfigKey] != nil {
		cfg := genesis.Config.GenesisPrecompiles[txallowlist.Module.ConfigKey].(*txallowlist.Config)
		appendToAddressTable(table, "Tx Allow List", cfg.AdminAddresses, cfg.EnabledAddresses)
		precompileSet = true
	}

	// Fee config allow list
	if genesis.Config.GenesisPrecompiles[feemanager.ConfigKey] != nil {
		cfg := genesis.Config.GenesisPrecompiles[feemanager.ConfigKey].(*feemanager.Config)
		appendToAddressTable(table, "Fee Config Allow List", cfg.AdminAddresses, cfg.EnabledAddresses)
		precompileSet = true
	}

	// Reward config allow list
	if genesis.Config.GenesisPrecompiles[rewardmanager.ConfigKey] != nil {
		cfg := genesis.Config.GenesisPrecompiles[rewardmanager.ConfigKey].(*rewardmanager.Config)
		appendToAddressTable(table, "Reward Manager Allow List", cfg.AdminAddresses, cfg.EnabledAddresses)
		precompileSet = true
	}

	if precompileSet {
		table.Render()
	} else {
		ux.Logger.PrintToUser("No precompiles set")
	}
}

func appendToAddressTable(
	table *tablewriter.Table,
	label string,
	adminAddresses []common.Address,
	enabledAddresses []common.Address,
) {
	admins := len(adminAddresses)
	enabled := len(enabledAddresses)
	max := int(math.Max(float64(admins), float64(enabled)))
	for i := 0; i < max; i++ {
		var admin, enable string
		if len(adminAddresses) >= i+1 && adminAddresses[i] != (common.Address{}) {
			admin = adminAddresses[i].Hex()
		}
		if len(enabledAddresses) >= i+1 && enabledAddresses[i] != (common.Address{}) {
			enable = enabledAddresses[i].Hex()
		}
		table.Append([]string{label, admin, enable})
	}
}

func describeSubnetEvmGenesis(sc models.Sidecar) error {
	// Load genesis
	genesis, err := app.LoadEvmGenesis(sc.Subnet)
	if err != nil {
		return err
	}

	if err := printDetails(genesis, sc); err != nil {
		return err
	}
	// Write gas table
	printGasTable(genesis)
	// fmt.Printf("\n\n")
	if err := printAirdropTable(genesis, sc); err != nil {
		return err
	}
	printPrecompileTable(genesis)
	return nil
}

func readGenesis(_ *cobra.Command, args []string) error {
	subnetName := args[0]
	if !app.GenesisExists(subnetName) {
		ux.Logger.PrintToUser("The provided subnet name %q does not exist", subnetName)
		return nil
	}
	sc, err := app.LoadSidecar(subnetName)
	if err != nil {
		return err
	}
	if printGenesisOnly {
		return printGenesis(sc, subnetName)
	}

	isEVM, err := HasSubnetEVMGenesis(subnetName)
	if err != nil {
		return err
	}
	if isEVM {
		return describeSubnetEvmGenesis(sc)
	}
	app.Log.Warn("Unknown genesis format", zap.Any("vm-type", sc.VM))
	ux.Logger.PrintToUser("Printing genesis")
	return printGenesis(sc, subnetName)
}
