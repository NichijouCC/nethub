package nethub

import (
	"fmt"
	"sync"
)

type Bucket struct {
	parent   *Bucket
	bucketId int64
	//ClientId->*Client
	clients sync.Map
	*PubSub
}

func newBucket(id int64, parent *Bucket) *Bucket {
	bucket := &Bucket{
		bucketId: id,
		parent:   parent,
		PubSub:   newPubSub(),
	}
	return bucket
}

func (m *Bucket) Broadcast(packet []byte) {
	m.clients.Range(func(key, value any) bool {
		value.(*Client).SendMessage(packet)
		return true
	})
}

func (m *Bucket) PubTopic(pkt *TransformedPublishPacket, from *Client) {
	if m.parent != nil {
		m.parent.PubTopic(&TransformedPublishPacket{
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

func (m *Bucket) FindClient(id string) (*Client, bool) {
	value, ok := m.clients.Load(id)
	if !ok {
		return nil, false
	}
	return value.(*Client), true
}

func (m *Bucket) AddClient(client *Client) {
	m.clients.Store(client.ClientId, client)
	client.Bucket = m
	m.PubTopic(&TransformedPublishPacket{Topic: "connect"}, client)
	client.OnDisconnect.AddEventListener(func(data interface{}) {
		m.clients.Delete(client.ClientId)
		m.PubTopic(&TransformedPublishPacket{Topic: "disconnect"}, client)
	})
}
