package main

import (
	"google.golang.org/grpc"
	"log"
	"net"
)

func main() {
	tcpAddr, _ := net.ResolveTCPAddr("tcp", ":8997")
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Fatalln(err)
	}
	defer listener.Close()
	server := grpc.NewServer()
	if err = server.Serve(listener); err != nil {
		log.Fatalln(err)
	}
}
