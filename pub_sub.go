package nethub

import (
	"context"
	"go.uber.org/zap"
	"strings"
	"sync/atomic"
)

var PubQueue int64
var PubConsume int64

type PubSub struct {
	ctx       context.Context
	subCh     chan *TopicListener
	unsubCh   chan *TopicListener
	pubCh     chan *PubTopic
	clearCh   chan struct{}
	listeners []*TopicListener
}

func newPubSub(ctx context.Context, pubSize int) *PubSub {
	center := &PubSub{
		ctx:       ctx,
		subCh:     make(chan *TopicListener, 100),
		unsubCh:   make(chan *TopicListener, 100),
		pubCh:     make(chan *PubTopic, pubSize),
		clearCh:   make(chan struct{}, 1),
		listeners: []*TopicListener{},
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sub := <-center.subCh:
				beContain := false
				for _, se := range center.listeners {
					if se.topic == sub.topic {
						beContain = true
						logger.Warn("Client已经订阅了TOPIC", zap.String("topic", sub.topic))
						break
					}
				}
				if !beContain {
					center.listeners = append(center.listeners, sub)
				}
			case unsub := <-center.unsubCh:
				for i := 0; i < len(center.listeners); i++ {
					if center.listeners[i].topic == unsub.topic {
						center.listeners = append(center.listeners[:i], center.listeners[i+1:]...)
						break
					}
				}
			case pub := <-center.pubCh:
				atomic.AddInt64(&PubQueue, -1)
				splitTopics := strings.Split(pub.pkt.Topic, "/")
				for _, listener := range center.listeners {
					if listener.Validate(splitTopics) {
						atomic.AddInt64(&PubConsume, 1)
						listener.callBack(pub.pkt, pub.from)
					}
				}
			case _ = <-center.clearCh:
				center.listeners = nil
			}
		}
	}()
	return center
}

func (p *PubSub) SubTopic(listener *TopicListener) {
	p.subCh <- listener
}

func (p *PubSub) UnsubTopic(topic string) {
	p.unsubCh <- NewTopicListener(topic, nil)
}

func (p *PubSub) ClearAllSubTopics() {
	p.clearCh <- struct{}{}
}

func (p *PubSub) PubTopic(pkt *PublishPacket, from *Client) {
	atomic.AddInt64(&PubQueue, 1)
	p.pubCh <- &PubTopic{pkt, from}
}

type PubTopic struct {
	pkt  *PublishPacket
	from *Client
}

type TopicListener struct {
	topic       string
	callBack    func(pkt *PublishPacket, from *Client)
	topicSplits []string
	clientId    string
}

func (t *TopicListener) Validate(pubTopic []string) bool {
	if len(t.topicSplits) > len(pubTopic) {
		return false
	}
	for i, split := range t.topicSplits {
		if split == "#" {
			return true
		}
		if split == "+" {
			continue
		}
		if split != pubTopic[i] {
			return false
		}
	}
	return true
}

func NewTopicListener(topic string, callback func(pkt *PublishPacket, from *Client)) *TopicListener {
	return &TopicListener{
		topic:       topic,
		topicSplits: strings.Split(topic, "/"),
		callBack:    callback,
	}
}
