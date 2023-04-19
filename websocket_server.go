package nethub

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"sync"
	"time"
)

type WebsocketServer struct {
	Addr string
	Opts *ServerOptions
	sync.RWMutex
	clients    map[*WebsocketConn]struct{}
	HttpServer *http.Server

	PongWait     time.Duration
	PingInterval time.Duration
	WriteWait    time.Duration

	OnReceiveMessage   func(message []byte, conn *WebsocketConn)
	OnError            func(err error)
	OnClientConnect    func(conn *WebsocketConn)
	OnClientDisconnect func(conn *WebsocketConn)
}

func NewWebsocketServer(addr string, args ...func(opt *ServerOptions)) *WebsocketServer {
	opts := &ServerOptions{}
	for _, el := range args {
		el(opts)
	}
	ws := &WebsocketServer{
		Addr:         addr,
		Opts:         opts,
		clients:      map[*WebsocketConn]struct{}{},
		PongWait:     15 * time.Second,
		PingInterval: (15 * time.Second * 8) / 10,
		WriteWait:    0,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.accept)
	ws.HttpServer = &http.Server{
		Addr:      ws.Addr,
		Handler:   mux,
		TLSConfig: ws.Opts.Tls,
	}
	return ws
}

func (ws *WebsocketServer) ListenAndServe() error {
	log.Println("websocket Server try listen to ", ws.Addr)
	if ws.Opts.Tls != nil {
		return ws.HttpServer.ListenAndServeTLS("", "")
	} else {
		return ws.HttpServer.ListenAndServe()
	}
}

func (ws *WebsocketServer) accept(w http.ResponseWriter, r *http.Request) {
	auth, err := interface{}(nil), error(nil)
	if ws.Opts.Auth != nil {
		paramsMap := r.URL.Query()
		params := map[string]string{}
		for s, strings := range paramsMap {
			params[s] = strings[0]
		}
		data, _ := json.Marshal(params)
		auth, err = ws.Opts.Auth.CheckFunc(data, nil)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		EnableCompression: true,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	newConn := NewWebsocketConn(conn)
	newConn.PingInterval = ws.PingInterval
	newConn.PongWait = ws.PongWait
	newConn.WriteWait = ws.WriteWait
	newConn.auth = auth
	newConn.OnMessage.AddEventListener(func(data interface{}) {
		if ws.OnReceiveMessage != nil {
			ws.OnReceiveMessage(data.([]byte), newConn)
		}
	})
	newConn.OnError.AddEventListener(func(data interface{}) {
		if ws.OnError != nil {
			ws.OnError(data.(error))
		}
	})
	newConn.OnDisconnect.AddEventListener(func(data interface{}) {
		ws.handleClientDisconnect(newConn)
	})
	ws.handleClientConnect(newConn)
	newConn.StartReadWrite()
}

func (ws *WebsocketServer) handleClientConnect(conn *WebsocketConn) {
	ws.Lock()
	ws.clients[conn] = struct{}{}
	ws.Unlock()
	log.Printf("NetWebsocket connected from: %v", conn.RemoteAddr().String())
	if ws.OnClientConnect != nil {
		ws.OnClientConnect(conn)
	}
}

func (ws *WebsocketServer) handleClientDisconnect(conn *WebsocketConn) {
	ws.Lock()
	delete(ws.clients, conn)
	ws.Unlock()
	log.Println("NetWebsocket disconnected from: " + conn.RemoteAddr().String())
	if ws.OnClientDisconnect != nil {
		ws.OnClientDisconnect(conn)
	}
}
