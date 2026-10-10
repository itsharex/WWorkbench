package cursorbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ToolHandler 执行自定义工具；返回必须是 JSON object。
type ToolHandler func(ctx context.Context, toolName string, args json.RawMessage, toolCallID, agentID string) (map[string]interface{}, error)

// ToolServer 本机回调服务（SdkCustomToolCallbackService）。
type ToolServer struct {
	mu      sync.Mutex
	srv     *http.Server
	ln      net.Listener
	token   string
	url     string
	handler ToolHandler
}

// StartToolServer 监听 127.0.0.1:0。
func StartToolServer(handler ToolHandler) (*ToolServer, error) {
	if handler == nil {
		return nil, fmt.Errorf("tool handler required")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := uuid.NewString()
	ts := &ToolServer{
		ln:      ln,
		token:   token,
		url:     "http://" + ln.Addr().String(),
		handler: handler,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/sdk.v1.SdkCustomToolCallbackService/CallCustomTool", ts.handleCall)
	ts.srv = &http.Server{Handler: mux}
	go func() { _ = ts.srv.Serve(ln) }()
	return ts, nil
}

// URL 回调基址。
func (t *ToolServer) URL() string { return t.url }

// AuthToken Bridge 回调时使用的 Bearer。
func (t *ToolServer) AuthToken() string { return t.token }

// Close 关闭回调服务。
func (t *ToolServer) Close() {
	if t == nil || t.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = t.srv.Shutdown(ctx)
}

func (t *ToolServer) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth := r.Header.Get("Authorization")
	if !strings.EqualFold(auth, "Bearer "+t.token) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthenticated","message":"Unauthorized"}`))
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeConnectErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	var in struct {
		ToolName   string          `json:"toolName"`
		Args       json.RawMessage `json:"args"`
		ToolCallID string          `json:"toolCallId"`
		AgentID    string          `json:"agentId"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeConnectErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	args := in.Args
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	ctx := r.Context()
	res, err := t.handler(ctx, in.ToolName, args, in.ToolCallID, in.AgentID)
	if err != nil {
		res = map[string]interface{}{"ok": false, "error": err.Error()}
	}
	if res == nil {
		res = map[string]interface{}{"ok": true}
	}
	out, _ := json.Marshal(map[string]interface{}{"result": res})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func writeConnectErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	b, _ := json.Marshal(map[string]string{"code": "invalid_argument", "message": msg})
	_, _ = w.Write(b)
}
