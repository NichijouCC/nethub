package node

import (
	"fmt"
	"github.com/NichijouCC/nethub/node/util"
	"log"
)

type Server struct {
	*GrpcServer
	ServiceName   string
	ServiceId     string
	BalancePolicy BALANCE_POLICY
	EndPoint      EndPoint
	Register      IRegister
	ToMain        *util.MainServiceMgr
}

type ServerOptions struct {
	ServiceName   string
	ServiceId     string
	BalancePolicy BALANCE_POLICY
	EndPoint      EndPoint
	Register      IRegister
	ToMain        *util.MainServiceMgr
}

type Service struct {
	Name    string `json:"name"`
	Id      string `json:"id"`
	Address string `json:"address"`
}

func NewServer(opts ServerOptions) *Server {
	ser := &Server{
		GrpcServer:    NewGrpcServer(),
		ServiceName:   opts.ServiceName,
		ServiceId:     opts.ServiceId,
		BalancePolicy: opts.BalancePolicy,
		EndPoint:      opts.EndPoint,
		Register:      opts.Register,
		ToMain:        opts.ToMain,
	}
	return ser
}

func (s *Server) Start() error {
	go s.Register.Register(&Service{Name: s.ServiceName, Id: s.ServiceId, Address: fmt.Sprintf("%v:%v", s.EndPoint.Ip, s.EndPoint.Port)})
	if s.BalancePolicy == BALANCE_TO_MAIN {
		go s.ToMain.StartVote(s.ServiceName, s.ServiceId)
	}
	log.Printf("启动服务节点,%v-%v", s.ServiceName, s.ServiceId)
	return s.GrpcServer.listenAndServer(s.EndPoint.Port)
}
