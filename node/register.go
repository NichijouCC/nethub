package node

import (
	"context"
	"encoding/json"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log"
	"time"
)

type IRegister interface {
	Register(service *Service)
	ListService(name string) ([]*Service, error)
	WatchService(name string) chan struct{}
}

type EtcdRegister struct {
	ctx    context.Context
	client *clientv3.Client
	Addr   []string
}

func NewEtcdRegister(adders []string) *EtcdRegister {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   adders,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal("etcd new client  err", err)
	}
	return &EtcdRegister{ctx: context.Background(), client: cli, Addr: adders}
}

func (e *EtcdRegister) Register(service *Service) {
	for {
		func() {
			lease := clientv3.NewLease(e.client)
			leaseGrantResponse, err := lease.Grant(context.Background(), 5)
			if err != nil {
				fmt.Println("lease grant err", err.Error())
				time.Sleep(time.Second)
				return
			}
			leaseId := leaseGrantResponse.ID
			defer lease.Revoke(context.TODO(), leaseId)

			data, _ := json.Marshal(service)
			_, err = e.client.Put(e.ctx, e.serviceKey(service.Name, service.Id), string(data), clientv3.WithLease(leaseId))
			if err != nil {
				fmt.Println("etcd put key-value err", err.Error())
				time.Sleep(time.Second)
				return
			}
			leaseKeepAliveChan, err := e.client.KeepAlive(context.Background(), leaseId)
			if err != nil {
				fmt.Println("lease keep alive err", err.Error())
				return
			}
			for resp := range leaseKeepAliveChan {
				if resp != nil {
					//fmt.Println("续租成功,leaseID :", resp.ID)
				} else {
					fmt.Println("续租失败")
					break
				}
			}
		}()
	}
}

func (e *EtcdRegister) servicePrefix(name string) string {
	return fmt.Sprintf("/register/%v", name)
}

func (e *EtcdRegister) serviceKey(name string, id string) string {
	return fmt.Sprintf("/register/%v/%v", name, id)
}

func (e *EtcdRegister) WatchService(name string) chan struct{} {
	watchCh := e.client.Watch(e.ctx, e.servicePrefix(name), clientv3.WithPrefix())
	ch := make(chan struct{}, 1)
	go func() {
		for range watchCh {
			ch <- struct{}{}
		}
	}()
	return ch
}

func (e *EtcdRegister) ListService(name string) ([]*Service, error) {
	resp, err := e.client.Get(context.Background(), e.servicePrefix(name), clientv3.WithPrefix())
	if err != nil {
		fmt.Println("etcd get key-value err", err.Error())
		return nil, err
	}
	var sers []*Service
	for _, kv := range resp.Kvs {
		var data Service
		err = json.Unmarshal(kv.Value, &data)
		if err == nil {
			sers = append(sers, &data)
		}
	}
	return sers, nil
}
