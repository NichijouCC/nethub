package util

import (
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log"
	"sync"
	"time"
)

// 服务节点如果采用主备模式,需要进行选主
type MainServiceMgr struct {
	Addr []string
	cli  *clientv3.Client
	//appName->appId
	mainServices sync.Map
}

func NewMainServiceMgr(addr []string) *MainServiceMgr {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   addr,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal("etcd new client err", err)
	}
	return &MainServiceMgr{Addr: addr, cli: cli}
}

func (m *MainServiceMgr) StartVote(serviceName string, serviceId string) {
	cli := m.cli
	for {
		func() {
			lease := clientv3.NewLease(cli)
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			leaseGrantResponse, err := lease.Grant(ctx, 5)
			if err != nil {
				log.Printf("lease grant err,%v", err.Error())
				time.Sleep(time.Second)
				return
			}
			leaseId := leaseGrantResponse.ID
			defer lease.Revoke(context.TODO(), leaseId)

			kv := clientv3.NewKV(cli)
			txn := kv.Txn(context.TODO())
			etcdKey := toEtcdKey(serviceName)
			txn.If(clientv3.Compare(clientv3.CreateRevision(etcdKey), "=", 0)).Then(
				clientv3.OpPut(etcdKey, serviceId, clientv3.WithLease(leaseId))).Else(
				clientv3.OpGet(etcdKey))
			txnResponse, err := txn.Commit()
			if err != nil {
				log.Printf("txn commit err,%v", err.Error())
				return
			}
			// 判断是否抢锁成功
			if txnResponse.Succeeded {
				fmt.Printf("抢到锁,当前节点[%v]选定为服务[%v]主节点. \n", serviceId, serviceName)
				//自动续约
				leaseKeepAliveChan, err := lease.KeepAlive(context.Background(), leaseId)
				if err != nil {
					log.Printf("lease keep alive err,%v", err.Error())
					return
				}
				for resp := range leaseKeepAliveChan {
					if resp != nil {
						//fmt.Println("续租成功,leaseID :", resp.ID)
					} else {
						log.Printf("续租失败")
						break
					}
				}
			} else {
				// 继续回头去抢，不停请求
				resp := txnResponse.Responses[0].GetResponseRange().Kvs[0].Value
				log.Printf("没抢到锁,服务[%v]当前主节点为[%v].", serviceName, string(resp))
				time.Sleep(time.Second * 1)
			}
		}()
	}
}

func (m *MainServiceMgr) StartWatch(serviceName string) {
	cli := m.cli
	//读取值
	etcdKey := toEtcdKey(serviceName)
	resp, err := cli.Get(context.TODO(), etcdKey)
	if err == nil && len(resp.Kvs) > 0 {
		m.mainServices.Store(serviceName, string(resp.Kvs[0].Value))
		log.Printf("服务[%v]当前主节点为[%v]", serviceName, string(resp.Kvs[0].Value))
	}
	//监控
	watchCh := cli.Watch(context.TODO(), etcdKey)
	for res := range watchCh {
		value := res.Events[0].Kv.Value
		m.mainServices.Store(serviceName, string(value))
		log.Printf("服务[%v]主节点修改为[%v]", serviceName, string(value))
	}
}

func (m *MainServiceMgr) FindMainServiceId(serviceName string) (string, bool) {
	serviceAddr, ok := m.mainServices.Load(serviceName)
	if !ok {
		return "", false
	}
	return serviceAddr.(string), true
}

func toEtcdKey(serviceName string) string {
	return fmt.Sprintf("main_service/%v", serviceName)
}
