package agency

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type EventBus interface {
	Publish(context.Context, WakeSignal) error
	Subscribe(context.Context, string) (<-chan WakeSignal, error)
	Close(context.Context) error
}

type MemoryEventBus struct {
	mu     sync.RWMutex
	subs   map[string]map[chan WakeSignal]struct{}
	closed bool
}

func NewMemoryEventBus() *MemoryEventBus {
	return &MemoryEventBus{
		subs: make(map[string]map[chan WakeSignal]struct{}),
	}
}

func (b *MemoryEventBus) Publish(ctx context.Context, signal WakeSignal) error {
	_ = ctx
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return nil
	}
	for ch := range b.subs[signal.Channel] {
		select {
		case ch <- signal:
		default:
		}
	}
	return nil
}

func (b *MemoryEventBus) Subscribe(ctx context.Context, channel string) (<-chan WakeSignal, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(chan WakeSignal, 32)
	if b.closed {
		close(out)
		return out, nil
	}
	if _, ok := b.subs[channel]; !ok {
		b.subs[channel] = make(map[chan WakeSignal]struct{})
	}
	b.subs[channel][out] = struct{}{}
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subs[channel], out)
		close(out)
	}()
	return out, nil
}

func (b *MemoryEventBus) Close(ctx context.Context) error {
	_ = ctx
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	for _, group := range b.subs {
		for ch := range group {
			close(ch)
		}
	}
	b.subs = map[string]map[chan WakeSignal]struct{}{}
	return nil
}

type RedisConfig struct {
	Addr        string
	Password    string
	DB          int
	DialTimeout time.Duration
}

type RedisEventBus struct {
	cfg   RedisConfig
	mu    sync.Mutex
	conns []net.Conn
}

func NewRedisEventBus(cfg RedisConfig) *RedisEventBus {
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 3 * time.Second
	}
	return &RedisEventBus{cfg: cfg}
}

func (b *RedisEventBus) Publish(ctx context.Context, signal WakeSignal) error {
	conn, err := b.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	payload, err := json.Marshal(signal)
	if err != nil {
		return err
	}
	if err := writeRedisCommand(conn, "PUBLISH", signal.Channel, string(payload)); err != nil {
		return err
	}
	return nil
}

func (b *RedisEventBus) Subscribe(ctx context.Context, channel string) (<-chan WakeSignal, error) {
	conn, err := b.dial(ctx)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.conns = append(b.conns, conn)
	b.mu.Unlock()

	if err := writeRedisCommand(conn, "SUBSCRIBE", channel); err != nil {
		conn.Close()
		return nil, err
	}

	out := make(chan WakeSignal, 32)
	go b.readLoop(ctx, conn, out)
	return out, nil
}

func (b *RedisEventBus) Close(ctx context.Context) error {
	_ = ctx
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, conn := range b.conns {
		_ = conn.Close()
	}
	b.conns = nil
	return nil
}

func (b *RedisEventBus) dial(ctx context.Context) (net.Conn, error) {
	dialer := net.Dialer{Timeout: b.cfg.DialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", b.cfg.Addr)
	if err != nil {
		return nil, err
	}
	if b.cfg.Password != "" {
		if err := writeRedisCommand(conn, "AUTH", b.cfg.Password); err != nil {
			conn.Close()
			return nil, err
		}
	}
	if b.cfg.DB > 0 {
		if err := writeRedisCommand(conn, "SELECT", strconv.Itoa(b.cfg.DB)); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

func (b *RedisEventBus) readLoop(ctx context.Context, conn net.Conn, out chan WakeSignal) {
	defer close(out)
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msg, err := readRedisPubSubMessage(reader)
		if err != nil {
			return
		}
		var signal WakeSignal
		if err := json.Unmarshal([]byte(msg), &signal); err != nil {
			continue
		}
		select {
		case out <- signal:
		case <-ctx.Done():
			return
		}
	}
}

func writeRedisCommand(conn net.Conn, args ...string) error {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(args)))
	b.WriteString("\r\n")
	for _, arg := range args {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(arg)))
		b.WriteString("\r\n")
		b.WriteString(arg)
		b.WriteString("\r\n")
	}
	_, err := conn.Write([]byte(b.String()))
	return err
}

func readRedisPubSubMessage(r *bufio.Reader) (string, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	if prefix != '*' {
		return "", fmt.Errorf("unexpected RESP prefix: %q", prefix)
	}
	countLine, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	count, err := strconv.Atoi(strings.TrimSpace(countLine))
	if err != nil {
		return "", err
	}
	parts := make([]string, 0, count)
	for i := 0; i < count; i++ {
		kind, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if kind != '$' {
			return "", fmt.Errorf("unexpected RESP kind: %q", kind)
		}
		lenLine, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		size, err := strconv.Atoi(strings.TrimSpace(lenLine))
		if err != nil {
			return "", err
		}
		buf := make([]byte, size+2)
		if _, err := r.Read(buf); err != nil {
			return "", err
		}
		parts = append(parts, string(buf[:size]))
	}
	if len(parts) < 3 || parts[0] != "message" {
		return "", fmt.Errorf("unexpected pubsub payload: %v", parts)
	}
	return parts[2], nil
}
