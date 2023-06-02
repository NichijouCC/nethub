package node

import (
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log"
	"sync/atomic"
	"time"
)

// 服务节点如果采用主备模式,需要进行选主
type LeaderVoter struct {
	AppName atomic.Value
	AppId   atomic.Value
	Addr    []string
}

func NewLeaderVoter(addr []string, appName string, appId int) *LeaderVoter {
	appNameAtt := atomic.Value{}
	appNameAtt.Store(appName)
	appIdAtt := atomic.Value{}
	appIdAtt.Store(appId)
	return &LeaderVoter{AppName: appNameAtt, AppId: appIdAtt, Addr: addr}
}

func (v *LeaderVoter) StartVote() {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   v.Addr,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal("etcd new client err", err)
	}
	defer cli.Close()

	for {
		func() {
			appName := v.AppName.Load().(string)
			appId := v.AppId.Load().(string)
			lease := clientv3.NewLease(cli)
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			leaseGrantResponse, err := lease.Grant(ctx, 5)
			if err != nil {
				fmt.Println("lease grant err", err.Error())
				time.Sleep(time.Second)
				return
			}
			leaseId := leaseGrantResponse.ID
			defer lease.Revoke(context.TODO(), leaseId)

			kv := clientv3.NewKV(cli)
			txn := kv.Txn(context.TODO())
			etcdKey := fmt.Sprintf("/service_leader/%v", appName)
			txn.If(clientv3.Compare(clientv3.CreateRevision(etcdKey), "=", 0)).Then(
				clientv3.OpPut(etcdKey, appId, clientv3.WithLease(leaseId))).Else(
				clientv3.OpGet(etcdKey))
			txnResponse, err := txn.Commit()
			if err != nil {
				fmt.Println("txn commit err", err.Error())
				return
			}
			// 判断是否抢锁成功
			if txnResponse.Succeeded {
				fmt.Printf("抢到锁,当前节点[%v]选定为服务[%v]主节点. \n", appId, appName)
				//自动续约
				leaseKeepAliveChan, err := lease.KeepAlive(context.Background(), leaseId)
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
			} else {
				// 继续回头去抢，不停请求
				resp := txnResponse.Responses[0].GetResponseRange().Kvs[0].Value
				fmt.Printf("没抢到锁,服务[%v]当前主节点为[%v]. \n", appName, string(resp))
				time.Sleep(time.Second * 1)
			}
		}()
	}
}

// 采用主备的服务,调用方需要监听哪个为主服务
type leaderWatcher struct {
	Addr      []string
	AppName   string
	MainAppId atomic.Value
}

func NewLeaderWatcher(addr []string, appName string) *leaderWatcher {
	return &leaderWatcher{Addr: addr, AppName: appName, MainAppId: atomic.Value{}}
}

func (m *leaderWatcher) StartWatch() {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   m.Addr,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal("etcd new client  err", err)
	}
	defer cli.Close()
	//读取值
	etcdKey := fmt.Sprintf("/service_leader/%v", m.AppName)
	resp, err := cli.Get(context.TODO(), etcdKey)
	if err == nil && len(resp.Kvs) > 0 {
		m.MainAppId.Store(string(resp.Kvs[0].Value))
		log.Printf("服务[%v]当前主节点为[%v]", m.AppName, string(resp.Kvs[0].Value))
	}
	//监控
	watchCh := cli.Watch(context.TODO(), etcdKey)
	for res := range watchCh {
		value := res.Events[0].Kv.Value
		m.MainAppId.Store(string(value))
		log.Printf("服务[%v]主节点修改为[%v]", m.AppName, string(value))
	}
}

func (m *leaderWatcher) MainApp() string {
	return m.MainAppId.Load().(string)
}
