package node

import (
	"google.golang.org/grpc"
	"log"
)

type Client struct {
	*GrpcClient
	watcher *leaderWatcher
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
	cli.GrpcClient = NewGrpcClient(options.Register)
	if appConfig.BalancePolicy == BALANCE_SINGLE {
		cli.watcher = NewLeaderWatcher(config.VoteAdders, appName)
		go cli.watcher.StartWatch()
		cli.GrpcClient.SetBalanceSingle(cli.watcher)
	} else {
		cli.GrpcClient.SetBalancePolicy(appConfig.BalancePolicy)
	}
	return cli
}

func (c *Client) Dial(opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return c.GrpcClient.Dial(c.appName, opts...)
}
