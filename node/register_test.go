package node

import (
	"log"
	"testing"
)

func TestRegister(t *testing.T) {
	register := NewEtcdRegister([]string{"127.0.0.1:2379"})
	register.Register(&Service{
		Name:    "test",
		Index:   1,
		Address: "127.0.0.1:8088",
	})
}

func TestRegisterListServices(t *testing.T) {
	register := NewEtcdRegister([]string{"127.0.0.1:2379"})
	data, _ := register.ListService("test")
	log.Println(data)
}

func TestRegisterWatchService(t *testing.T) {
	register := NewEtcdRegister([]string{"127.0.0.1:2379"})
	ch := register.WatchService("greeter")
	for range ch {
		data, _ := register.ListService("greeter")
		log.Println(data)
	}
}
