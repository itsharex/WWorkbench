package cursorbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CustomTool 注册给 Bridge 的自定义工具元数据。
type CustomTool struct {
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// CreateAgentOpts 创建本地 Agent。
type CreateAgentOpts struct {
	APIKey      string
	Model       string
	CWD         string
	Mode        string // agent | plan
	CustomTools map[string]CustomTool
}

// CreateAgent 创建本地 Cursor Agent。
func (c *Client) CreateAgent(ctx context.Context, opts CreateAgentOpts) (agentID string, err error) {
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = "composer-2.5"
	}
	local := map[string]interface{}{}
	if cwd := strings.TrimSpace(opts.CWD); cwd != "" {
		local["cwd"] = []string{cwd}
	}
	if len(opts.CustomTools) > 0 {
		tools := map[string]interface{}{}
		for name, t := range opts.CustomTools {
			tools[name] = map[string]interface{}{
				"description": t.Description,
				"inputSchema": t.InputSchema,
			}
		}
		local["customTools"] = tools
	}
	options := map[string]interface{}{
		"model":  map[string]interface{}{"id": model},
		"apiKey": opts.APIKey,
		"local":  local,
	}
	switch strings.ToLower(strings.TrimSpace(opts.Mode)) {
	case "plan":
		options["mode"] = 2 // AGENT_MODE_OPTION_PLAN
	case "agent":
		options["mode"] = 1 // AGENT_MODE_OPTION_AGENT
	}
	var out struct {
		AgentID string `json:"agentId"`
	}
	if err := c.unary(ctx, "SdkAgentService", "CreateAgent", map[string]interface{}{
		"options": options,
	}, &out); err != nil {
		return "", err
	}
	if out.AgentID == "" {
		return "", fmt.Errorf("CreateAgent 未返回 agentId")
	}
	return out.AgentID, nil
}

// ResumeAgent 恢复已有 Agent（可附带更新 custom tools）。
func (c *Client) ResumeAgent(ctx context.Context, agentID string, opts CreateAgentOpts) (string, error) {
	options := map[string]interface{}{
		"apiKey": opts.APIKey,
	}
	if strings.TrimSpace(opts.Model) != "" {
		options["model"] = map[string]interface{}{"id": opts.Model}
	}
	local := map[string]interface{}{}
	if cwd := strings.TrimSpace(opts.CWD); cwd != "" {
		local["cwd"] = []string{cwd}
	}
	if len(opts.CustomTools) > 0 {
		tools := map[string]interface{}{}
		for name, t := range opts.CustomTools {
			tools[name] = map[string]interface{}{
				"description": t.Description,
				"inputSchema": t.InputSchema,
			}
		}
		local["customTools"] = tools
	}
	if len(local) > 0 {
		options["local"] = local
	}
	var out struct {
		AgentID string `json:"agentId"`
	}
	if err := c.unary(ctx, "SdkAgentService", "ResumeAgent", map[string]interface{}{
		"agentId": agentID,
		"options": options,
	}, &out); err != nil {
		return "", err
	}
	if out.AgentID == "" {
		return agentID, nil
	}
	return out.AgentID, nil
}

// CancelRun 取消进行中的 run。
func (c *Client) CancelRun(ctx context.Context, runID, agentID string) error {
	body := map[string]interface{}{"runId": runID}
	if strings.TrimSpace(agentID) != "" {
		body["agentId"] = agentID
	}
	return c.unary(ctx, "SdkAgentService", "CancelRun", body, &struct{}{})
}

// CancelActiveRuns 列出并取消该 Agent 上仍在跑的 run（解决 already has active run）。
func (c *Client) CancelActiveRuns(ctx context.Context, agentID, apiKey string) (int, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return 0, nil
	}
	var out struct {
		Items []struct {
			RunID  string      `json:"runId"`
			Status interface{} `json:"status"`
		} `json:"items"`
	}
	opts := map[string]interface{}{"limit": 10}
	if strings.TrimSpace(apiKey) != "" {
		opts["apiKey"] = apiKey
	}
	if err := c.unary(ctx, "SdkAgentService", "ListRuns", map[string]interface{}{
		"agentId": agentID,
		"options": opts,
	}, &out); err != nil {
		return 0, err
	}
	n := 0
	for _, it := range out.Items {
		st := strings.ToLower(fmt.Sprint(it.Status))
		active := strings.Contains(st, "running") || strings.Contains(st, "creating") ||
			st == "1" || st == "2" // CREATING=1 RUNNING=2
		if !active || strings.TrimSpace(it.RunID) == "" {
			continue
		}
		if err := c.CancelRun(ctx, it.RunID, agentID); err == nil {
			n++
		}
	}
	return n, nil
}

// Ping 健康检查。
func (c *Client) Ping(ctx context.Context) error {
	var out struct {
		Message string `json:"message"`
	}
	if err := c.unary(ctx, "SdkBridgeControlService", "Ping", map[string]interface{}{}, &out); err != nil {
		return err
	}
	return nil
}

// Me 校验 API Key。
func (c *Client) Me(ctx context.Context, apiKey string) (string, error) {
	var out struct {
		User struct {
			APIKeyName string `json:"apiKeyName"`
			UserEmail  string `json:"userEmail"`
		} `json:"user"`
	}
	if err := c.unary(ctx, "SdkCursorService", "Me", map[string]interface{}{
		"options": map[string]interface{}{"apiKey": apiKey},
	}, &out); err != nil {
		return "", err
	}
	label := strings.TrimSpace(out.User.UserEmail)
	if label == "" {
		label = strings.TrimSpace(out.User.APIKeyName)
	}
	if label == "" {
		label = "ok"
	}
	return label, nil
}

// SetToolCallback 注册自定义工具回调。
func (c *Client) SetToolCallback(ctx context.Context, url, authToken string) error {
	return c.unary(ctx, "SdkBridgeControlService", "SetToolCallback", map[string]interface{}{
		"url":       url,
		"authToken": authToken,
	}, &struct{}{})
}

// Shutdown 请求 Bridge 退出。
func (c *Client) Shutdown(ctx context.Context) error {
	return c.unary(ctx, "SdkBridgeControlService", "Shutdown", map[string]interface{}{
		"graceSeconds": 1,
	}, &struct{}{})
}

// SendEvent 流事件（已归一）。
type SendEvent struct {
	Kind    string // assistant_delta | assistant | tool_call | status | result | other
	Text    string
	RawType string
	Payload map[string]interface{}
	Status  string
	OK      bool
}

// Send 发送用户消息并消费流；onEvent 可为空。
func (c *Client) Send(ctx context.Context, agentID, text string, onEvent func(SendEvent)) (finalText string, status string, err error) {
	var (
		assistant  strings.Builder
		lastStatus string
		runOK      = true
		// sdkMessage.assistant 每帧只带增量分片；interactionUpdate.text-delta 亦然。
		// 用 sawTextDelta 避免同一分片被 delta + assistant 各记一次。
		sawTextDelta bool
	)
	err = c.serverStream(ctx, "SdkAgentService", "Send", map[string]interface{}{
		"agentId": agentID,
		"message": map[string]interface{}{"text": text},
		"options": map[string]interface{}{"enableDeltas": true},
	}, func(env StreamEnvelope) error {
		if len(env.InteractionUpdate) > 0 {
			kind, delta := extractDeltaText(env.InteractionUpdate)
			if kind == "text-delta" && delta != "" {
				sawTextDelta = true
				assistant.WriteString(delta)
				if onEvent != nil {
					onEvent(SendEvent{Kind: "assistant_delta", Text: delta})
				}
			}
			return nil
		}
		if len(env.SdkMessage) > 0 {
			typ, payload := parseSdkMessage(env.SdkMessage)
			switch typ {
			case "assistant":
				t := extractAssistantText(payload)
				// 已有 text-delta 时 assistant 帧是重复分片，跳过；否则追加分片
				if t != "" && !sawTextDelta {
					assistant.WriteString(t)
					if onEvent != nil {
						onEvent(SendEvent{Kind: "assistant_delta", Text: t, RawType: typ, Payload: payload})
					}
				}
			case "status":
				msg := stringField(payload, "message")
				if msg == "" {
					msg = stringField(payload, "text")
				}
				if st := stringField(payload, "status"); st != "" {
					lastStatus = st
				} else {
					lastStatus = msg
				}
				if onEvent != nil {
					onEvent(SendEvent{Kind: "status", Text: msg, RawType: typ, Payload: payload})
				}
			case "tool_call", "tool-call":
				if onEvent != nil {
					onEvent(SendEvent{Kind: "tool_call", RawType: typ, Payload: payload})
				}
			default:
				// thinking 等忽略
				if onEvent != nil && typ != "thinking" {
					onEvent(SendEvent{Kind: "other", RawType: typ, Payload: payload})
				}
			}
			return nil
		}
		if len(env.Result) > 0 {
			var r struct {
				Status    interface{} `json:"status"`
				Result    string      `json:"result"`
				ErrorCode string      `json:"errorCode"`
			}
			_ = json.Unmarshal(env.Result, &r)
			st := strings.ToLower(fmt.Sprint(r.Status))
			if strings.Contains(st, "error") || st == "4" {
				runOK = false
			}
			// 终态全文优先：流式若丢片则用 result 补齐
			if r.Result != "" && (assistant.Len() == 0 || len(r.Result) > assistant.Len()) {
				if assistant.Len() == 0 && onEvent != nil {
					onEvent(SendEvent{Kind: "assistant_delta", Text: r.Result})
				}
				assistant.Reset()
				assistant.WriteString(r.Result)
			}
			status = st
			if onEvent != nil {
				onEvent(SendEvent{Kind: "result", Text: r.Result, Status: st, OK: runOK})
			}
			return nil
		}
		return nil
	})
	if err != nil {
		return assistant.String(), status, err
	}
	if !runOK {
		msg := lastStatus
		if msg == "" {
			msg = "Cursor Agent 运行失败"
		}
		return assistant.String(), status, fmt.Errorf("%s", msg)
	}
	return assistant.String(), status, nil
}

func parseSdkMessage(raw json.RawMessage) (string, map[string]interface{}) {
	var m struct {
		Type    string                 `json:"type"`
		Message map[string]interface{} `json:"message"`
	}
	_ = json.Unmarshal(raw, &m)
	if m.Message == nil {
		m.Message = map[string]interface{}{}
	}
	return strings.ToLower(strings.TrimSpace(m.Type)), m.Message
}

// extractDeltaText 解析 interactionUpdate；仅 text-delta 返回正文。
func extractDeltaText(raw json.RawMessage) (kind, text string) {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", ""
	}
	kind = strings.ToLower(stringField(m, "type"))
	if update, ok := m["update"].(map[string]interface{}); ok {
		if kind == "" {
			kind = strings.ToLower(stringField(update, "type"))
		}
		text = stringField(update, "text")
	}
	if text == "" {
		text = stringField(m, "text")
	}
	return kind, text
}

func extractAssistantText(payload map[string]interface{}) string {
	if t := stringField(payload, "text"); t != "" {
		return t
	}
	if content, ok := payload["content"].([]interface{}); ok {
		var b strings.Builder
		for _, c := range content {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if stringField(cm, "type") == "text" || stringField(cm, "type") == "" {
				b.WriteString(stringField(cm, "text"))
			}
		}
		return b.String()
	}
	if msg, ok := payload["message"].(map[string]interface{}); ok {
		return extractAssistantText(msg)
	}
	return ""
}

func stringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}
