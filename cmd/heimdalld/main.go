package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/store"

	bridgeCmd "github.com/maticnetwork/heimdall/bridge/cmd"
	restServer "github.com/maticnetwork/heimdall/server"
	tserver "github.com/tendermint/tendermint/abci/server"

	sdk "github.com/cosmos/cosmos-sdk/types"
	ethCommon "github.com/maticnetwork/bor/common"
	hmbridge "github.com/maticnetwork/heimdall/bridge/cmd"
	"github.com/maticnetwork/heimdall/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	abci "github.com/tendermint/tendermint/abci/types"
	tcmd "github.com/tendermint/tendermint/cmd/tendermint/commands"

	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/secp256k1"
	"github.com/tendermint/tendermint/libs/cli"
	"github.com/tendermint/tendermint/libs/common"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	pvm "github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"
	tmTypes "github.com/tendermint/tendermint/types"
	tmtime "github.com/tendermint/tendermint/types/time"
	dbm "github.com/tendermint/tm-db"

	"github.com/maticnetwork/heimdall/app"
	authTypes "github.com/maticnetwork/heimdall/auth/types"
	"github.com/maticnetwork/heimdall/helper"
	hmserver "github.com/maticnetwork/heimdall/server"
	hmTypes "github.com/maticnetwork/heimdall/types"
	hmModule "github.com/maticnetwork/heimdall/types/module"
)

var logger = helper.Logger.With("module", "cmd/heimdalld")

var (
	flagNodeDirPrefix    = "node-dir-prefix"
	flagNumValidators    = "v"
	flagNumNonValidators = "n"
	flagOutputDir        = "output-dir"
	flagNodeDaemonHome   = "node-daemon-home"
	flagNodeCliHome      = "node-cli-home"
	flagNodeHostPrefix   = "node-host-prefix"
)

// Tendermint full-node start flags
const (
	flagWithTendermint = "with-tendermint"
	flagAddress        = "address"
	flagTraceStore     = "trace-store"
	flagPruning        = "pruning"
	flagCPUProfile     = "cpu-profile"
	FlagMinGasPrices   = "minimum-gas-prices"
	FlagHaltHeight     = "halt-height"
	FlagHaltTime       = "halt-time"
)
const (
	nodeDirPerm = 0755
)

var ZeroIntString = big.NewInt(0).String()

// ValidatorAccountFormatter helps to print local validator account information
type ValidatorAccountFormatter struct {
	Address string `json:"address,omitempty" yaml:"address"`
	PrivKey string `json:"priv_key,omitempty" yaml:"priv_key"`
	PubKey  string `json:"pub_key,omitempty" yaml:"pub_key"`
}

// GetSignerInfo returns signer information
func GetSignerInfo(pub crypto.PubKey, priv []byte, cdc *codec.Codec) ValidatorAccountFormatter {
	var privObject secp256k1.PrivKeySecp256k1
	cdc.MustUnmarshalBinaryBare(priv, &privObject)
	return ValidatorAccountFormatter{
		Address: ethCommon.BytesToAddress(pub.Address().Bytes()).String(),
		PubKey:  CryptoKeyToPubkey(pub).String(),
		PrivKey: "0x" + hex.EncodeToString(privObject[:]),
	}
}

func main() {
	cdc := app.MakeCodec()
	ctx := server.NewDefaultContext()

	rootCmd := &cobra.Command{
		Use:               "heimdalld",
		Short:             "Heimdall Daemon (server)",
		PersistentPreRunE: server.PersistentPreRunEFn(ctx),
	}

	tendermintCmd := &cobra.Command{
		Use:   "tendermint",
		Short: "Tendermint subcommands",
	}

	// add new persistent flag for heimdall-config
	rootCmd.PersistentFlags().String(
		helper.WithHeimdallConfigFlag,
		"",
		"Heimdall config file path (default <home>/config/heimdall-config.json)",
	)

	rootCmd.PersistentFlags().String(
		helper.ChainFlag,
		"",
		fmt.Sprintf("Set one of the chains: [%s]", strings.Join(helper.GetValidChains(), ",")),
	)

	// bind with-heimdall-config config and chain flag with root cmd
	if err := viper.BindPFlag(helper.WithHeimdallConfigFlag, rootCmd.PersistentFlags().Lookup(helper.WithHeimdallConfigFlag)); err != nil {
		logger.Error("main | BindPFlag | helper.WithHeimdallConfigFlag", "Error", err)
	}
	if err := viper.BindPFlag(helper.ChainFlag, rootCmd.PersistentFlags().Lookup(helper.ChainFlag)); err != nil {
		logger.Error("main | BindPFlag | helper.ChainFlag", "Error", err)
	}

	rootCmd.AddCommand(heimdallStart(ctx, newApp, cdc)) // New Heimdall start command

	tendermintCmd.AddCommand(
		server.ShowNodeIDCmd(ctx),
		server.ShowValidatorCmd(ctx),
		server.ShowAddressCmd(ctx),
		server.VersionCmd(ctx),
	)

	rootCmd.AddCommand(server.UnsafeResetAllCmd(ctx))
	rootCmd.AddCommand(flags.LineBreak)
	rootCmd.AddCommand(tendermintCmd)
	rootCmd.AddCommand(server.ExportCmd(ctx, cdc, exportAppStateAndTMValidators))
	rootCmd.AddCommand(flags.LineBreak)
	rootCmd.AddCommand(version.Cmd) // Using heimdall version, not Cosmos SDK version
	// End of block

	rootCmd.AddCommand(showAccountCmd())
	rootCmd.AddCommand(showPrivateKeyCmd())
	rootCmd.AddCommand(hmserver.ServeCommands(cdc, hmserver.RegisterRoutes))
	rootCmd.AddCommand(hmbridge.BridgeCommands())
	rootCmd.AddCommand(VerifyGenesis(ctx, cdc))
	rootCmd.AddCommand(initCmd(ctx, cdc))
	rootCmd.AddCommand(testnetCmd(ctx, cdc))

	// prepare and add flags
	executor := cli.PrepareBaseCmd(rootCmd, "HD", os.ExpandEnv("$HOME/.heimdalld"))
	err := executor.Execute()
	if err != nil {
		// Note: Handle with #870
		panic(err)
	}
}

func newApp(logger log.Logger, db dbm.DB, storeTracer io.Writer) abci.Application {
	// init heimdall config
	helper.InitHeimdallConfig("")
	// create new heimdall app
	return app.NewHeimdallApp(logger, db, baseapp.SetPruning(store.NewPruningOptionsFromString(viper.GetString("pruning"))))
}

func exportAppStateAndTMValidators(logger log.Logger, db dbm.DB, storeTracer io.Writer, height int64, forZeroHeight bool, jailWhiteList []string) (json.RawMessage, []tmTypes.GenesisValidator, error) {
	bapp := app.NewHeimdallApp(logger, db)
	return bapp.ExportAppStateAndValidators()
}

func heimdallStart(ctx *server.Context, appCreator server.AppCreator, cdc *codec.Codec) *cobra.Command { // cmd *cobra.Command
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run the full node",
		Long: `Run the full node application with Tendermint in or out of process. By
default, the application will run with Tendermint in process.
Starting rest server is provided with the flag --rest-server and starting bridge with 
the flag --bridge when starting Tendermint in process.
Pruning options can be provided via the '--pruning' flag. The options are as follows:

syncable: only those states not needed for state syncing will be deleted (keeps last 100 + every 10000th)
nothing: all historic states will be saved, nothing will be deleted (i.e. archiving node)
everything: all saved states will be deleted, storing only the current state

Node halting configurations exist in the form of two flags: '--halt-height' and '--halt-time'. During
the ABCI Commit phase, the node will check if the current block height is greater than or equal to
the halt-height or if the current block time is greater than or equal to the halt-time. If so, the
node will attempt to gracefully shutdown and the block will not be committed. In addition, the node
will not be able to commit subsequent blocks.

For profiling and benchmarking purposes, CPU profiling can be enabled via the '--cpu-profile' flag
which accepts a path for the resulting pprof file.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !viper.GetBool(flagWithTendermint) {
				ctx.Logger.Info("starting ABCI without Tendermint")
				return startStandAlone(ctx, appCreator)
			}

			ctx.Logger.Info("starting ABCI with Tendermint")

			startRestServer, _ := cmd.Flags().GetBool(helper.RestServerFlag)
			startBridge, _ := cmd.Flags().GetBool(helper.BridgeFlag)

			_, err := startInProcess(ctx, appCreator, cdc, startRestServer, startBridge)
			return err
		},
		PreRun: func(cmd *cobra.Command, args []string) {
			// bridge binding
			if err := viper.BindPFlag("all", cmd.Flags().Lookup("all")); err != nil {
				logger.Error("GetStartCmd | BindPFlag | all", "Error", err)
			}

			if err := viper.BindPFlag("only", cmd.Flags().Lookup("only")); err != nil {
				logger.Error("GetStartCmd | BindPFlag | only", "Error", err)
			}
		},
	}

	cmd.Flags().Bool(
		helper.RestServerFlag,
		false,
		"Start rest server",
	)

	cmd.Flags().Bool(
		helper.BridgeFlag,
		false,
		"Start bridge service",
	)

	cmd.PersistentFlags().String(helper.LogLevel, ctx.Config.LogLevel, "Log level")
	if err := viper.BindPFlag(helper.LogLevel, cmd.PersistentFlags().Lookup(helper.LogLevel)); err != nil {
		logger.Error("main | BindPFlag | helper.LogLevel", "Error", err)
	}
	// bridge flags
	cmd.Flags().Bool("all", false, "start all bridge services")
	cmd.Flags().StringSlice("only", []string{}, "comma separated bridge services to start")

	// rest server flags
	cmd.Flags().String(client.FlagListenAddr, "tcp://0.0.0.0:1317", "The address for the server to listen on")
	cmd.Flags().Bool(client.FlagTrustNode, true, "Trust connected full node (don't verify proofs for responses)")
	cmd.Flags().String(client.FlagChainID, "", "The chain ID to connect to")
	cmd.Flags().String(client.FlagNode, helper.DefaultTendermintNode, "Address of the node to connect to")
	cmd.Flags().Int(client.FlagMaxOpenConnections, 1000, "The number of maximum open connections")

	// core flags for the ABCI application
	cmd.Flags().Bool(flagWithTendermint, true, "Run abci app embedded in-process with tendermint")
	cmd.Flags().String(flagAddress, "tcp://0.0.0.0:26658", "Listen address")
	cmd.Flags().String(flagTraceStore, "", "Enable KVStore tracing to an output file")
	cmd.Flags().String(flagPruning, "syncable", "Pruning strategy: syncable, nothing, everything")
	cmd.Flags().String(
		FlagMinGasPrices, "",
		"Minimum gas prices to accept for transactions; Any fee in a tx must meet this minimum (e.g. 0.01photino;0.0001stake)",
	)
	cmd.Flags().Uint64(FlagHaltHeight, 0, "Height at which to gracefully halt the chain and shutdown the node")
	cmd.Flags().Uint64(FlagHaltTime, 0, "Minimum block time (in Unix seconds) at which to gracefully halt the chain and shutdown the node")
	cmd.Flags().String(flagCPUProfile, "", "Enable CPU profiling and write to the provided file")

	// add support for all Tendermint-specific command line options
	tcmd.AddNodeFlags(cmd)
	return cmd
}

func startStandAlone(ctx *server.Context, appCreator server.AppCreator) error {
	addr := viper.GetString(flagAddress)
	home := viper.GetString("home")
	traceWriterFile := viper.GetString(flagTraceStore)

	db, err := openDB(home)
	if err != nil {
		return err
	}
	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		return err
	}

	app := appCreator(ctx.Logger, db, traceWriter)

	svr, err := tserver.NewServer(addr, "socket", app)
	if err != nil {
		return fmt.Errorf("error creating listener: %v", err)
	}

	svr.SetLogger(ctx.Logger.With("module", "abci-server"))

	err = svr.Start()
	if err != nil {
		common.Exit(err.Error())
	}

	common.TrapSignal(ctx.Logger, func() {
		// cleanup
		err = svr.Stop()
		if err != nil {
			common.Exit(err.Error())
		}
	})

	// run forever (the node will not be returned)
	select {}
}

func startInProcess(ctx *server.Context, appCreator server.AppCreator, cdc *codec.Codec, startRestServer bool, startBridge bool) (*node.Node, error) {
	cfg := ctx.Config
	home := cfg.RootDir
	traceWriterFile := viper.GetString(flagTraceStore)

	db, err := openDB(home)
	if err != nil {
		return nil, err
	}
	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		return nil, err
	}

	app := appCreator(ctx.Logger, db, traceWriter)

	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		return nil, err
	}

	server.UpgradeOldPrivValFile(cfg)

	// create & start tendermint node
	tmNode, err := node.NewNode(
		cfg,
		pvm.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile()),
		nodeKey,
		proxy.NewLocalClientCreator(app),
		node.DefaultGenesisDocProviderFunc(cfg),
		node.DefaultDBProvider,
		node.DefaultMetricsProvider(cfg.Instrumentation),
		ctx.Logger.With("module", "node"),
	)
	if err != nil {
		return nil, err
	}

	if err := tmNode.Start(); err != nil {
		return nil, err
	}

	var cpuProfileCleanup func()

	if cpuProfile := viper.GetString(flagCPUProfile); cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			return nil, err
		}

		ctx.Logger.Info("starting CPU profiler", "profile", cpuProfile)
		if err := pprof.StartCPUProfile(f); err != nil {
			return nil, err
		}

		cpuProfileCleanup = func() {
			ctx.Logger.Info("stopping CPU profiler", "profile", cpuProfile)
			pprof.StopCPUProfile()
			f.Close()
		}
	}

	// start rest
	if startRestServer {
		restCh := make(chan struct{})
		go func() {
			_ = restServer.StartRestServer(cdc, hmserver.RegisterRoutes, restCh)
		}()
		<-restCh
	}

	// start bridge
	if startBridge {
		go func() {
			bridgeCmd.StartBridge(false)
		}()
	}

	server.TrapSignal(func() {
		ctx.Logger.Info("trap signal")

		if tmNode.IsRunning() {
			_ = tmNode.Stop()
		}

		if cpuProfileCleanup != nil {
			cpuProfileCleanup()
		}

		ctx.Logger.Info("exiting...")
	})

	// run forever (the node will not be returned)
	select {}
}

func openDB(rootDir string) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	db, err := sdk.NewLevelDB("application", dataDir)
	return db, err
}

func openTraceWriter(traceWriterFile string) (w io.Writer, err error) {
	if traceWriterFile != "" {
		w, err = os.OpenFile(
			traceWriterFile,
			os.O_WRONLY|os.O_APPEND|os.O_CREATE,
			0666,
		)
		return
	}
	return
}

func showAccountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-account",
		Short: "Print the account's address and public key",
		Run: func(cmd *cobra.Command, args []string) {
			// init heimdall config
			helper.InitHeimdallConfig("")

			// get public keys
			pubObject := helper.GetPubKey()

			account := &ValidatorAccountFormatter{
				Address: ethCommon.BytesToAddress(pubObject.Address().Bytes()).String(),
				PubKey:  "0x" + hex.EncodeToString(pubObject[:]),
			}

			b, err := json.MarshalIndent(account, "", "    ")
			if err != nil {
				panic(err)
			}

			// prints json info
			fmt.Printf("%s", string(b))
		},
	}
}

func showPrivateKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-privatekey",
		Short: "Print the account's private key",
		Run: func(cmd *cobra.Command, args []string) {
			// init heimdall config
			helper.InitHeimdallConfig("")

			// get private and public keys
			privObject := helper.GetPrivKey()

			account := &ValidatorAccountFormatter{
				PrivKey: "0x" + hex.EncodeToString(privObject[:]),
			}

			b, err := json.MarshalIndent(account, "", "    ")
			if err != nil {
				panic(err)
			}

			// prints json info
			fmt.Printf("%s", string(b))
		},
	}
}

// VerifyGenesis verifies the genesis file and brings it in sync with on-chain contract
func VerifyGenesis(ctx *server.Context, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify-genesis",
		Short: "Verify if the genesis matches",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			config := ctx.Config
			config.SetRoot(viper.GetString(cli.HomeFlag))
			helper.InitHeimdallConfig("")

			// Loading genesis doc
			genDoc, err := tmTypes.GenesisDocFromFile(filepath.Join(config.RootDir, "config/genesis.json"))
			if err != nil {
				return err
			}

			// get genesis state
			var genesisState app.GenesisState
			err = json.Unmarshal(genDoc.AppState, &genesisState)
			if err != nil {
				return err
			}

			// verify genesis
			for _, b := range app.ModuleBasics {
				m := b.(hmModule.HeimdallModuleBasic)
				if err := m.VerifyGenesis(genesisState); err != nil {
					return err
				}
			}

			return nil
		},
	}

	return cmd
}

// Total Validators to be included in the testnet
func totalValidators() int {
	numValidators := viper.GetInt(flagNumValidators)
	numNonValidators := viper.GetInt(flagNumNonValidators)
	return numNonValidators + numValidators
}

// get node directory path
func nodeDir(i int) string {
	outDir := viper.GetString(flagOutputDir)
	nodeDirName := fmt.Sprintf("%s%d", viper.GetString(flagNodeDirPrefix), i)
	nodeDaemonHomeName := viper.GetString(flagNodeDaemonHome)
	return filepath.Join(outDir, nodeDirName, nodeDaemonHomeName)
}

// hostname of ip of nodes
func hostnameOrIP(i int) string {
	return fmt.Sprintf("%s%d", viper.GetString(flagNodeHostPrefix), i)
}

// populate persistent peers in config
func populatePersistentPeersInConfigAndWriteIt(config *cfg.Config) {
	persistentPeers := make([]string, totalValidators())
	for i := 0; i < totalValidators(); i++ {
		config.SetRoot(nodeDir(i))
		nodeKey, err := p2p.LoadNodeKey(config.NodeKeyFile())
		if err != nil {
			return
		}
		persistentPeers[i] = p2p.IDAddressString(nodeKey.ID(), fmt.Sprintf("%s:%d", hostnameOrIP(i), 26656))
	}

	persistentPeersList := strings.Join(persistentPeers, ",")
	for i := 0; i < totalValidators(); i++ {
		config.SetRoot(nodeDir(i))
		config.P2P.PersistentPeers = persistentPeersList
		config.P2P.AddrBookStrict = false

		// overwrite default config
		cfg.WriteConfigFile(filepath.Join(nodeDir(i), "config", "config.toml"), config)
	}
}

func getGenesisAccount(address []byte) authTypes.GenesisAccount {
	acc := authTypes.NewBaseAccountWithAddress(hmTypes.BytesToHeimdallAddress(address))
	genesisBalance, _ := big.NewInt(0).SetString("1000000000000000000000", 10)
	if err := acc.SetCoins(sdk.Coins{sdk.Coin{Denom: authTypes.FeeToken, Amount: sdk.NewIntFromBigInt(genesisBalance)}}); err != nil {
		logger.Error("getGenesisAccount | SetCoins", "Error", err)
	}
	result, _ := authTypes.NewGenesisAccountI(&acc)
	return result
}

// WriteGenesisFile creates and writes the genesis configuration to disk. An
// error is returned if building or writing the configuration to file fails.
// nolint: unparam
func writeGenesisFile(genesisTime time.Time, genesisFile, chainID string, appState json.RawMessage) error {
	genDoc := tmTypes.GenesisDoc{
		GenesisTime: genesisTime,
		ChainID:     chainID,
		AppState:    appState,
	}

	if genDoc.GenesisTime.IsZero() {
		genDoc.GenesisTime = tmtime.Now()
	}

	if err := genDoc.ValidateAndComplete(); err != nil {
		return err
	}

	return genDoc.SaveAs(genesisFile)
}

// InitializeNodeValidatorFiles initializes node and priv validator files
func InitializeNodeValidatorFiles(
	config *cfg.Config) (nodeID string, valPubKey crypto.PubKey, priv crypto.PrivKey, err error,
) {

	nodeKey, err := p2p.LoadOrGenNodeKey(config.NodeKeyFile())
	if err != nil {
		return nodeID, valPubKey, priv, err
	}

	nodeID = string(nodeKey.ID())
	server.UpgradeOldPrivValFile(config)

	pvKeyFile := config.PrivValidatorKeyFile()
	if err := common.EnsureDir(filepath.Dir(pvKeyFile), 0777); err != nil {
		return nodeID, valPubKey, priv, err
	}

	pvStateFile := config.PrivValidatorStateFile()
	if err := common.EnsureDir(filepath.Dir(pvStateFile), 0777); err != nil {
		return nodeID, valPubKey, priv, err
	}

	FilePv := privval.LoadOrGenFilePV(pvKeyFile, pvStateFile)
	valPubKey = FilePv.GetPubKey()
	return nodeID, valPubKey, FilePv.Key.PrivKey, nil
}

// WriteDefaultHeimdallConfig writes default heimdall config to the given path
func WriteDefaultHeimdallConfig(path string, conf helper.Configuration) {
	heimdallConf := helper.GetDefaultHeimdallConfig()
	helper.WriteConfigFile(path, &heimdallConf)
}

func CryptoKeyToPubkey(key crypto.PubKey) hmTypes.PubKey {
	validatorPublicKey := helper.GetPubObjects(key)
	return hmTypes.NewPubKey(validatorPublicKey[:])
}
