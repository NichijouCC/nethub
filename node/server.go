package node

import (
	"fmt"
	"log"
)

type Server struct {
	*GrpcServer
	voter       *LeaderVoter
	appName     string
	appIndex    int
	appConfig   *AppConfig
	appEndpoint *AppEndPoint
	Opts        ServerOptions
}

type ServerOptions struct {
	Register IRegister
	Voter    *LeaderVoter
}

type ServerOption func(opts *ServerOptions)

func WithRegister(register IRegister) ServerOption {
	return func(opts *ServerOptions) { opts.Register = register }
}

type Service struct {
	Name    string `json:"name"`
	Index   int    `json:"index"`
	Address string `json:"address"`
}

func NewServer(config *Config, appName string, appIndex int, opts ...ServerOption) *Server {
	appConfig := config.Apps[appName]
	endpoint := appConfig.Endpoints[appIndex]
	if appConfig == nil {
		log.Fatalln("APP SERVER初始失败,配置为空", appName)
	}

	var options ServerOptions
	for _, opt := range opts {
		opt(&options)
	}

	if options.Register == nil {
		options.Register = NewEtcdRegister(config.RegisterAdders)
	}

	ser := &Server{
		GrpcServer:  NewGrpcServer(),
		appName:     appName,
		appIndex:    appIndex,
		appConfig:   appConfig,
		appEndpoint: endpoint,
		Opts:        options,
	}

	if appConfig.BalancePolicy == BALANCE_SINGLE {
		ser.voter = NewLeaderVoter(config.VoteAdders, appName, appIndex)
	}

	return ser
}

func (s *Server) Start() error {
	go s.Opts.Register.Register(&Service{Name: s.appName, Index: s.appIndex, Address: fmt.Sprintf("%v:%v", s.appEndpoint.Ip, s.appEndpoint.Port)})
	if s.appConfig.BalancePolicy == BALANCE_SINGLE {
		go s.voter.StartVote()
	}
	log.Printf("启动服务节点,%v-%v", s.appName, s.appIndex)
	return s.GrpcServer.listenAndServer(s.appEndpoint.Port)
}
