// Copyright (C) 2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
package ssh

import (
	"bytes"
	"embed"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"text/template"
	"time"

	"github.com/ixAnkit/cryft/pkg/monitoring"
	"github.com/ixAnkit/cryft/pkg/utils"
	"github.com/ixAnkit/cryft/pkg/ux"

	"github.com/ixAnkit/cryft/pkg/constants"
	"github.com/ixAnkit/cryft/pkg/models"
)

type scriptInputs struct {
	AvalancheGoVersion      string
	CLIVersion              string
	SubnetExportFileName    string
	SubnetName              string
	ClusterName             string
	GoVersion               string
	CliBranch               string
	IsDevNet                bool
	IsE2E                   bool
	NetworkFlag             string
	SubnetEVMBinaryPath     string
	SubnetEVMReleaseURL     string
	SubnetEVMArchive        string
	MonitoringDashboardPath string
	LoadTestRepoDir         string
	LoadTestRepo            string
	LoadTestPath            string
	LoadTestCommand         string
	LoadTestBranch          string
	LoadTestGitCommit       string
	CheckoutCommit          bool
	LoadTestResultFile      string
	GrafanaPkg              string
}

//go:embed shell/*.sh
var script embed.FS

// RunOverSSH runs provided script path over ssh.
// This script can be template as it will be rendered using scriptInputs vars
func RunOverSSH(
	scriptDesc string,
	host *models.Host,
	timeout time.Duration,
	scriptPath string,
	templateVars scriptInputs,
) error {
	startTime := time.Now()
	shellScript, err := script.ReadFile(scriptPath)
	if err != nil {
		return err
	}
	var script bytes.Buffer
	t, err := template.New(scriptDesc).Parse(string(shellScript))
	if err != nil {
		return err
	}
	err = t.Execute(&script, templateVars)
	if err != nil {
		return err
	}

	if output, err := host.Command(script.String(), nil, timeout); err != nil {
		return fmt.Errorf("%w: %s", err, string(output))
	}
	executionTime := time.Since(startTime)
	ux.Logger.Info("RunOverSSH[%s]%s took %s with err: %v", host.NodeID, scriptDesc, executionTime, err)
	return nil
}

func PostOverSSH(host *models.Host, path string, requestBody string) ([]byte, error) {
	if path == "" {
		path = "/ext/info"
	}
	localhost, err := url.Parse(constants.LocalAPIEndpoint)
	if err != nil {
		return nil, err
	}
	requestHeaders := fmt.Sprintf("POST %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Content-Length: %d\r\n"+
		"Content-Type: application/json\r\n\r\n", path, localhost.Host, len(requestBody))
	httpRequest := requestHeaders + requestBody
	return host.Forward(httpRequest, constants.SSHPOSTTimeout)
}

// RunSSHSetupNode runs script to setup node
func RunSSHSetupNode(host *models.Host, configPath, avalancheGoVersion string, cliVersion string, isDevNet bool) error {
	if err := RunOverSSH(
		"Setup Node",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupNode.sh",
		scriptInputs{AvalancheGoVersion: avalancheGoVersion, CLIVersion: cliVersion, IsDevNet: isDevNet, IsE2E: utils.IsE2E()},
	); err != nil {
		return err
	}
	if utils.IsE2E() && utils.E2EDocker() {
		if err := RunOverSSH(
			"E2E Start Avalanchego",
			host,
			constants.SSHScriptTimeout,
			"shell/e2e_startNode.sh",
			scriptInputs{},
		); err != nil {
			return err
		}
	}
	// name: copy metrics config to cloud server
	return host.Upload(
		configPath,
		filepath.Join(constants.CloudNodeCLIConfigBasePath, filepath.Base(configPath)),
		constants.SSHFileOpsTimeout,
	)
}

// RunSSHRestartNode runs script to restart avalanchego
func RunSSHRestartNode(host *models.Host) error {
	return RunOverSSH(
		"Restart Avalanchego",
		host,
		constants.SSHScriptTimeout,
		"shell/restartNode.sh",
		scriptInputs{},
	)
}

// RunSSHSetupAWMRelayerService runs script to set up an AWM Relayer Service
func RunSSHSetupAWMRelayerService(host *models.Host) error {
	return RunOverSSH(
		"Setup AWM Relayer Service",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupRelayerService.sh",
		scriptInputs{},
	)
}

// RunSSHStartAWMRelayerService runs script to start an AWM Relayer Service
func RunSSHStartAWMRelayerService(host *models.Host) error {
	return RunOverSSH(
		"Starts AWM Relayer Service",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/startRelayerService.sh",
		scriptInputs{},
	)
}

// RunSSHStopAWMRelayerService runs script to start an AWM Relayer Service
func RunSSHStopAWMRelayerService(host *models.Host) error {
	return RunOverSSH(
		"Stops AWM Relayer Service",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/stopRelayerService.sh",
		scriptInputs{},
	)
}

// RunSSHUpgradeAvalanchego runs script to upgrade avalanchego
func RunSSHUpgradeAvalanchego(host *models.Host, avalancheGoVersion string) error {
	if utils.IsE2E() && utils.E2EDocker() {
		return RunOverSSH(
			"E2E Upgrade Avalanchego",
			host,
			constants.SSHScriptTimeout,
			"shell/e2e_upgradeAvalancheGo.sh",
			scriptInputs{AvalancheGoVersion: avalancheGoVersion},
		)
	}
	return RunOverSSH(
		"Upgrade Avalanchego",
		host,
		constants.SSHScriptTimeout,
		"shell/upgradeAvalancheGo.sh",
		scriptInputs{AvalancheGoVersion: avalancheGoVersion},
	)
}

// RunSSHStartNode runs script to start avalanchego
func RunSSHStartNode(host *models.Host) error {
	if utils.IsE2E() && utils.E2EDocker() {
		return RunOverSSH(
			"E2E Start Avalanchego",
			host,
			constants.SSHScriptTimeout,
			"shell/e2e_startNode.sh",
			scriptInputs{},
		)
	}
	return RunOverSSH(
		"Start Avalanchego",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/startNode.sh",
		scriptInputs{},
	)
}

// RunSSHStopNode runs script to stop avalanchego
func RunSSHStopNode(host *models.Host) error {
	if utils.IsE2E() && utils.E2EDocker() {
		return RunOverSSH(
			"E2E Stop Avalanchego",
			host,
			constants.SSHScriptTimeout,
			"shell/e2e_stopNode.sh",
			scriptInputs{},
		)
	}
	return RunOverSSH(
		"Stop Avalanchego",
		host,
		constants.SSHScriptTimeout,
		"shell/stopNode.sh",
		scriptInputs{},
	)
}

// RunSSHUpgradeSubnetEVM runs script to upgrade subnet evm
func RunSSHUpgradeSubnetEVM(host *models.Host, subnetEVMBinaryPath string) error {
	return RunOverSSH(
		"Upgrade Subnet EVM",
		host,
		constants.SSHScriptTimeout,
		"shell/upgradeSubnetEVM.sh",
		scriptInputs{SubnetEVMBinaryPath: subnetEVMBinaryPath},
	)
}

func replaceCustomVarDashboardValues(monitoringDashboardPath, customGrafanaDashboardFileName, chainID string) error {
	sedScript := fmt.Sprintf("s/\"text\": \"CHAIN_ID_VAL\"/\"text\": \"%[1]v\"/g; s/\"value\": \"CHAIN_ID_VAL\"/\"value\": \"%[1]v\"/g; s/\"query\": \"CHAIN_ID_VAL\"/\"query\": \"%[1]v\"/g", chainID)
	cmd := exec.Command("sed", "-i", "-e", sedScript, customGrafanaDashboardFileName)
	cmd.Dir = monitoringDashboardPath
	_, err := cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

func RunSSHUpdateMonitoringDashboards(host *models.Host, monitoringDashboardPath, customGrafanaDashboardPath, chainID string) error {
	remoteDashboardsPath := "/home/ubuntu/dashboards"
	if !utils.DirectoryExists(monitoringDashboardPath) {
		return fmt.Errorf("%s does not exist", monitoringDashboardPath)
	}
	if customGrafanaDashboardPath != "" && utils.FileExists(utils.ExpandHome(customGrafanaDashboardPath)) {
		if err := utils.FileCopy(utils.ExpandHome(customGrafanaDashboardPath), filepath.Join(monitoringDashboardPath, constants.CustomGrafanaDashboardJSON)); err != nil {
			return err
		}
		if err := replaceCustomVarDashboardValues(monitoringDashboardPath, constants.CustomGrafanaDashboardJSON, chainID); err != nil {
			return err
		}
	}
	if err := host.MkdirAll(remoteDashboardsPath, constants.SSHFileOpsTimeout); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(monitoringDashboardPath, constants.CustomGrafanaDashboardJSON),
		filepath.Join(remoteDashboardsPath, constants.CustomGrafanaDashboardJSON),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return RunOverSSH(
		"Sync Grafana Dashboards",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/updateGrafanaDashboards.sh",
		scriptInputs{},
	)
}

func RunSSHCopyMonitoringDashboards(host *models.Host, monitoringDashboardPath string) error {
	// TODO: download dashboards from github instead
	remoteDashboardsPath := "/home/ubuntu/dashboards"
	if !utils.DirectoryExists(monitoringDashboardPath) {
		return fmt.Errorf("%s does not exist", monitoringDashboardPath)
	}
	if err := host.MkdirAll(remoteDashboardsPath, constants.SSHFileOpsTimeout); err != nil {
		return err
	}
	dashboards, err := os.ReadDir(monitoringDashboardPath)
	if err != nil {
		return err
	}
	for _, dashboard := range dashboards {
		if err := host.Upload(
			filepath.Join(monitoringDashboardPath, dashboard.Name()),
			filepath.Join(remoteDashboardsPath, dashboard.Name()),
			constants.SSHFileOpsTimeout,
		); err != nil {
			return err
		}
	}
	return RunOverSSH(
		"Sync Grafana Dashboards",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/updateGrafanaDashboards.sh",
		scriptInputs{},
	)
}

func RunSSHCopyYAMLFile(host *models.Host, yamlFilePath string) error {
	if err := host.Upload(
		yamlFilePath,
		fmt.Sprintf("/home/ubuntu/%s", filepath.Base(yamlFilePath)),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return nil
}

func RunSSHSetupMachineMetrics(host *models.Host) error {
	return RunOverSSH(
		"Setup Machine Metrics",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupMachineMetrics.sh",
		scriptInputs{},
	)
}

func RunSSHSetupSeparateMonitoring(host *models.Host, grafanaPkg string) error {
	return RunOverSSH(
		"Setup Prometheus and Grafana",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupMonitoring.sh",
		scriptInputs{
			IsE2E:      utils.IsE2E(),
			GrafanaPkg: grafanaPkg,
		},
	)
}

func RunSSHUpdatePrometheusConfig(host *models.Host, avalancheGoPorts, machinePorts, loadTestPorts []string) error {
	const cloudNodePrometheusConfigTemp = "/tmp/prometheus.yml"
	promConfig, err := os.CreateTemp("", "prometheus")
	if err != nil {
		return err
	}
	defer os.Remove(promConfig.Name())
	if err := monitoring.WritePrometheusConfig(promConfig.Name(), avalancheGoPorts, machinePorts, loadTestPorts); err != nil {
		return err
	}
	if err := host.Upload(
		promConfig.Name(),
		cloudNodePrometheusConfigTemp,
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return RunOverSSH(
		"Update Prometheus Config",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/updatePrometheusConfig.sh",
		scriptInputs{},
	)
}

func RunSSHUpdateLokiConfig(host *models.Host, port int) error {
	const cloudNodeLokiConfigTemp = "/tmp/loki.yml"
	lokiConfig, err := os.CreateTemp("", "loki")
	if err != nil {
		return err
	}
	defer os.Remove(lokiConfig.Name())
	if err := monitoring.WriteLokiConfig(lokiConfig.Name(), strconv.Itoa(port)); err != nil {
		return err
	}
	if err := host.Upload(
		lokiConfig.Name(),
		cloudNodeLokiConfigTemp,
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return RunOverSSH(
		"Update Loki Config",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/updateLokiConfig.sh",
		scriptInputs{},
	)
}

func RunSSHSetupPromtail(host *models.Host) error {
	return RunOverSSH(
		"Setup Promtail",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupPromtail.sh",
		scriptInputs{},
	)
}

func RunSSHSetupLoki(host *models.Host, grafanaPkg string) error {
	return RunOverSSH(
		"Setup Loki",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupLoki.sh",
		scriptInputs{
			GrafanaPkg: grafanaPkg,
		},
	)
}

func RunSSHUpdatePromtailConfig(host *models.Host, ip string, port int, cloudID string, nodeID string) error {
	const cloudNodePromtailConfigTemp = "/tmp/promtail.yml"
	promtailConfig, err := os.CreateTemp("", "promtail")
	if err != nil {
		return err
	}
	defer os.Remove(promtailConfig.Name())
	// get NodeID
	if err := monitoring.WritePromtailConfig(promtailConfig.Name(), ip, strconv.Itoa(port), cloudID, nodeID); err != nil {
		return err
	}
	if err := host.Upload(
		promtailConfig.Name(),
		cloudNodePromtailConfigTemp,
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return RunOverSSH(
		"Update Promtail Config",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/updatePromtailConfig.sh",
		scriptInputs{},
	)
}

func RunSSHDownloadNodePrometheusConfig(host *models.Host, nodeInstanceDirPath string) error {
	return host.Download(
		constants.CloudNodePrometheusConfigPath,
		filepath.Join(nodeInstanceDirPath, constants.NodePrometheusConfigFileName),
		constants.SSHFileOpsTimeout,
	)
}

func RunSSHDownloadNodeMonitoringConfig(host *models.Host, nodeInstanceDirPath string) error {
	return host.Download(
		filepath.Join(constants.CloudNodeConfigPath, constants.NodeFileName),
		filepath.Join(nodeInstanceDirPath, constants.NodeFileName),
		constants.SSHFileOpsTimeout,
	)
}

func RunSSHUploadNodeAWMRelayerConfig(host *models.Host, nodeInstanceDirPath string) error {
	cloudAWMRelayerConfigDir := filepath.Join(constants.CloudNodeCLIConfigBasePath, constants.ServicesDir, constants.AWMRelayerInstallDir)
	if err := host.MkdirAll(cloudAWMRelayerConfigDir, constants.SSHDirOpsTimeout); err != nil {
		return err
	}
	return host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.ServicesDir, constants.AWMRelayerInstallDir, constants.AWMRelayerConfigFilename),
		filepath.Join(cloudAWMRelayerConfigDir, constants.AWMRelayerConfigFilename),
		constants.SSHFileOpsTimeout,
	)
}

func RunSSHUploadNodeMonitoringConfig(host *models.Host, nodeInstanceDirPath string) error {
	if err := host.MkdirAll(
		constants.CloudNodeConfigPath,
		constants.SSHDirOpsTimeout,
	); err != nil {
		return err
	}
	return host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.NodeFileName),
		filepath.Join(constants.CloudNodeConfigPath, constants.NodeFileName),
		constants.SSHFileOpsTimeout,
	)
}

// RunSSHGetNewSubnetEVMRelease runs script to download new subnet evm
func RunSSHGetNewSubnetEVMRelease(host *models.Host, subnetEVMReleaseURL, subnetEVMArchive string) error {
	return RunOverSSH(
		"Get Subnet EVM Release",
		host,
		constants.SSHScriptTimeout,
		"shell/getNewSubnetEVMRelease.sh",
		scriptInputs{SubnetEVMReleaseURL: subnetEVMReleaseURL, SubnetEVMArchive: subnetEVMArchive},
	)
}

// RunSSHSetupDevNet runs script to setup devnet
func RunSSHSetupDevNet(host *models.Host, nodeInstanceDirPath string) error {
	if err := host.MkdirAll(
		constants.CloudNodeConfigPath,
		constants.SSHDirOpsTimeout,
	); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.GenesisFileName),
		filepath.Join(constants.CloudNodeConfigPath, constants.GenesisFileName),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.NodeFileName),
		filepath.Join(constants.CloudNodeConfigPath, constants.NodeFileName),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	// name: setup devnet
	return RunOverSSH(
		"Setup DevNet",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupDevnet.sh",
		scriptInputs{IsE2E: utils.IsE2E()},
	)
}

func RunSSHUploadClustersConfig(host *models.Host, localClustersConfigPath string) error {
	remoteNodesDir := filepath.Join(constants.CloudNodeCLIConfigBasePath, constants.NodesDir)
	if err := host.MkdirAll(
		remoteNodesDir,
		constants.SSHDirOpsTimeout,
	); err != nil {
		return err
	}
	remoteClustersConfigPath := filepath.Join(remoteNodesDir, constants.ClustersConfigFileName)
	return host.Upload(
		localClustersConfigPath,
		remoteClustersConfigPath,
		constants.SSHFileOpsTimeout,
	)
}

// RunSSHUploadStakingFiles uploads staking files to a remote host via SSH.
func RunSSHUploadStakingFiles(host *models.Host, nodeInstanceDirPath string) error {
	if err := host.MkdirAll(
		constants.CloudNodeStakingPath,
		constants.SSHDirOpsTimeout,
	); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.StakerCertFileName),
		filepath.Join(constants.CloudNodeStakingPath, constants.StakerCertFileName),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	if err := host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.StakerKeyFileName),
		filepath.Join(constants.CloudNodeStakingPath, constants.StakerKeyFileName),
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return host.Upload(
		filepath.Join(nodeInstanceDirPath, constants.BLSKeyFileName),
		filepath.Join(constants.CloudNodeStakingPath, constants.BLSKeyFileName),
		constants.SSHFileOpsTimeout,
	)
}

// RunSSHExportSubnet exports deployed Subnet from local machine to cloud server
func RunSSHExportSubnet(host *models.Host, exportPath, cloudServerSubnetPath string) error {
	// name: copy exported subnet VM spec to cloud server
	return host.Upload(
		exportPath,
		cloudServerSubnetPath,
		constants.SSHFileOpsTimeout,
	)
}

// RunSSHTrackSubnet enables tracking of specified subnet
func RunSSHTrackSubnet(host *models.Host, subnetName, importPath, networkFlag string) error {
	return RunOverSSH(
		"Track Subnet",
		host,
		constants.SSHScriptTimeout,
		"shell/trackSubnet.sh",
		scriptInputs{SubnetName: subnetName, SubnetExportFileName: importPath, NetworkFlag: networkFlag},
	)
}

// RunSSHUpdateSubnet runs avalanche subnet join <subnetName> in cloud server using update subnet info
func RunSSHUpdateSubnet(host *models.Host, subnetName, importPath string) error {
	return RunOverSSH(
		"Update Subnet",
		host,
		constants.SSHScriptTimeout,
		"shell/updateSubnet.sh",
		scriptInputs{SubnetName: subnetName, SubnetExportFileName: importPath},
	)
}

// RunSSHSetupBuildEnv installs gcc, golang, rust and etc
func RunSSHSetupBuildEnv(host *models.Host) error {
	return RunOverSSH(
		"Setup Build Env",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/setupBuildEnv.sh",
		scriptInputs{GoVersion: constants.BuildEnvGolangVersion},
	)
}

func RunSSHBuildLoadTestCode(host *models.Host, loadTestRepo, loadTestPath, loadTestGitCommit, repoDirName, loadTestBranch string, checkoutCommit bool) error {
	return StreamOverSSH(
		"Build Load Test",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/buildLoadTest.sh",
		scriptInputs{
			LoadTestRepoDir: repoDirName,
			LoadTestRepo:    loadTestRepo, LoadTestPath: loadTestPath, LoadTestGitCommit: loadTestGitCommit,
			CheckoutCommit: checkoutCommit, LoadTestBranch: loadTestBranch,
		},
	)
}

func RunSSHBuildLoadTestDependencies(host *models.Host) error {
	return RunOverSSH(
		"Build Load Test",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/buildLoadTestDeps.sh",
		scriptInputs{GoVersion: constants.BuildEnvGolangVersion},
	)
}

func RunSSHRunLoadTest(host *models.Host, loadTestCommand, loadTestName string) error {
	return RunOverSSH(
		"Run Load Test",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/runLoadTest.sh",
		scriptInputs{GoVersion: constants.BuildEnvGolangVersion, LoadTestCommand: loadTestCommand, LoadTestResultFile: fmt.Sprintf("/home/ubuntu/loadtest_%s.txt", loadTestName)},
	)
}

// RunSSHSetupCLIFromSource installs any CLI branch from source
func RunSSHSetupCLIFromSource(host *models.Host, cliBranch string) error {
	if !constants.EnableSetupCLIFromSource {
		return nil
	}
	timeout := constants.SSHLongRunningScriptTimeout
	if utils.IsE2E() && utils.E2EDocker() {
		timeout = 10 * time.Minute
	}
	return RunOverSSH(
		"Setup CLI From Source",
		host,
		timeout,
		"shell/setupCLIFromSource.sh",
		scriptInputs{CliBranch: cliBranch},
	)
}

// RunSSHCheckAvalancheGoVersion checks node avalanchego version
func RunSSHCheckAvalancheGoVersion(host *models.Host) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := "{\"jsonrpc\":\"2.0\", \"id\":1,\"method\" :\"info.getNodeVersion\"}"
	return PostOverSSH(host, "", requestBody)
}

// RunSSHCheckBootstrapped checks if node is bootstrapped to primary network
func RunSSHCheckBootstrapped(host *models.Host) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := "{\"jsonrpc\":\"2.0\", \"id\":1,\"method\" :\"info.isBootstrapped\", \"params\": {\"chain\":\"X\"}}"
	return PostOverSSH(host, "", requestBody)
}

// RunSSHCheckHealthy checks if node is healthy
func RunSSHCheckHealthy(host *models.Host) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := "{\"jsonrpc\":\"2.0\", \"id\":1,\"method\":\"health.health\",\"params\": {\"tags\": [\"P\"]}}"
	return PostOverSSH(host, "/ext/health", requestBody)
}

// RunSSHGetNodeID reads nodeID from avalanchego
func RunSSHGetNodeID(host *models.Host) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := "{\"jsonrpc\":\"2.0\", \"id\":1,\"method\" :\"info.getNodeID\"}"
	return PostOverSSH(host, "", requestBody)
}

// SubnetSyncStatus checks if node is synced to subnet
func RunSSHSubnetSyncStatus(host *models.Host, blockchainID string) ([]byte, error) {
	// Craft and send the HTTP POST request
	requestBody := fmt.Sprintf("{\"jsonrpc\":\"2.0\", \"id\":1,\"method\" :\"platform.getBlockchainStatus\", \"params\": {\"blockchainID\":\"%s\"}}", blockchainID)
	return PostOverSSH(host, "/ext/bc/P", requestBody)
}

// StreamOverSSH runs provided script path over ssh.
// This script can be template as it will be rendered using scriptInputs vars
func StreamOverSSH(
	scriptDesc string,
	host *models.Host,
	timeout time.Duration,
	scriptPath string,
	templateVars scriptInputs,
) error {
	shellScript, err := script.ReadFile(scriptPath)
	if err != nil {
		return err
	}
	var script bytes.Buffer
	t, err := template.New(scriptDesc).Parse(string(shellScript))
	if err != nil {
		return err
	}
	err = t.Execute(&script, templateVars)
	if err != nil {
		return err
	}

	if err := host.StreamSSHCommand(script.String(), nil, timeout); err != nil {
		return err
	}
	return nil
}

// RunSSHWhitelistPubKey downloads the authorized_keys file from the specified host, appends the provided sshPubKey to it, and uploads the file back to the host.
func RunSSHWhitelistPubKey(host *models.Host, sshPubKey string) error {
	const sshAuthFile = "/home/ubuntu/.ssh/authorized_keys"
	tmpName := filepath.Join(os.TempDir(), utils.RandomString(10))
	defer os.Remove(tmpName)
	if err := host.Download(sshAuthFile, tmpName, constants.SSHFileOpsTimeout); err != nil {
		return err
	}
	// write ssh public key
	tmpFile, err := os.OpenFile(tmpName, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := tmpFile.WriteString(sshPubKey + "\n"); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return host.Upload(tmpFile.Name(), sshAuthFile, constants.SSHFileOpsTimeout)
}

// RunSSHDownloadFile downloads specified file from the specified host
func RunSSHDownloadFile(host *models.Host, filePath string, localFilePath string) error {
	return host.Download(filePath, localFilePath, constants.SSHFileOpsTimeout)
}

func RunSSHUpdatePromtailConfigSubnet(host *models.Host, ip string, port int, cloudID string, nodeID string, chainID string) error {
	const cloudNodePromtailConfigTemp = "/tmp/promtail.yml"
	promtailConfig, err := os.CreateTemp("", "promtailSubnet")
	if err != nil {
		return err
	}
	defer os.Remove(promtailConfig.Name())
	// get NodeID
	if err := monitoring.WritePromtailConfigSubnet(promtailConfig.Name(), ip, strconv.Itoa(port), cloudID, nodeID, fmt.Sprintf("/home/ubuntu/.avalanchego/logs/%s.log", chainID)); err != nil {
		return err
	}
	if err := host.Upload(
		promtailConfig.Name(),
		cloudNodePromtailConfigTemp,
		constants.SSHFileOpsTimeout,
	); err != nil {
		return err
	}
	return RunOverSSH(
		"Update Promtail Config",
		host,
		constants.SSHLongRunningScriptTimeout,
		"shell/updatePromtailConfig.sh",
		scriptInputs{},
	)
}

func RunSSHUpsizeRootDisk(host *models.Host) error {
	return RunOverSSH(
		"Upsize Disk",
		host,
		constants.SSHScriptTimeout,
		"shell/upsizeRootDisk.sh",
		scriptInputs{},
	)
}
