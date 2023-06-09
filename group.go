package nethub

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"sync"
)

type Group struct {
	ctx    context.Context
	cancel context.CancelFunc
	parent *Group
	id     int64
	//ClientId->*Client
	clients sync.Map
	*PubSub
}

// group发布队列大小
var GroupPubChLen = 100

func newGroup(id int64, parent *Group) *Group {
	ctx, cancel := context.WithCancel(context.Background())
	bucket := &Group{
		ctx:    ctx,
		cancel: cancel,
		id:     id,
		parent: parent,
		PubSub: newPubSub(ctx, GroupPubChLen),
	}
	return bucket
}

func (m *Group) Broadcast(packet []byte) {
	m.clients.Range(func(key, value any) bool {
		value.(*Client).SendMessage(packet)
		return true
	})
}

func (m *Group) PubTopic(pkt *PublishPacket, from *Client) {
	if m.parent != nil {
		m.parent.PubTopic(&PublishPacket{
			Id:       pkt.Id,
			Topic:    fmt.Sprintf("%v/%v", m.id, pkt.Topic),
			Params:   pkt.Params,
			ClientId: pkt.ClientId,
		}, from)
	}
	m.PubSub.PubTopic(pkt, from)
	m.clients.Range(func(key, value any) bool {
		value.(*Client).PubTopic(pkt, from)
		return true
	})
}

func (m *Group) FindClient(id string) (*Client, bool) {
	value, ok := m.clients.Load(id)
	if !ok {
		return nil, false
	}
	return value.(*Client), true
}

func (m *Group) AddClient(client *Client) {
	logger.Info(fmt.Sprintf("GROUP[%v]新增加客户端", m.id), zap.String("clientId", client.ClientId))
	m.clients.Store(client.ClientId, client)
	client.Group = m
}

func (m *Group) RemoveClient(clientId string) {
	logger.Info(fmt.Sprintf("GROUP[%v]移除客户端", m.id), zap.String("clientId", clientId))
	m.clients.Delete(clientId)
}
