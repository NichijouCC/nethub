package nethub

type IConn interface {
	//异步发送,进入发送队列
	SendMessage(msg []byte) error
	//直接发送
	SendMessageDirect(msg []byte) error
	//关闭
	Close()
	//是否关闭
	IsClosed() bool
	//获取auth信息,可为nil
	GetAuth() interface{}

	ListenToOnDisconnect(func(data interface{}))

	ListenToOnMessage(func(data interface{}))
}
