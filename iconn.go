package nethub

type IConn interface {
	//异步发送,进入发送队列
	SendMessage(msg []byte) error
	//直接发送
	SendMessageDirect(msg []byte) error
	//关闭
	Close()

	ListenToOnMessage(func(data interface{}))
}
