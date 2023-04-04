# Nethub
兼容多种协议（tcp、udp、websocket）支持多种模式（请求回复、发布订阅、双向流）进行数据交互

# 支持特性
1. 请求回复（远程调用）
服务端注册服务
```
hub := New(&HubOptions{
    HeartbeatTimeout: 10,
    RetryTimeout:     10,
    RetryInterval:    3,
})
hub.ListenAndServeTcp(":1235", 2)

hub.RegisterRequestHandler("add", func(pkt *Packet) (interface{}, error) {
    req := pkt.PacketContent.(*RequestPacket)
    _ = req.Params
    //todo
    return nil, nil
})
```
客户端调用服务
```
client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String()})
client.OnLogin.AddEventListener(func(data interface{}) {
    //远程调用
    result, err := client.RequestWithRetry("load_data", map[string]interface{}{"a": 1, "b": 2})
    log.Println(result, err)
})
```
2. 支持分Room的发布订阅
   1. 发布订阅topic支持同MQTT一样的统配符
   2. 如果客户属于某Room，登录后自动进入该Room,可以订阅该Room的消息，不属于room则会进入世界空间中，拥有监听所有消息的权限
   3. 客户端发布xx的Topic，服务端在客户端所在Room按照``${ClientID}/${xx}``进行发布，在世界空间按照``${RoomId}/${ClientID}/${xx}`` 进行发布
   4. 如果消息为json型，支持订阅消息的部分字段


客户端发布TOPIC
```
 var projectId int64 = 53010217439105
 client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
 client.OnLogin.AddEventListener(func(data interface{}) {
     //模拟实时上报消息
     ticker := time.NewTicker(time.Second)
     for range ticker.C {
         client.Publish("rt_message", `{"timestamp": 1671610334.1461706, "network_latency": 100.0, "longitude": 101.75232644, "latitude": 26.63366599, "altitude": 1116.06578656}`)
     }
 })
```
客户端订阅TOPIC
```
 var projectId int64 = 53010217439105
 client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
 client.OnLogin.AddEventListener(func(data interface{}) {
     client.Subscribe("+/rt_message", func(data *TransformedPublishPacket, from *Client) {
         log.Println(data.ClientId, string(data.Params))
     })
 })
```
客户端订阅TOPIC的部分字段
```
 var projectId int64 = 53010217439105
 client := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
 client.OnLogin.AddEventListener(func(data interface{}) {
     var gpsAtts = []string{"longitude", "latitude", "altitude", "yaw"}
     client.SubscribeAttributes("+/rt_message", gpsAtts, func(data *TransformedPublishPacket, from *Client) {
         log.Println(data.ClientId, string(data.Params))
     })
 })
```
3. 支持客户端服务端按照类似GRPC的Stream流形式进行交互
服务端注册服务
```
hub.RegisterStreamHandler("add", func(first *Packet, stream *Stream) {
     //初次触发,执行
     stream.OnRequest = func(pkt *StreamRequestPacket) {
         _ = pkt.Method
         _ = pkt.Params
         _ = pkt.Id
         //持续处理客户端发来的流消息
         //todo

         //发送消息到客户端,举例
         stream.RequestWithRetry("xx", nil)
         //服务端可以主动关闭数据流,举例
         stream.Close()
     }
 })
```

客户端调用服务
```
 var projectId int64 = 53010217439105
 conn := DialHubTcp("127.0.0.1:1235", LoginParams{ClientId: uuid.New().String(), BucketId: &projectId})
 conn.OnLogin.AddEventListener(func(data interface{}) {
     //初始化流
     stream := conn.StreamRequest("load_data", nil)
     go func() {
         //stream close监听
         <-stream.CloseCh
     }()

     for j := 0; j < 100; j++ {
         //持续发送流消息
         err := stream.Request(map[string]interface{}{"pp": j})
         if err != nil {
             log.Println("error", err)
         }
     }
     //处理服务端返回的回复
     stream.OnResponse = func(pkt *StreamResponsePacket) {
         str, _ := json.Marshal(pkt)
         log.Println("client receive pkt", string(str))
     }
     //客户端主动关闭数据流
     stream.Close()
 })
```