package node

import (
	"fmt"
	"google.golang.org/grpc"
	"log"
	"net"
	"sync"
)

type GrpcServer struct {
	Addr         string
	server       *grpc.Server
	interceptors []grpc.UnaryServerInterceptor
	services     []registerService

	clients sync.Map
}

type registerService struct {
	Desc    *grpc.ServiceDesc
	Service interface{}
}

func NewGrpcServer() *GrpcServer {
	return &GrpcServer{}
}

func (g *GrpcServer) RegisterServiceHandler(sd *grpc.ServiceDesc, ss interface{}) {
	g.services = append(g.services, registerService{Desc: sd, Service: ss})
}

func (g *GrpcServer) listenAndServer(port int) error {
	g.Addr = fmt.Sprintf(":%v", port)
	log.Printf("grpc Listen to %s", g.Addr)
	var err error
	tcpAddr, err := net.ResolveTCPAddr("tcp", g.Addr)
	if err != nil {
		return err
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}
	defer listener.Close()
	g.server = grpc.NewServer(grpc.ChainUnaryInterceptor(g.interceptors...))
	for _, service := range g.services {
		g.server.RegisterService(service.Desc, service.Service)
	}
	return g.server.Serve(listener)
}
