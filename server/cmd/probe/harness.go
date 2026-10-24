package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// 7. The official harness. It starts `qloo mcp` pointed at QLOO_BASE_URL,
// speaks MCP over stdio (newline-delimited JSON-RPC), lists the tools and
// calls qloo_find_tags once.
func (p *prober) harness(ctx context.Context) (verdict, error) {
	if p.cfg.QlooBin == "" {
		return verdict{skip, "qloo CLI not on PATH (npm i -g @qloo/qloo-harness, Node ≥22.19), then rerun with -only 7"}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.cfg.QlooBin, "mcp")
	cmd.Env = append(os.Environ(), "QLOO_API_KEY="+p.cfg.APIKey, "QLOO_BASE_URL="+p.cfg.BaseURL)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return verdict{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return verdict{}, err
	}
	if err := cmd.Start(); err != nil {
		return verdict{}, err
	}
	defer func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()
	m := &mcpConn{in: stdin, out: bufio.NewScanner(stdout)}
	m.out.Buffer(make([]byte, 0, 64<<10), 8<<20)
	broke := func(step string, err error) (verdict, error) {
		return verdict{}, fmt.Errorf("qloo mcp %s: %w (stderr: %s)", step, err, truncate(strings.TrimSpace(stderr.String()), 300))
	}

	var initRes struct {
		ServerInfo struct{ Name, Version string } `json:"serverInfo"`
	}
	if err := m.call(1, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "blastingroom-probe", "version": "0"},
	}, &initRes); err != nil {
		return broke("initialize", err)
	}
	if err := m.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		return broke("initialized", err)
	}
	p.say("server %s %s, QLOO_BASE_URL=%s", initRes.ServerInfo.Name, initRes.ServerInfo.Version, p.cfg.BaseURL)

	var list struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := m.call(2, "tools/list", map[string]any{}, &list); err != nil {
		return broke("tools/list", err)
	}
	var toolNames []string
	arg := ""
	for _, t := range list.Tools {
		toolNames = append(toolNames, t.Name)
		if t.Name == "qloo_find_tags" {
			arg = queryArg(t.InputSchema.Properties, t.InputSchema.Required)
		}
	}
	sort.Strings(toolNames)
	p.say("tools: %s", strings.Join(toolNames, ", "))
	if arg == "" {
		return verdict{weak, fmt.Sprintf("server starts with %d tools, but no qloo_find_tags with a text argument to test a live call", len(toolNames))}, nil
	}

	var callRes struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := m.call(3, "tools/call", map[string]any{
		"name":      "qloo_find_tags",
		"arguments": map[string]any{arg: p.cfg.Genre},
	}, &callRes); err != nil {
		return broke("tools/call", err)
	}
	var text string
	for _, c := range callRes.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	// The harness wraps results in {status, summary, results, error}.
	var env struct {
		Status  string `json:"status"`
		Summary string `json:"summary"`
	}
	json.Unmarshal([]byte(text), &env)
	p.say("qloo_find_tags(%s=%q): status %q, %s", arg, p.cfg.Genre, env.Status, truncate(firstNonEmpty(env.Summary, text), 200))

	ok := !callRes.IsError && (env.Status == "" || env.Status == "ok" || env.Status == "success")
	if !ok {
		return verdict{fail, fmt.Sprintf("qloo_find_tags failed against %s: %s", p.cfg.BaseURL, truncate(text, 160))}, nil
	}
	return verdict{pass, fmt.Sprintf("qloo mcp works against %s (%d tools)", p.cfg.BaseURL, len(toolNames))}, nil
}

// queryArg picks the tool's free-text argument: "query" if present, else the
// first required string property.
func queryArg(props map[string]any, required []string) string {
	if _, ok := props["query"]; ok {
		return "query"
	}
	for _, r := range required {
		if pr, ok := props[r].(map[string]any); ok && pr["type"] == "string" {
			return r
		}
	}
	return ""
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

type mcpConn struct {
	in  io.Writer
	out *bufio.Scanner
}

func (m *mcpConn) send(msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = m.in.Write(append(b, '\n'))
	return err
}

// call sends a request and reads lines until the response with the same id,
// skipping notifications and anything that isn't JSON.
func (m *mcpConn) call(id int, method string, params, result any) error {
	if err := m.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for m.out.Scan() {
		var resp struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(m.out.Bytes(), &resp) != nil || resp.ID == nil || *resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return fmt.Errorf("JSON-RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return json.Unmarshal(resp.Result, result)
	}
	if err := m.out.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}
