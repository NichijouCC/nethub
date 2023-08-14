package node

import (
	"github.com/NichijouCC/nethub/node/util"
	"google.golang.org/grpc"
	"log"
)

type Client struct {
	*GrpcClient
	watcher *util.MainServiceMgr
	appName string
}

type ClientConfig struct {
	AppName       string
	BalancePolicy BALANCE_POLICY
}

type ClientOptions struct {
	Register IRegister
}

type ClientOption func(opts *ClientOptions)

func NewClient(config *Config, appName string, opts ...ClientOption) *Client {
	appConfig := config.Apps[appName]
	if appConfig == nil {
		log.Fatalln("APP CLIENT初始失败,配置为空", appName)
	}
	var options ClientOptions
	for _, opt := range opts {
		opt(&options)
	}
	if options.Register == nil {
		options.Register = NewEtcdRegister(config.RegisterAdders)
	}
	cli := &Client{appName: appName}
	cli.GrpcClient = NewGrpcClient(appName, options.Register)
	return cli
}

func (c *Client) Dial(opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return c.GrpcClient.Dial(opts...)
}
