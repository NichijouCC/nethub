package node

import (
	"encoding/json"
	"github.com/spf13/viper"
	"log"
	"os"
)

func Init(path string) *Config {
	vip := viper.New()
	switch os.Getenv("GO_ENV") {
	case "PROD":
		vip.SetConfigName("config.prod")
	case "DEV":
		vip.SetConfigName("config.dev")
	case "LOCAL":
		vip.SetConfigName("config.local")
	default:
		vip.SetConfigName("config.local")
	}
	vip.AddConfigPath(path)
	err := vip.ReadInConfig()
	if err != nil {
		log.Fatalf("初始化项目配置-readInConfig出错,%v", err.Error())
	}
	var config = &Config{}
	err = vip.Unmarshal(config)
	if err != nil {
		log.Fatalf("初始化项目配置-unmarshal出错,%v", err.Error())
	}
	configBytes, _ := json.Marshal(config)
	log.Printf("项目配置初始化完毕.配置（%v）", string(configBytes))
	return config
}

type Config struct {
	RegisterAdders []string              `json:"register_adders"`
	VoteAdders     []string              `json:"vote_adders"`
	Apps           map[string]*AppConfig `json:"apps"`
}

type AppConfig struct {
	Endpoints map[string]*EndPoint `json:"endpoints"`
	//负载均衡方式,包含
	BalancePolicy BALANCE_POLICY `json:"balance_policy"`
}

type EndPoint struct {
	//服务ip,grpc注册
	Ip string `json:"ip"`
	//服务端口,grpc注册
	Port int `json:"port"`
}
