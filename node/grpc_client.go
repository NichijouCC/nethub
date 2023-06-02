package node

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/serialx/hashring"
	"google.golang.org/grpc"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

const (
	balanceAttrKey       = "balance_attr_key"
	schemeKey            = "router"
	metaBalancePolicyKey = "meta_balance_policy"
	metaSingleId         = "meta_single_id"

	balancePolicy     = "pipi_balance_policy"
	BalanceRoundRobin = "balance_round_robin"
	BalanceRandom     = "balance_random"
	BalanceSingle     = "balance_single"
)

type BALANCE_POLICY string

var (
	BALANCE_ROUND_ROBIN BALANCE_POLICY = "balance_round_robin"
	BALANCE_RANDOM      BALANCE_POLICY = "balance_random"
	BALANCE_SINGLE      BALANCE_POLICY = "balance_single"
)

type resolverBuilder struct {
	register IRegister
}

func newResolverBuilder(register IRegister) *resolverBuilder {
	return &resolverBuilder{register: register}
}

func (g *resolverBuilder) Scheme() string {
	return schemeKey
}

func (g *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &schemeResolver{}
	go func() {
		update := func() {
			instances, err := g.register.ListService(target.URL.Host)
			if err != nil {
				log.Printf("GetService %s fail err : %s", target.URL.Host, err.Error())
				return
			}
			var newAddrs []resolver.Address
			for _, instance := range instances {
				newAddrs = append(newAddrs, resolver.Address{
					Addr:               instance.Address,
					BalancerAttributes: attributes.New(balanceAttrKey, instance.Index),
				})
			}
			cc.UpdateState(resolver.State{Addresses: newAddrs})
		}
		update()
		ch := g.register.WatchService(target.URL.Host)
		for range ch {
			update()
		}
	}()
	return r, nil
}

type schemeResolver struct {
}

func (g *schemeResolver) ResolveNow(options resolver.ResolveNowOptions) {
}

func (g *schemeResolver) Close() {
}

type balancePickerBuilder struct {
}

func (b *balancePickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	subConns := make(map[string]balancer.SubConn)
	var subConnList []balancer.SubConn
	var idList []string
	for subConn, conInfo := range info.ReadySCs {
		v := conInfo.Address.BalancerAttributes.Value(balanceAttrKey)
		if v == nil {
			continue
		}
		id := fmt.Sprintf("%v", v)
		subConns[id] = subConn
		subConnList = append(subConnList, subConn)
		idList = append(idList, id)
	}
	return &balancePicker{subConnDic: subConns, subConns: subConnList, hashRing: hashring.New(idList)}
}

type balancePicker struct {
	subConnDic map[string]balancer.SubConn
	subConns   []balancer.SubConn
	hashRing   *hashring.HashRing
	next       uint32
}

func (b *balancePicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	data, ok := metadata.FromOutgoingContext(info.Ctx)
	if !ok {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	balanceType := data[metaBalancePolicyKey]
	if len(balanceType) != 1 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	switch balanceType[0] {
	case BalanceRandom:
		return balancer.PickResult{SubConn: b.subConns[rand.Intn(len(b.subConns))]}, nil
	case BalanceRoundRobin:
		nextIndex := atomic.AddUint32(&b.next, 1)
		return balancer.PickResult{SubConn: b.subConns[nextIndex%uint32(len(b.subConns))]}, nil
	default:
		return balancer.PickResult{}, fmt.Errorf("未知路由类型[%v]", balanceType[0])
	}
}

type GrpcClient struct {
	conn          *grpc.ClientConn
	interceptors  []grpc.UnaryClientInterceptor
	balancePolicy BALANCE_POLICY
	policyInit    sync.Once
	resolver      *resolverBuilder
}

var balancerInit sync.Once

func NewGrpcClient(register IRegister) *GrpcClient {
	client := &GrpcClient{
		balancePolicy: BalanceRoundRobin,
		resolver:      newResolverBuilder(register),
	}
	balancerInit.Do(func() {
		balancer.Register(base.NewBalancerBuilder(balancePolicy, &balancePickerBuilder{}, base.Config{HealthCheck: true}))
	})
	return client
}

// 主备服务,设置client均衡策略为single
func (g *GrpcClient) SetBalanceSingle(watcher *leaderWatcher) {
	g.policyInit.Do(func() {
		g.balancePolicy = BalanceSingle
		g.AddInterceptor(func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx = metadata.AppendToOutgoingContext(ctx, metaBalancePolicyKey, BalanceSingle, metaSingleId, watcher.MainApp())
			return nil
		})
	})
}

// 设置client均衡策略为随机或者轮流策略
func (g *GrpcClient) SetBalancePolicy(balancePolicy BALANCE_POLICY) {
	g.policyInit.Do(func() {
		g.balancePolicy = balancePolicy
		g.AddInterceptor(func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx = metadata.AppendToOutgoingContext(ctx, metaBalancePolicyKey, string(balancePolicy))
			return invoker(ctx, method, req, reply, cc, opts...)
		})
	})
}

func (g *GrpcClient) AddInterceptor(interceptor grpc.UnaryClientInterceptor) {
	g.interceptors = append(g.interceptors, interceptor)
}

func (g *GrpcClient) Dial(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	targetAddr := fmt.Sprintf("%v://%s", schemeKey, target)
	opts = append(opts,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":"%v"}`, balancePolicy)),
		grpc.WithChainUnaryInterceptor(g.interceptors...),
		grpc.WithResolvers(g.resolver),
	)
	ctx, _ := context.WithTimeout(context.Background(), time.Second*10)
	if conn, err := grpc.DialContext(ctx, targetAddr, opts...); err != nil {
		return nil, errors.Wrapf(err, "Dial Target %s", targetAddr)
	} else {
		g.conn = conn
		return conn, err
	}
}
