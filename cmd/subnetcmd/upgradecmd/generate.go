// Copyright (C) 2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package upgradecmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/MetalBlockchain/coreth/ethclient"
	"github.com/ixAnkit/cryft/pkg/models"
	"github.com/MetalBlockchain/metalgo/utils/units"
	"go.uber.org/zap"

	"github.com/ixAnkit/cryft/pkg/constants"
	"github.com/ixAnkit/cryft/pkg/prompts"
	"github.com/ixAnkit/cryft/pkg/utils"
	"github.com/ixAnkit/cryft/pkg/ux"
	"github.com/ixAnkit/cryft/pkg/vm"
	"github.com/MetalBlockchain/metalgo/utils/logging"
	"github.com/MetalBlockchain/subnet-evm/commontype"
	"github.com/MetalBlockchain/subnet-evm/params"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/deployerallowlist"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/feemanager"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/nativeminter"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/rewardmanager"
	"github.com/MetalBlockchain/subnet-evm/precompile/contracts/txallowlist"
	subnetevmutils "github.com/MetalBlockchain/subnet-evm/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/spf13/cobra"
)

const (
	blockTimestampKey   = "blockTimestamp"
	feeConfigKey        = "initialFeeConfig"
	initialMintKey      = "initialMint"
	adminAddressesKey   = "adminAddresses"
	managerAddressesKey = "managerAddresses"
	enabledAddressesKey = "enabledAddresses"

	enabledLabel = "enabled"
	managerLabel = "manager"
	adminLabel   = "admin"
)

var subnetName string

// avalanche subnet upgrade generate
func newUpgradeGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate [subnetName]",
		Short: "Generate the configuration file to upgrade subnet nodes",
		Long: `The subnet upgrade generate command builds a new upgrade.json file to customize your Subnet. It
guides the user through the process using an interactive wizard.`,
		RunE: upgradeGenerateCmd,
		Args: cobra.ExactArgs(1),
	}
	return cmd
}

func upgradeGenerateCmd(_ *cobra.Command, args []string) error {
	subnetName = args[0]
	if !app.GenesisExists(subnetName) {
		ux.Logger.PrintToUser("The provided subnet name %q does not exist", subnetName)
		return nil
	}
	// print some warning/info message
	ux.Logger.PrintToUser(logging.Bold.Wrap(logging.Yellow.Wrap(
		"Performing a network upgrade requires coordinating the upgrade network-wide.")))
	ux.Logger.PrintToUser(logging.White.Wrap(logging.Reset.Wrap(
		"A network upgrade changes the rule set used to process and verify blocks, " +
			"such that any node that upgrades incorrectly or fails to upgrade by the time " +
			"that upgrade goes into effect may become out of sync with the rest of the network.\n")))
	ux.Logger.PrintToUser(logging.Bold.Wrap(logging.Red.Wrap(
		"Any mistakes in configuring network upgrades or coordinating them on validators " +
			"may cause the network to halt and recovering may be difficult.")))
	ux.Logger.PrintToUser(logging.Reset.Wrap(
		"Please consult " + logging.Cyan.Wrap(
			"https://docs.avax.network/subnets/customize-a-subnet#network-upgrades-enabledisable-precompiles ") +
			logging.Reset.Wrap("for more information")))

	txt := "Press [Enter] to continue, or abort by choosing 'no'"
	yes, err := app.Prompt.CaptureYesNo(txt)
	if err != nil {
		return err
	}
	if !yes {
		ux.Logger.PrintToUser("Aborted by user")
		return nil
	}

	allPreComps := []string{
		vm.ContractAllowList,
		vm.FeeManager,
		vm.NativeMint,
		vm.TxAllowList,
		vm.RewardManager,
	}

	fmt.Println()
	ux.Logger.PrintToUser(logging.Yellow.Wrap(
		"Avalanchego and this tool support configuring multiple precompiles. " +
			"However, we suggest to only configure one per upgrade."))
	fmt.Println()

	// use the correct data types from subnet-evm right away
	precompiles := params.UpgradeConfig{
		PrecompileUpgrades: make([]params.PrecompileUpgrade, 0),
	}

	for {
		precomp, err := app.Prompt.CaptureList("Select the precompile to configure", allPreComps)
		if err != nil {
			return err
		}

		ux.Logger.PrintToUser(fmt.Sprintf("Set parameters for the %q precompile", precomp))
		if err := promptParams(precomp, &precompiles.PrecompileUpgrades); err != nil {
			return err
		}

		if len(allPreComps) > 1 {
			yes, err := app.Prompt.CaptureNoYes("Should we configure another precompile?")
			if err != nil {
				return err
			}
			if !yes {
				break
			}

			for i := 0; i < len(allPreComps); i++ {
				if allPreComps[i] == precomp {
					allPreComps = append(allPreComps[:i], allPreComps[i+1:]...)
					break
				}
			}
		}
	}

	jsonBytes, err := json.Marshal(&precompiles)
	if err != nil {
		return err
	}

	return app.WriteUpgradeFile(subnetName, jsonBytes)
}

func queryActivationTimestamp() (time.Time, error) {
	const (
		in5min   = "In 5 minutes"
		in1day   = "In 1 day"
		in1week  = "In 1 week"
		in2weeks = "In 2 weeks"
		custom   = "Custom"
	)
	options := []string{in5min, in1day, in1week, in2weeks, custom}
	choice, err := app.Prompt.CaptureList("When should the precompile be activated?", options)
	if err != nil {
		return time.Time{}, err
	}

	var date time.Time
	now := time.Now()

	switch choice {
	case in5min:
		date = now.Add(5 * time.Minute)
	case in1day:
		date = now.Add(24 * time.Hour)
	case in1week:
		date = now.Add(7 * 24 * time.Hour)
	case in2weeks:
		date = now.Add(14 * 24 * time.Hour)
	case custom:
		date, err = app.Prompt.CaptureFutureDate(
			"Enter the block activation UTC datetime in 'YYYY-MM-DD HH:MM:SS' format", time.Now().Add(time.Minute).UTC())
		if err != nil {
			return time.Time{}, err
		}
	}

	ux.Logger.PrintToUser("The chosen block activation time is %s", date.Format(constants.TimeParseLayout))
	return date, nil
}

func promptParams(precomp string, precompiles *[]params.PrecompileUpgrade) error {
	date, err := queryActivationTimestamp()
	if err != nil {
		return err
	}
	switch precomp {
	case vm.ContractAllowList:
		return promptContractAllowListParams(precompiles, date)
	case vm.TxAllowList:
		return promptTxAllowListParams(precompiles, date)
	case vm.NativeMint:
		return promptNativeMintParams(precompiles, date)
	case vm.FeeManager:
		return promptFeeManagerParams(precompiles, date)
	case vm.RewardManager:
		return promptRewardManagerParams(precompiles, date)
	default:
		return fmt.Errorf("unexpected precompile identifier: %q", precomp)
	}
}

func promptNativeMintParams(precompiles *[]params.PrecompileUpgrade, date time.Time) error {
	initialMint := map[common.Address]*math.HexOrDecimal256{}

	adminAddrs, managerAddrs, enabledAddrs, err := promptAdminManagerAndEnabledAddresses()
	if err != nil {
		return err
	}

	yes, err := app.Prompt.CaptureYesNo(fmt.Sprintf("Airdrop more tokens? (`%s` section in file)", initialMintKey))
	if err != nil {
		return err
	}

	sc, err := app.LoadSidecar(subnetName)
	if err != nil {
		return err
	}

	if yes {
		_, cancel, err := prompts.CaptureListDecision(
			app.Prompt,
			"How would you like to distribute your funds",
			func(_ string) (string, error) {
				addr, err := app.Prompt.CaptureAddress("Address to airdrop to")
				if err != nil {
					return "", err
				}
				amount, err := app.Prompt.CaptureUint64(fmt.Sprintf("Amount to airdrop (in %s units)", sc.TokenSymbol))
				if err != nil {
					return "", err
				}
				initialMint[addr] = math.NewHexOrDecimal256(int64(amount))
				return fmt.Sprintf("%s-%d", addr.Hex(), amount), nil
			},
			"Add an address to amount pair",
			"Address-Amount",
			"Hex-formatted address and it's initial amount value, "+
				"for example: 0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC (address) and 1000000000000000000 (value)",
		)
		if err != nil {
			return err
		}
		if cancel {
			return errors.New("aborted by user")
		}
	}

	config := nativeminter.NewConfig(
		subnetevmutils.NewUint64(uint64(date.Unix())),
		adminAddrs,
		enabledAddrs,
		managerAddrs,
		initialMint,
	)
	upgrade := params.PrecompileUpgrade{
		Config: config,
	}
	*precompiles = append(*precompiles, upgrade)
	return nil
}

func promptRewardManagerParams(precompiles *[]params.PrecompileUpgrade, date time.Time) error {
	adminAddrs, managerAddrs, enabledAddrs, err := promptAdminManagerAndEnabledAddresses()
	if err != nil {
		return err
	}

	initialConfig, err := vm.ConfigureInitialRewardConfig(app)
	if err != nil {
		return err
	}

	config := rewardmanager.NewConfig(
		subnetevmutils.NewUint64(uint64(date.Unix())),
		adminAddrs,
		enabledAddrs,
		managerAddrs,
		initialConfig,
	)

	upgrade := params.PrecompileUpgrade{
		Config: config,
	}
	*precompiles = append(*precompiles, upgrade)
	return nil
}

func promptFeeManagerParams(precompiles *[]params.PrecompileUpgrade, date time.Time) error {
	adminAddrs, managerAddrs, enabledAddrs, err := promptAdminManagerAndEnabledAddresses()
	if err != nil {
		return err
	}

	yes, err := app.Prompt.CaptureYesNo(fmt.Sprintf(
		"Do you want to update the fee config upon precompile activation? ('%s' section in file)", feeConfigKey))
	if err != nil {
		return err
	}

	var feeConfig *commontype.FeeConfig

	if yes {
		chainConfig, _, err := vm.GetFeeConfig(params.ChainConfig{}, app, false)
		if err != nil {
			return err
		}
		feeConfig = &chainConfig.FeeConfig
	}

	config := feemanager.NewConfig(
		subnetevmutils.NewUint64(uint64(date.Unix())),
		adminAddrs,
		enabledAddrs,
		managerAddrs,
		feeConfig,
	)
	upgrade := params.PrecompileUpgrade{
		Config: config,
	}
	*precompiles = append(*precompiles, upgrade)
	return nil
}

func promptContractAllowListParams(precompiles *[]params.PrecompileUpgrade, date time.Time) error {
	adminAddrs, managerAddrs, enabledAddrs, err := promptAdminManagerAndEnabledAddresses()
	if err != nil {
		return err
	}

	config := deployerallowlist.NewConfig(
		subnetevmutils.NewUint64(uint64(date.Unix())),
		adminAddrs,
		enabledAddrs,
		managerAddrs,
	)
	upgrade := params.PrecompileUpgrade{
		Config: config,
	}
	*precompiles = append(*precompiles, upgrade)
	return nil
}

func promptTxAllowListParams(precompiles *[]params.PrecompileUpgrade, date time.Time) error {
	adminAddrs, managerAddrs, enabledAddrs, err := promptAdminManagerAndEnabledAddresses()
	if err != nil {
		return err
	}

	config := txallowlist.NewConfig(
		subnetevmutils.NewUint64(uint64(date.Unix())),
		adminAddrs,
		enabledAddrs,
		managerAddrs,
	)
	upgrade := params.PrecompileUpgrade{
		Config: config,
	}
	*precompiles = append(*precompiles, upgrade)
	return nil
}

func getCClient(apiEndpoint string, blockchainID string) (ethclient.Client, error) {
	cClient, err := ethclient.Dial(fmt.Sprintf("%s/ext/bc/%s/rpc", apiEndpoint, blockchainID))
	if err != nil {
		return nil, err
	}
	return cClient, nil
}

func ensureHaveBalanceLocalNetwork(which string, addresses []common.Address, blockchainID string) error {
	cClient, err := getCClient(constants.LocalAPIEndpoint, blockchainID)
	if err != nil {
		return err
	}

	for _, address := range addresses {
		// we can break at the first address who has a non-zero balance
		accountBalance, err := getAccountBalance(cClient, address.String())
		if err != nil {
			return err
		}
		if accountBalance > float64(0) {
			return nil
		}
	}

	return fmt.Errorf("at least one of the %s addresses requires a positive token balance", which)
}

func ensureHaveBalance(which string, addresses []common.Address, subnetName string) error {
	if len(addresses) < 1 {
		return nil
	}

	if !app.GenesisExists(subnetName) {
		ux.Logger.PrintToUser("The provided subnet name %q does not exist", subnetName)
		return nil
	}

	// read in sidecar
	sc, err := app.LoadSidecar(subnetName)
	if err != nil {
		return err
	}
	switch sc.VM {
	case models.SubnetEvm:
		// Currently only checking if admins have balance for subnets deployed in Local Network
		if networkData, ok := sc.Networks["Local Network"]; ok {
			blockchainID := networkData.BlockchainID.String()
			err = ensureHaveBalanceLocalNetwork(which, addresses, blockchainID)
			if err != nil {
				return err
			}
		}
	default:
		app.Log.Warn("Unsupported VM type", zap.Any("vm-type", sc.VM))
	}
	return nil
}

func getAccountBalance(cClient ethclient.Client, addrStr string) (float64, error) {
	addr := common.HexToAddress(addrStr)
	ctx, cancel := utils.GetAPIContext()
	balance, err := cClient.BalanceAt(ctx, addr, nil)
	defer cancel()
	if err != nil {
		return 0, err
	}
	// convert to nAvax
	balance = balance.Div(balance, big.NewInt(int64(units.Avax)))
	if balance.Cmp(big.NewInt(0)) == 0 {
		return 0, nil
	}
	return float64(balance.Uint64()) / float64(units.Avax), nil
}

func promptAdminManagerAndEnabledAddresses() ([]common.Address, []common.Address, []common.Address, error) {
	var admin, manager, enabled []common.Address

	for {
		if err := captureAddress(adminLabel, &admin); err != nil {
			return nil, nil, nil, err
		}

		if err := ensureHaveBalance(adminLabel, admin, subnetName); err != nil {
			return nil, nil, nil, err
		}

		if err := captureAddress(managerLabel, &manager); err != nil {
			return nil, nil, nil, err
		}

		if err := ensureHaveBalance(managerLabel, admin, subnetName); err != nil {
			return nil, nil, nil, err
		}

		adminsMap := make(map[string]bool)
		for _, adminsAddress := range admin {
			adminsMap[adminsAddress.String()] = true
		}
		managersMap := make(map[string]bool)
		for _, managerAddress := range manager {
			managersMap[managerAddress.String()] = true
		}

		for _, managerAddress := range manager {
			if _, ok := adminsMap[managerAddress.String()]; ok {
				return nil, nil, nil, fmt.Errorf("can't have address %s in both admin and manager addresses", managerAddress.String())
			}
		}

		if err := captureAddress(enabledLabel, &enabled); err != nil {
			return nil, nil, nil, err
		}

		for _, enabledAddress := range enabled {
			if _, ok := adminsMap[enabledAddress.String()]; ok {
				return nil, nil, nil, fmt.Errorf("can't have address %s in both admin and enabled addresses", enabledAddress.String())
			}
			if _, ok := managersMap[enabledAddress.String()]; ok {
				return nil, nil, nil, fmt.Errorf("can't have address %s in both manager and enabled addresses", enabledAddress.String())
			}
		}
		if len(enabled) == 0 && len(admin) == 0 && len(manager) == 0 {
			ux.Logger.PrintToUser(fmt.Sprintf(
				"We need at least one address for either '%s', '%s' or '%s'. Otherwise abort.", enabledAddressesKey, managerAddressesKey, adminAddressesKey))
			continue
		}
		return admin, manager, enabled, nil
	}
}

func captureAddress(which string, addrsField *[]common.Address) error {
	yes, err := app.Prompt.CaptureYesNo(fmt.Sprintf("Add '%sAddresses'?", which))
	if err != nil {
		return err
	}
	if yes {
		var (
			cancel bool
			err    error
		)
		*addrsField, cancel, err = prompts.CaptureListDecision(
			app.Prompt,
			fmt.Sprintf("Provide '%sAddresses'", which),
			app.Prompt.CaptureAddress,
			"Add an address",
			"Address",
			fmt.Sprintf("Hex-formatted %s addresses", which),
		)
		if err != nil {
			return err
		}
		if cancel {
			return errors.New("aborted by user")
		}
	}
	return nil
}
