package nethub

import (
	"sync"
)

type handlerMgr struct {
	//map[string]requestHandler
	handlers sync.Map
	//map[string]streamHandler
	streamHandlers       sync.Map
	defaultHandler       requestHandler
	defaultStreamHandler streamHandler
}

func (h *handlerMgr) RegisterRequestHandler(method string, handler requestHandler) {
	h.handlers.Store(method, handler)
}

func (h *handlerMgr) findRequestHandler(method string) (requestHandler, bool) {
	handler, loaded := h.handlers.Load(method)
	if loaded {
		return handler.(requestHandler), true
	}
	if h.defaultHandler != nil {
		return h.defaultHandler, true
	}
	return nil, false
}

func (h *handlerMgr) RegisterStreamHandler(method string, handler streamHandler) {
	h.streamHandlers.Store(method, handler)
}

func (h *handlerMgr) findStreamHandler(method string) (streamHandler, bool) {
	handler, loaded := h.streamHandlers.Load(method)
	if loaded {
		return handler.(streamHandler), true
	}
	if h.defaultStreamHandler != nil {
		return h.defaultStreamHandler, true
	}
	return nil, false
}
