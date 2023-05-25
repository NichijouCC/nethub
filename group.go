package nethub

import (
	"context"
	"fmt"
	"sync"
)

type Group struct {
	ctx      context.Context
	cancel   context.CancelFunc
	parent   *Group
	bucketId int64
	//ClientId->*Client
	clients sync.Map
	*PubSub
}

// group发布队列大小
var GroupPubChLen = 1000

func newGroup(id int64, parent *Group) *Group {
	ctx, cancel := context.WithCancel(context.Background())
	bucket := &Group{
		ctx:      ctx,
		cancel:   cancel,
		bucketId: id,
		parent:   parent,
		PubSub:   newPubSub(ctx, GroupPubChLen),
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
			Topic:    fmt.Sprintf("%v/%v", m.bucketId, pkt.Topic),
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
	m.clients.Store(client.ClientId, client)
	client.Group = m
	m.PubTopic(&PublishPacket{Topic: "connect"}, client)
	client.OnDispose.AddEventListener(func(data interface{}) {
		m.clients.Delete(client.ClientId)
		m.PubTopic(&PublishPacket{Topic: "disconnect"}, client)
	})
}
