package main

import (
	"github.com/spf13/viper"
	"log"
	"os"
)

type MyConfig struct {
	Mysql string `json:"mysql"`
}

var Config = &MyConfig{}

func InitConfig() {
	vip := viper.New()
	//os.Setenv("GO_ENV", "DEV139")
	switch os.Getenv("GO_ENV") {
	case "PROD":
		vip.SetConfigName("config.prod")
	default:
		vip.SetConfigName("config.local")
	}
	vip.AddConfigPath("./game")
	err := vip.ReadInConfig()
	if err != nil {
		log.Println("初始化项目配置-readInConfig出错", err)
	}
	err = vip.Unmarshal(Config)
	if err != nil {
		log.Println("初始化项目配置-unmarshal出错", err)
	}
	log.Println("项目配置初始化完毕.", Config)
}
