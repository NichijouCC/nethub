package nethub

import (
	"fmt"
	"reflect"
	"sync"
)

type IEventEmitter interface {
	AddListener(event string, listener interface{}) error
	On(event string, listener interface{}) error
	Once(event string, listener interface{}) error
	Off(event string, listener interface{}) error
	RemoveListener(event string, listener interface{}) error
	RemoveAllListeners(events ...string)
	Emit(event string, args ...interface{})
	EventNames() []string
	Listeners(event string) []interface{}
	ListenerCount(event string) int
}

type eventListener struct {
	callback reflect.Value
	once     bool
}

type EventEmitter struct {
	listeners map[string][]*eventListener
}

func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		listeners: map[string][]*eventListener{},
	}
}

func (e *EventEmitter) AddListener(event string, listener interface{}) error {
	err := isValidListener(listener)
	if err != nil {
		return err
	}
	e.listeners[event] = append(e.listeners[event], &eventListener{callback: reflect.ValueOf(listener)})
	return nil
}

func (e *EventEmitter) On(event string, listener interface{}) error {
	return e.AddListener(event, listener)
}

func (e *EventEmitter) Once(event string, listener interface{}) error {
	err := isValidListener(listener)
	if err != nil {
		return err
	}
	e.listeners[event] = append(e.listeners[event], &eventListener{callback: reflect.ValueOf(listener), once: true})
	return nil
}

func (e *EventEmitter) Off(event string, listener interface{}) error {
	return e.RemoveListener(event, listener)
}
func (e *EventEmitter) RemoveListener(event string, listener interface{}) error {
	err := isValidListener(listener)
	if err != nil {
		return err
	}
	rv := reflect.ValueOf(listener)
	if list, ok := e.listeners[event]; ok {
		for index, el := range list {
			if el.callback == rv {
				if len(list) == 1 {
					delete(e.listeners, event)
				} else {
					e.listeners[event] = append(list[:index], list[index+1:]...)
				}
				return nil
			}
		}
	}
	return nil
}

func (e *EventEmitter) RemoveAllListeners(events ...string) {
	if len(events) > 0 {
		for _, el := range events {
			delete(e.listeners, el)
		}
	} else {
		e.listeners = map[string][]*eventListener{}
	}
}

func (e *EventEmitter) Emit(event string, args ...interface{}) {
	reflectedArgs := argsToReflectValues(args...)
	if list, ok := e.listeners[event]; ok {
		for i := 0; i < len(list); i++ {
			list[i].callback.Call(reflectedArgs)
			if list[i].once {
				list = append(list[:i], list[i+1:]...)
				i--
			}
		}
	}
}

func (e *EventEmitter) EventNames() []string {
	var arr []string
	for el, _ := range e.listeners {
		arr = append(arr, el)
	}
	return arr
}

func (e *EventEmitter) Listeners(event string) []interface{} {
	var arr []interface{}
	if list, ok := e.listeners[event]; ok {
		for _, el := range list {
			arr = append(arr, el.callback.Interface())
		}
	}
	return arr
}

func (e *EventEmitter) ListenerCount(event string) int {
	if list, ok := e.listeners[event]; ok {
		return len(list)
	}
	return 0
}

func isValidListener(fn interface{}) error {
	if reflect.TypeOf(fn).Kind() != reflect.Func {
		return fmt.Errorf("%s is not a reflect.Func", reflect.TypeOf(fn))
	}
	return nil
}

func argsToReflectValues(args ...interface{}) []reflect.Value {
	reflectedArgs := make([]reflect.Value, 0)
	for _, arg := range args {
		reflectedArgs = append(reflectedArgs, reflect.ValueOf(arg))
	}
	return reflectedArgs
}

type EventTarget struct {
	listenerMtx sync.RWMutex
	listeners   []func(data interface{})
}

func newEventTarget() *EventTarget {
	return &EventTarget{}
}
func (e *EventTarget) AddEventListener(listener func(data interface{})) {
	e.listenerMtx.Lock()
	defer e.listenerMtx.Unlock()
	e.listeners = append(e.listeners, listener)
}

func (e *EventTarget) RiseEvent(data interface{}) {
	e.listenerMtx.RLock()
	defer e.listenerMtx.RUnlock()
	for _, f := range e.listeners {
		f(data)
	}
}
