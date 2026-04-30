package langfuse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ClientConfig struct {
	Host              string
	PublicKey         string
	SecretKey         string
	HTTPClient        *http.Client
	FlushAt           int
	FlushInterval     time.Duration
	MaxQueueSize      int
	RequestTimeout    time.Duration
	Environment       string
	Release           string
	Version           string
	LogIngestErrors   bool
	DropOnQueueFull   bool
	IngestionMetadata any
}

type Client struct {
	cfg      ClientConfig
	endpoint string
	http     *http.Client

	mu     sync.Mutex
	queue  []ingestionEvent
	closed bool

	wake chan struct{}
	done chan struct{}
}

func NewClient(cfg ClientConfig) (*Client, error) {
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return nil, errors.New("langfuse host is required")
	}
	publicKey := strings.TrimSpace(cfg.PublicKey)
	secretKey := strings.TrimSpace(cfg.SecretKey)
	if publicKey == "" || secretKey == "" {
		return nil, errors.New("langfuse public key and secret key are required")
	}

	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse langfuse host: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/public/ingestion"
	u.RawQuery = ""
	u.Fragment = ""

	if cfg.FlushAt <= 0 {
		cfg.FlushAt = 15
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 500 * time.Millisecond
	}
	if cfg.MaxQueueSize <= 0 {
		cfg.MaxQueueSize = 1000
	}
	if strings.TrimSpace(cfg.Environment) == "" {
		cfg.Environment = defaultEnvironment
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.RequestTimeout}
	}
	if httpClient.Timeout == 0 && cfg.RequestTimeout > 0 {
		copyClient := *httpClient
		copyClient.Timeout = cfg.RequestTimeout
		httpClient = &copyClient
	}

	c := &Client{
		cfg:      cfg,
		endpoint: u.String(),
		http:     httpClient,
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	go c.loop()
	return c, nil
}

func (c *Client) Enqueue(eventType string, body any) {
	if c == nil {
		return
	}
	ev := ingestionEvent{
		ID:        uuid.NewString(),
		Timestamp: formatTime(time.Now()),
		Type:      eventType,
		Body:      body,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if len(c.queue) >= c.cfg.MaxQueueSize {
		if c.cfg.DropOnQueueFull {
			return
		}
		c.queue = c.queue[1:]
	}
	c.queue = append(c.queue, ev)
	if len(c.queue) >= c.cfg.FlushAt {
		c.signal()
	}
}

func (c *Client) Flush() {
	if c == nil {
		return
	}
	for {
		batch := c.takeBatch(0)
		if len(batch) == 0 {
			return
		}
		c.send(context.Background(), batch)
	}
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.done)
	}
	c.mu.Unlock()
	c.Flush()
}

func (c *Client) loop() {
	ticker := time.NewTicker(c.cfg.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.flushOne()
		case <-c.wake:
			c.flushOne()
		case <-c.done:
			return
		}
	}
}

func (c *Client) flushOne() {
	batch := c.takeBatch(c.cfg.FlushAt)
	if len(batch) == 0 {
		return
	}
	c.send(context.Background(), batch)
}

func (c *Client) takeBatch(limit int) []ingestionEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.queue) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(c.queue) {
		limit = len(c.queue)
	}
	batch := append([]ingestionEvent(nil), c.queue[:limit]...)
	copy(c.queue, c.queue[limit:])
	c.queue = c.queue[:len(c.queue)-limit]
	return batch
}

func (c *Client) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Client) send(ctx context.Context, batch []ingestionEvent) {
	if len(batch) == 0 {
		return
	}

	payload, err := json.Marshal(ingestionRequest{
		Batch:    batch,
		Metadata: c.cfg.IngestionMetadata,
	})
	if err != nil {
		if c.cfg.LogIngestErrors {
			log.Printf("langfuse marshal ingestion batch: %v", err)
		}
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		if c.cfg.LogIngestErrors {
			log.Printf("langfuse create ingestion request: %v", err)
		}
		return
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Basic "+basicAuth(c.cfg.PublicKey, c.cfg.SecretKey))

	resp, err := c.http.Do(req)
	if err != nil {
		if c.cfg.LogIngestErrors {
			log.Printf("langfuse send ingestion batch: %v", err)
		}
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if c.cfg.LogIngestErrors {
			log.Printf("langfuse ingestion status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return
	}

	var parsed ingestionResponse
	if len(body) > 0 && json.Unmarshal(body, &parsed) == nil && len(parsed.Errors) > 0 && c.cfg.LogIngestErrors {
		log.Printf("langfuse ingestion partial errors: %+v", parsed.Errors)
	}
}

func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}
