package nethub

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	pongWait     = 60 * time.Second
	pingInterval = (pongWait * 9) / 10
	writeWait    = 3 * time.Second
)

type WebsocketServer struct {
	Addr string
	Opts *ServerOptions
	sync.RWMutex
	clients map[*WebsocketConn]struct{}

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
	return &WebsocketServer{
		Addr:    addr,
		Opts:    opts,
		clients: map[*WebsocketConn]struct{}{},
	}
}

func (ws *WebsocketServer) ListenAndServe(login func(w http.ResponseWriter, r *http.Request)) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ws.accept)
	mux.HandleFunc("/login", login)
	srv := &http.Server{
		Addr:      ws.Addr,
		Handler:   mux,
		TLSConfig: ws.Opts.Tls,
	}
	log.Println("websocket Server try listen to ", ws.Addr)
	if ws.Opts.Tls != nil {
		return srv.ListenAndServeTLS("", "")
	} else {
		return srv.ListenAndServe()
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
