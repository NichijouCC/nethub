package balancer

import (
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/metadata"
)

const ToMain = "toMain"
const META_SERVICE_NAME = "meta_service_name"
const BALANCE_ATT_ID = "blance_att_service_id"

func InitToMain(findMainAddr func(serviceName string) string) {
	balancer.Register(newBuilder(findMainAddr))
}

func newBuilder(findMainAddr func(serviceName string) string) balancer.Builder {
	return base.NewBalancerBuilder(ToMain, &mainPickerBuilder{findMainAddr: findMainAddr}, base.Config{HealthCheck: true})
}

type mainPickerBuilder struct {
	findMainAddr func(serviceName string) string
}

func (b *mainPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	subConns := make(map[string]balancer.SubConn)
	for subConn, conInfo := range info.ReadySCs {
		v := conInfo.Address.BalancerAttributes.Value(BALANCE_ATT_ID)
		if v == nil {
			continue
		}
		if serviceId, ok := v.(string); ok {
			subConns[serviceId] = subConn
		}
	}
	return &mainPicker{subConns: subConns}
}

type mainPicker struct {
	subConns   map[string]balancer.SubConn
	findMainId func(serviceName string) string
}

func (b *mainPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	data, ok := metadata.FromOutgoingContext(info.Ctx)
	if !ok {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	serviceName := data[META_SERVICE_NAME]
	if len(serviceName) != 1 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}
	serviceId := b.findMainId(serviceName[0])
	if subConn, ok := b.subConns[serviceId]; ok {
		return balancer.PickResult{SubConn: subConn}, balancer.ErrNoSubConnAvailable
	}
	return balancer.PickResult{}, nil
}
