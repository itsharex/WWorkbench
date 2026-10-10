package cursorbridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client Connect JSON 客户端（手写，对齐 smoke-test）。
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 0} // 流式无全局超时
}

func (c *Client) unary(ctx context.Context, service, method string, reqBody any, out any) error {
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/sdk.v1.%s/%s", strings.TrimRight(c.BaseURL, "/"), service, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Connect-Protocol-Version", "1")
	cli := c.http()
	if cli.Timeout == 0 {
		cli = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s/%s: HTTP %d %s", service, method, resp.StatusCode, truncate(string(raw), 400))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s/%s 解析失败: %w (%s)", service, method, err, truncate(string(raw), 200))
	}
	return nil
}

// StreamEnvelope Send/Observe 流上的一条消息（JSON）。
type StreamEnvelope struct {
	SdkMessage         json.RawMessage `json:"sdkMessage"`
	Result             json.RawMessage `json:"result"`
	Done               json.RawMessage `json:"done"`
	InteractionUpdate  json.RawMessage `json:"interactionUpdate"`
	Step               json.RawMessage `json:"step"`
	Offset             string          `json:"offset"`
}

func (c *Client) serverStream(ctx context.Context, service, method string, reqBody any, onEnv func(StreamEnvelope) error) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	var frame bytes.Buffer
	_ = frame.WriteByte(0)
	_ = binary.Write(&frame, binary.BigEndian, uint32(len(payload)))
	_, _ = frame.Write(payload)

	url := fmt.Sprintf("%s/sdk.v1.%s/%s", strings.TrimRight(c.BaseURL, "/"), service, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &frame)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Connect-Protocol-Version", "1")

	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s/%s: HTTP %d %s", service, method, resp.StatusCode, truncate(string(raw), 400))
	}
	for {
		hdr := make([]byte, 5)
		if _, err := io.ReadFull(resp.Body, hdr); err != nil {
			if err == io.EOF {
				return fmt.Errorf("流意外结束")
			}
			return err
		}
		flags := hdr[0]
		length := binary.BigEndian.Uint32(hdr[1:])
		body := make([]byte, length)
		if _, err := io.ReadFull(resp.Body, body); err != nil {
			return err
		}
		if flags&0x02 != 0 {
			if len(body) == 0 {
				return nil
			}
			var end struct {
				Error json.RawMessage `json:"error"`
			}
			_ = json.Unmarshal(body, &end)
			if len(end.Error) > 0 {
				return fmt.Errorf("stream error: %s", truncate(string(end.Error), 400))
			}
			return nil
		}
		var env StreamEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			continue
		}
		// keepalive / 未知：无 envelope
		if len(env.SdkMessage) == 0 && len(env.Result) == 0 && len(env.Done) == 0 &&
			len(env.InteractionUpdate) == 0 && len(env.Step) == 0 {
			continue
		}
		if onEnv != nil {
			if err := onEnv(env); err != nil {
				return err
			}
		}
		if len(env.Done) > 0 {
			return nil
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
