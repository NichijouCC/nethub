package nethub

import (
	"sync"
)

type HandlerMgr struct {
	//map[string]requestHandler
	handlers sync.Map
	//map[string]streamHandler
	streamHandlers       sync.Map
	defaultHandler       requestHandler
	defaultStreamHandler streamHandler
}

func (h *HandlerMgr) RegisterRequestHandler(method string, handler requestHandler) {
	h.handlers.Store(method, handler)
}

func (h *HandlerMgr) findRequestHandler(method string) (requestHandler, bool) {
	handler, loaded := h.handlers.Load(method)
	if loaded {
		return handler.(requestHandler), true
	}
	if h.defaultHandler != nil {
		return h.defaultHandler, true
	}
	return nil, false
}

func (h *HandlerMgr) RegisterStreamHandler(method string, handler streamHandler) {
	h.streamHandlers.Store(method, handler)
}

func (h *HandlerMgr) findStreamHandler(method string) (streamHandler, bool) {
	handler, loaded := h.streamHandlers.Load(method)
	if loaded {
		return handler.(streamHandler), true
	}
	if h.defaultStreamHandler != nil {
		return h.defaultStreamHandler, true
	}
	return nil, false
}
