package helper

import (
	"log"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/crypto/secp256k1"

	"github.com/maticnetwork/heimdall/contracts/stakemanager"
	"github.com/maticnetwork/heimdall/contracts/validatorSet"
	logger "github.com/maticnetwork/heimdall/log"
)

func init() {
	cdc.RegisterConcrete(secp256k1.PubKeySecp256k1{}, secp256k1.Secp256k1PubKeyAminoRoute, nil)
	cdc.RegisterConcrete(secp256k1.PrivKeySecp256k1{}, secp256k1.Secp256k1PrivKeyAminoRoute, nil)

	Logger = logger.NewMainLogger(logger.NewSyncWriter(os.Stdout))

}

// Configuration represents heimdall config
type Configuration struct {
	MainRPCUrl          string `mapstructure:"main_rpcurl"`
	MaticRPCUrl         string `mapstructure:"matic_rpcurl"`
	StakeManagerAddress string `mapstructure:"stakemanager_address"`
	RootchainAddress    string `mapstructure:"rootchain_address"`
	ValidatorSetAddress string `mapstructure:"validatorset_address"`
	ValidatorFilePVPath string `mapstructure:"priv_validator_path"`

	// Tendermint endpoint
	TendermintEndpoint string `mapstructure:"tendermint_endpoint"`
}

var conf Configuration

// MainChainClient stores eth client for Main chain Network
var mainChainClient *ethclient.Client

// MaticClient stores eth client for Matic Network
var maticClient *ethclient.Client

// Logger stores global logger object
var Logger logger.Logger

func InitHeimdallConfig() {
	if strings.Compare(conf.MaticRPCUrl, "") != 0 {
		Logger.Debug("Initializing heimdall config")
		return
	}

	heimdallViper := viper.New()
	heimdallViper.SetConfigName("heimdall-config")         // name of config file (without extension)
	heimdallViper.AddConfigPath("$HOME/.heimdalld/config") // call multiple times to add many search paths
	heimdallViper.AddConfigPath("$HOME/.heimdalld")        // call multiple times to add many search paths

	err := heimdallViper.ReadInConfig()
	if err != nil { // Handle errors reading the config file
		log.Fatal(err)
	}
	if err = heimdallViper.Unmarshal(&conf); err != nil {
		log.Fatal(err)
	}

	// setup eth client
	if mainChainClient, err = ethclient.Dial(GetConfig().MainRPCUrl); err != nil {
		Logger.Error("Error while creating main chain client", "error", err)
		log.Fatal(err)
	}

	if maticClient, err = ethclient.Dial(GetConfig().MaticRPCUrl); err != nil {
		Logger.Error("Error while creating matic chain client", "error", err)
		log.Fatal(err)
	}
}

func GetConfig() Configuration {
	InitHeimdallConfig()
	return conf
}

func GetValidatorSetAddress() common.Address {
	InitHeimdallConfig()
	return common.HexToAddress(GetConfig().ValidatorSetAddress)
}

func GetValidatorSetInstance() (*validatorset.ValidatorSet, error) {
	InitHeimdallConfig()
	validatorSetInstance, err := validatorset.NewValidatorSet(common.HexToAddress(GetConfig().ValidatorSetAddress), mainChainClient)
	if err != nil {
		Logger.Error("Unable to create validator set instance", "error", err)
	}

	return validatorSetInstance, err
}

func GetValidatorSetABI() (abi.ABI, error) {
	return abi.JSON(strings.NewReader(validatorset.ValidatorSetABI))
}

func GetStakeManagerAddress() common.Address {
	InitHeimdallConfig()
	return common.HexToAddress(GetConfig().StakeManagerAddress)
}

func GetStakeManagerInstance() (*stakemanager.Stakemanager, error) {
	InitHeimdallConfig()
	stakeManagerInstance, err := stakemanager.NewStakemanager(common.HexToAddress(GetConfig().StakeManagerAddress), mainChainClient)
	if err != nil {
		Logger.Error("Unable to create stakemanager instance", "error", err)
	}

	return stakeManagerInstance, err
}

func GetStakeManagerABI() (abi.ABI, error) {
	return abi.JSON(strings.NewReader(stakemanager.StakemanagerABI))
}

func GetValidatorDetails() {

}

func GetMainClient() *ethclient.Client {
	InitHeimdallConfig()
	return mainChainClient
}

func GetMaticClient() *ethclient.Client {
	InitHeimdallConfig()
	return maticClient
}
