package bridge

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type CallLogEntry struct {
	ID           uint64 `json:"id"`
	Time         string `json:"time"`
	RouteName    string `json:"routeName"`
	RoutePath    string `json:"routePath"`
	HTTPMethod   string `json:"httpMethod"`
	RPCMethod    string `json:"rpcMethod,omitempty"`
	Status       int    `json:"status"`
	DurationMs   int64  `json:"durationMs"`
	RequestBody  string `json:"requestBody,omitempty"`
	ResponseBody string `json:"responseBody,omitempty"`
}

type CallLogBroker struct {
	nextID atomic.Uint64

	mu          sync.RWMutex
	subscribers map[chan CallLogEntry]struct{}
}

func NewCallLogBroker() *CallLogBroker {
	return &CallLogBroker{
		subscribers: make(map[chan CallLogEntry]struct{}),
	}
}

func (b *CallLogBroker) HasSubscribers() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers) > 0
}

func (b *CallLogBroker) Subscribe() (<-chan CallLogEntry, func()) {
	ch := make(chan CallLogEntry, 128)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
	}

	return ch, unsubscribe
}

func (b *CallLogBroker) Publish(entry CallLogEntry) {
	b.mu.RLock()
	if len(b.subscribers) == 0 {
		b.mu.RUnlock()
		return
	}

	subscribers := make([]chan CallLogEntry, 0, len(b.subscribers))
	for ch := range b.subscribers {
		subscribers = append(subscribers, ch)
	}
	b.mu.RUnlock()

	entry.ID = b.nextID.Add(1)
	for _, ch := range subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
}

func captureRequestBody(req *http.Request) (string, string) {
	if req == nil || req.Body == nil {
		return "", ""
	}

	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))

	if err != nil {
		return fmt.Sprintf("<read error: %v>", err), ""
	}

	return normalizeLogText(body), extractRPCMethod(body)
}

func extractRPCMethod(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return ""
	}

	if body[0] == '[' {
		var batch []map[string]any
		if err := json.Unmarshal(body, &batch); err != nil || len(batch) == 0 {
			return ""
		}
		if method, ok := batch[0]["method"].(string); ok {
			return method
		}
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	method, _ := payload["method"].(string)
	return method
}

func normalizeLogText(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	return strings.ToValidUTF8(string(body), "")
}

type captureResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func newCaptureResponseWriter(w http.ResponseWriter) *captureResponseWriter {
	return &captureResponseWriter{ResponseWriter: w}
}

func (w *captureResponseWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *captureResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(status)
		return
	}

	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *captureResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	_, _ = w.body.Write(p)
	return w.ResponseWriter.Write(p)
}

func (w *captureResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *captureResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijack not supported")
	}
	return hijacker.Hijack()
}

func (w *captureResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (w *captureResponseWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *captureResponseWriter) BodyText() string {
	return normalizeLogText(w.body.Bytes())
}

func writeSSE(w http.ResponseWriter, payload []byte) error {
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	return nil
}

func writeSSEComment(w http.ResponseWriter, text string) error {
	if _, err := w.Write([]byte(": " + text + "\n\n")); err != nil {
		return err
	}
	return nil
}

func newCallLogEntry(routeName, routePath, httpMethod, rpcMethod, requestBody, responseBody string, status int, startedAt time.Time) CallLogEntry {
	return CallLogEntry{
		Time:         startedAt.UTC().Format(time.RFC3339Nano),
		RouteName:    routeName,
		RoutePath:    routePath,
		HTTPMethod:   httpMethod,
		RPCMethod:    rpcMethod,
		Status:       status,
		DurationMs:   time.Since(startedAt).Milliseconds(),
		RequestBody:  requestBody,
		ResponseBody: responseBody,
	}
}
