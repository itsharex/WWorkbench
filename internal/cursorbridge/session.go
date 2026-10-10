package cursorbridge

import (
	"context"
	"path/filepath"
	"sync"
)

// Session 产品侧 Cursor 驱动会话：Bridge + 工具回调 + Client。
type Session struct {
	mu      sync.Mutex
	dataDir string
	apiKey  string
	mgr     *Manager
	tools   *ToolServer
	client  *Client
	handler ToolHandler
}

// NewSession 创建会话（未启动）。
func NewSession(dataDir, apiKey string, handler ToolHandler) *Session {
	ws := filepath.Join(dataDir, "cursor-workspace")
	return &Session{
		dataDir: dataDir,
		apiKey:  apiKey,
		mgr:     NewManager(dataDir, apiKey, ws),
		handler: handler,
	}
}

// Workspace Cursor 本地 Agent 工作目录。
func (s *Session) Workspace() string {
	return filepath.Join(s.dataDir, "cursor-workspace")
}

// Ready 会话是否已热（Bridge + Client 可直接复用）。
func (s *Session) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil && s.mgr != nil && s.mgr.Alive()
}

// Ensure 先起工具回调，再启动 Bridge（带 launch flags），最后拿到 Client。
// 已 Ready 时直接返回缓存 Client，不再打进度、不再重复 SetToolCallback。
func (s *Session) Ensure(ctx context.Context, onProgress ProgressFn) (*Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mgr.SetAPIKey(s.apiKey)

	if s.client != nil && s.mgr.Alive() {
		return s.client, nil
	}

	if s.tools == nil && s.handler != nil {
		if onProgress != nil {
			onProgress("tools", "正在启动工作台工具回调…")
		}
		ts, err := StartToolServer(s.handler)
		if err != nil {
			return nil, err
		}
		s.tools = ts
		s.mgr.SetToolCallbackLaunch(ts.URL(), ts.AuthToken())
	}

	ep, err := s.mgr.Ensure(ctx, onProgress)
	if err != nil {
		return nil, err
	}
	cli := &Client{BaseURL: ep.URL, Token: ep.AuthToken}
	if s.tools != nil {
		_ = cli.SetToolCallback(ctx, s.tools.URL(), s.tools.AuthToken())
	}
	s.client = cli
	return s.client, nil
}

// SetAPIKey 更新密钥。
func (s *Session) SetAPIKey(key string) {
	s.mu.Lock()
	s.apiKey = key
	s.mgr.SetAPIKey(key)
	s.mu.Unlock()
}

// Close 关闭回调与 Bridge。
func (s *Session) Close(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mgr != nil {
		s.mgr.Stop(ctx)
	}
	if s.tools != nil {
		s.tools.Close()
		s.tools = nil
	}
	s.client = nil
}
