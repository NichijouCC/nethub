package nethub

import (
	"go.uber.org/zap"
	"strings"
	"sync/atomic"
)

var PubQueue int64
var PubConsume int64

type PubSub struct {
	removeCh  chan string
	subCh     chan *TopicListener
	unsubCh   chan *TopicListener
	pubCh     chan *PubTopic
	clearCh   chan struct{}
	listeners []*TopicListener
}

func newPubSub() *PubSub {
	center := &PubSub{
		removeCh:  make(chan string, 1),
		subCh:     make(chan *TopicListener, 1000),
		unsubCh:   make(chan *TopicListener, 1000),
		pubCh:     make(chan *PubTopic, maxQueueSize),
		clearCh:   make(chan struct{}, 1),
		listeners: []*TopicListener{},
	}

	go func() {
		for {
			select {
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

func (web *PubSub) SubTopic(listener *TopicListener) {
	web.subCh <- listener
}

func (web *PubSub) UnsubTopic(topic string) {
	web.unsubCh <- NewTopicListener(topic, nil)
}

func (web *PubSub) ClearAllSubTopics() {
	web.clearCh <- struct{}{}
}

func (web *PubSub) PubTopic(pkt *TransformedPublishPacket, from *Client) {
	atomic.AddInt64(&PubQueue, 1)
	web.pubCh <- &PubTopic{pkt, from}
}

type PubTopic struct {
	pkt  *TransformedPublishPacket
	from *Client
}

type TopicListener struct {
	topic       string
	callBack    func(pkt *TransformedPublishPacket, from *Client)
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

func NewTopicListener(topic string, callback func(pkt *TransformedPublishPacket, from *Client)) *TopicListener {
	return &TopicListener{
		topic:       topic,
		topicSplits: strings.Split(topic, "/"),
		callBack:    callback,
	}
}
