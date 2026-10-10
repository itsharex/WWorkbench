package cursorbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const readyPrefix = "cursor-sdk-bridge ready "

// Endpoint Bridge 握手结果。
type Endpoint struct {
	URL           string
	AuthToken     string
	ServerVersion string
	PID           int
}

// Manager 管理一个 cursor-sdk-bridge 子进程。
type Manager struct {
	mu              sync.Mutex
	cmd             *exec.Cmd
	endpoint        *Endpoint
	binPath         string
	dataDir         string
	apiKey          string
	workspace       string
	toolCallbackURL string
	toolCallbackTok string
}

// NewManager 创建管理器（懒启动）。
func NewManager(dataDir, apiKey, workspace string) *Manager {
	return &Manager{dataDir: dataDir, apiKey: apiKey, workspace: workspace}
}

// SetAPIKey 更新密钥（下次 Ensure 生效；已运行则需 Restart）。
func (m *Manager) SetAPIKey(key string) {
	m.mu.Lock()
	m.apiKey = key
	m.mu.Unlock()
}

// SetWorkspace 更新工作目录。
func (m *Manager) SetWorkspace(ws string) {
	m.mu.Lock()
	m.workspace = ws
	m.mu.Unlock()
}

// SetToolCallbackLaunch 启动时注入自定义工具回调（CreateAgent 带 customTools 所必需）。
func (m *Manager) SetToolCallbackLaunch(url, authToken string) {
	m.mu.Lock()
	m.toolCallbackURL = strings.TrimSpace(url)
	m.toolCallbackTok = strings.TrimSpace(authToken)
	m.mu.Unlock()
}

// Alive Bridge 子进程是否已在跑（可热复用）。
func (m *Manager) Alive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.aliveLocked()
}

func (m *Manager) aliveLocked() bool {
	return m.endpoint != nil && m.cmd != nil && m.cmd.Process != nil && m.cmd.ProcessState == nil
}

// Ensure 确保 Bridge 已启动并返回 endpoint。
func (m *Manager) Ensure(ctx context.Context, onProgress ProgressFn) (*Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.aliveLocked() {
		return m.endpoint, nil
	}
	if onProgress != nil {
		onProgress("ensure", "正在准备 Cursor SDK Bridge…")
	}
	bin, err := EnsureBinary(m.dataDir, onProgress)
	if err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress("spawn", "正在启动 cursor-sdk-bridge…")
	}
	m.binPath = bin
	ws := m.workspace
	if ws == "" {
		ws = m.dataDir
	}
	if err := os.MkdirAll(ws, 0o700); err != nil {
		return nil, err
	}
	// Bridge 生命周期独立于单次对话 ctx，避免超时取消把进程杀掉。
	args := []string{"--workspace", ws}
	if m.toolCallbackURL != "" && m.toolCallbackTok != "" {
		args = append(args,
			"--tool-callback-url", m.toolCallbackURL,
			"--tool-callback-auth-token", m.toolCallbackTok,
		)
	}
	cmd := exec.Command(bin, args...)
	env := os.Environ()
	env = append(env, "CURSOR_SDK_CLIENT_LANGUAGE=go")
	if k := strings.TrimSpace(m.apiKey); k != "" {
		env = append(env, "CURSOR_API_KEY="+k)
	}
	cmd.Env = env
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 cursor-sdk-bridge 失败: %w", err)
	}
	m.cmd = cmd

	type result struct {
		ep  *Endpoint
		err error
	}
	ch := make(chan result, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 1024*1024)
		var diag strings.Builder
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, readyPrefix) {
				raw := strings.TrimPrefix(line, readyPrefix)
				ep, err := parseReady(raw)
				ch <- result{ep: ep, err: err}
				for sc.Scan() {
				}
				return
			}
			if diag.Len() < 4000 {
				diag.WriteString(line)
				diag.WriteByte('\n')
			}
		}
		waitErr := cmd.Wait()
		msg := strings.TrimSpace(diag.String())
		if msg == "" {
			msg = "bridge 退出且未输出 ready 行"
		}
		if waitErr != nil {
			ch <- result{err: fmt.Errorf("%s (%v)", msg, waitErr)}
			return
		}
		ch <- result{err: fmt.Errorf("%s", msg)}
	}()

	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		m.cmd = nil
		return nil, ctx.Err()
	case <-time.After(45 * time.Second):
		_ = cmd.Process.Kill()
		m.cmd = nil
		return nil, fmt.Errorf("cursor-sdk-bridge 启动超时（45s）")
	case r := <-ch:
		if r.err != nil {
			_ = cmd.Process.Kill()
			m.cmd = nil
			return nil, r.err
		}
		m.endpoint = r.ep
		return r.ep, nil
	}
}

func parseReady(raw string) (*Endpoint, error) {
	var d struct {
		SchemaVersion int    `json:"schemaVersion"`
		ServerVersion string `json:"serverVersion"`
		PID           int    `json:"pid"`
		Transport     string `json:"transport"`
		Protocol      string `json:"protocol"`
		URL           string `json:"url"`
		AuthTokenFile string `json:"authTokenFile"`
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, fmt.Errorf("解析 ready 行失败: %w", err)
	}
	if d.SchemaVersion != 1 || d.Transport != "tcp" || d.Protocol != "connect" {
		return nil, fmt.Errorf("不支持的 bridge discovery: schema=%d transport=%s protocol=%s",
			d.SchemaVersion, d.Transport, d.Protocol)
	}
	tok, err := os.ReadFile(d.AuthTokenFile)
	if err != nil {
		return nil, fmt.Errorf("读 auth token 失败: %w", err)
	}
	token := strings.TrimSpace(string(tok))
	if token == "" {
		return nil, fmt.Errorf("auth token 为空")
	}
	return &Endpoint{
		URL:           strings.TrimRight(d.URL, "/"),
		AuthToken:     token,
		ServerVersion: d.ServerVersion,
		PID:           d.PID,
	}, nil
}

// Stop 优雅关闭 Bridge。
func (m *Manager) Stop(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.endpoint != nil {
		cli := &Client{BaseURL: m.endpoint.URL, Token: m.endpoint.AuthToken}
		_ = cli.Shutdown(ctx)
	}
	if m.cmd != nil && m.cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = m.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = m.cmd.Process.Kill()
		}
	}
	m.cmd = nil
	m.endpoint = nil
}
