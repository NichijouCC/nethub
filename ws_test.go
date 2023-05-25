package nethub

import (
	"github.com/google/uuid"
	"log"
	"testing"
)

func TestWebSub_SubGps(t *testing.T) {

	var projectId int64 = 53010217439105
	for i := 1; i < 2; i++ {
		ws := DialHubWebsocket("ws://127.0.0.1:1555", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
		//ws, err := DialWebsocket("ws://192.168.9.86:1555")
		//ws, err := DialWebsocket("ws://139.196.75.44:1555")
		//sn := fmt.Sprintf("T_%03d", i)
		ws.OnLogin.AddEventListener(func(data interface{}) {
			//var topic = fmt.Sprintf("%v/rt_message", sn)
			err := ws.Subscribe("+/rt_message", func(data *PublishPacket, from *Client) {
				log.Println(data.ClientId, string(data.Params))
			})
			if err != nil {
				log.Println("订阅消息出错", err.Error())
			}
			ws.Subscribe("+/gps_message", func(data *PublishPacket, from *Client) {
				log.Println(data.ClientId, string(data.Params))
			})
		})
	}

	select {}
}
