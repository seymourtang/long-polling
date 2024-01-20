package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type Config struct {
	LastUpdated int64  `json:"lastUpdated,omitempty"`
	Content     string `json:"content,omitempty"`
}

func Filter[T any](data []T, f func(T) bool) []T {
	ret := make([]T, 0, len(data))
	for _, d := range data {
		if f(d) {
			ret = append(ret, d)
		}
	}
	return ret
}

type CappedQueue[T any] struct {
	items    []T
	lock     sync.RWMutex
	capacity int
}

func NewCappedQueue[T any](capacity int) *CappedQueue[T] {
	return &CappedQueue[T]{
		items:    make([]T, 0, capacity),
		capacity: capacity,
	}
}

func (q *CappedQueue[T]) Append(item T) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if l := len(q.items); l == 0 {
		q.items = append(q.items, item)
	} else {
		to := q.capacity - 1
		if l < q.capacity {
			to = l
		}
		q.items = append([]T{item}, q.items[:to]...)
	}
}

func (q *CappedQueue[T]) Copy() []T {
	q.lock.RLock()
	defer q.lock.RUnlock()

	copied := make([]T, cap(q.items))
	copy(copied, q.items)
	return copied
}

type Store[T any] struct {
	m sync.Map
}

func (s *Store[T]) GetQueue(key string) *CappedQueue[T] {
	v, _ := s.m.LoadOrStore(key, NewCappedQueue[T](32))
	vv, _ := v.(*CappedQueue[T])
	return vv
}

type PubSub struct {
	channels []chan struct{}
	lock     *sync.RWMutex
}

func NewPubSub() *PubSub {
	return &PubSub{
		channels: make([]chan struct{}, 0),
		lock:     new(sync.RWMutex),
	}
}

func (p *PubSub) subscribe() (<-chan struct{}, func()) {
	p.lock.Lock()
	defer p.lock.Unlock()

	c := make(chan struct{}, 1)
	p.channels = append(p.channels, c)
	return c, func() {
		p.lock.Lock()
		defer p.lock.Unlock()

		for i, ch := range p.channels {
			if ch == c {
				p.channels = append(p.channels[:i], p.channels[i+1:]...)
				close(c)
				return
			}
		}
	}
}

func (p *PubSub) publish() {
	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, channel := range p.channels {
		channel <- struct{}{}
	}
}

type Hub struct {
	lock    sync.RWMutex
	pubSubs map[string]*PubSub
}

func NewHub() *Hub {
	return &Hub{
		pubSubs: make(map[string]*PubSub),
	}
}

func (h *Hub) Subscribe(key string) (<-chan struct{}, func()) {
	h.lock.RLock()
	v, ok := h.pubSubs[key]
	if ok {
		h.lock.RUnlock()
		return v.subscribe()
	}
	h.lock.RUnlock()

	h.lock.Lock()

	v, ok = h.pubSubs[key]
	if !ok {
		v = NewPubSub()
		h.pubSubs[key] = v
	}
	h.lock.Unlock()
	return v.subscribe()
}

func (h *Hub) Publish(key string) {
	h.lock.RLock()
	defer h.lock.RUnlock()
	v, ok := h.pubSubs[key]
	if ok {
		v.publish()
		return
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	h := NewHub()
	store := &Store[Config]{}
	mux := http.NewServeMux()
	mux.HandleFunc("/updates", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		key := query.Get("key")
		q := store.GetQueue(key)
		version := query.Get("version")
		lastUpdated, _ := strconv.ParseInt(version, 10, 64)
		log.Printf("query parameters:key:%s,version:%s", key, version)
		getUpdates := func(lastUpdated int64) []Config {
			return Filter(q.Copy(), func(c Config) bool {
				return c.LastUpdated > lastUpdated
			})
		}
		ret := getUpdates(lastUpdated)
		if len(ret) > 0 {
			if err := json.NewEncoder(writer).Encode(ret); err != nil {
				log.Printf("failed to marshal data,err:%s", err.Error())
				http.Error(writer, "internal server err", http.StatusInternalServerError)
				return
			}
			return
		}
		w := query.Get("watch")
		watchSecond, _ := strconv.Atoi(w)
		if watchSecond <= 0 {
			watchSecond = 10
		}
		ch, c := h.Subscribe(key)
		defer c()

		log.Println("long polling")
		select {
		case <-ch:
			ret = getUpdates(lastUpdated)
			if len(ret) == 0 {
				writer.WriteHeader(http.StatusNoContent)
				return
			}
			log.Printf("new updates num(s):%d", len(ret))
			if err := json.NewEncoder(writer).Encode(ret); err != nil {
				log.Printf("failed to marshal data,err:%s", err.Error())
				http.Error(writer, "internal server err", http.StatusInternalServerError)
			}
			return
		case <-time.After(time.Duration(watchSecond) * time.Second):
			writer.WriteHeader(http.StatusNoContent)
		case <-request.Context().Done():
			log.Println("timeout")
			http.Error(writer, "timeout", http.StatusRequestTimeout)
		}
	})
	mux.HandleFunc("/send", func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		key := query.Get("key")
		q := store.GetQueue(key)
		now := time.Now().UnixMilli()
		q.Append(Config{
			LastUpdated: now,
			Content:     fmt.Sprintf("new config is now available:%d", now),
		})
		h.Publish(key)
		_, _ = writer.Write([]byte("ok"))
	})
	s := http.Server{
		Addr:         ":8089",
		Handler:      mux,
		ReadTimeout:  time.Second * 60,
		WriteTimeout: time.Second * 60,
	}
	log.Fatalln(s.ListenAndServe())
}
