package node

import (
	"context"
	"fmt"
	balancer2 "github.com/NichijouCC/nethub/node/balancer"
	"github.com/NichijouCC/nethub/node/util"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"log"
	"sync"
	"time"
)

const schemeKey = "router"

type BALANCE_POLICY string

var (
	BALANCE_ROUND_ROBIN BALANCE_POLICY = roundrobin.Name
	BALANCE_TO_MAIN     BALANCE_POLICY = balancer2.ToMain
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
					BalancerAttributes: attributes.New(balancer2.BALANCE_ATT_ID, instance.Id),
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

type GrpcClient struct {
	serviceName   string
	conn          *grpc.ClientConn
	interceptors  []grpc.UnaryClientInterceptor
	balancePolicy BALANCE_POLICY
	toMainMgr     *util.MainServiceMgr
	policyInit    sync.Once
	resolver      *resolverBuilder
}

func NewGrpcClient(serviceName string, register IRegister) *GrpcClient {
	client := &GrpcClient{
		serviceName:   serviceName,
		balancePolicy: roundrobin.Name,
		resolver:      newResolverBuilder(register),
		interceptors: []grpc.UnaryClientInterceptor{func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			metadata.AppendToOutgoingContext(ctx, balancer2.META_SERVICE_NAME, serviceName)
			return invoker(ctx, method, req, reply, cc, opts...)
		}},
	}
	return client
}

// 轮流策略
func (g *GrpcClient) UseRoundRobinBalancer() {
	g.balancePolicy = roundrobin.Name
}

// 主备策略
func (g *GrpcClient) UseToMainBalancer(mgr *util.MainServiceMgr) {
	g.balancePolicy = balancer2.ToMain
	go func() {
		go mgr.StartWatch(g.serviceName)
		balancer2.InitToMain(func(serviceName string) string {
			addr, _ := mgr.FindMainServiceId(serviceName)
			return addr
		})
	}()
}

func (g *GrpcClient) AddInterceptor(interceptor grpc.UnaryClientInterceptor) {
	g.interceptors = append(g.interceptors, interceptor)
}

func (g *GrpcClient) Dial(opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	targetAddr := fmt.Sprintf("%v://%s", schemeKey, g.serviceName)
	opts = append(opts,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingPolicy":"%v"}`, g.balancePolicy)),
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
