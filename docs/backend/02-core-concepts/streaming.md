# SSD 流式传输

CopCon 使用 Server-Sent Events (SSE) 实现实时流式传输,让客户端能够实时接收 LLM 生成的内容。

## 工作流程

### 1. HTTP 请求建立 SSE 连接

客户端发起 POST 请求,服务端将响应头设置为 SSE 格式:

```go
// 设置响应头
c.Writer.Header().Set("Content-Type", "text/event-stream")
c.Writer.Header().Set("Cache-Control", "no-cache")
c.Writer.Header().Set("Connection", "keep-alive")
c.Writer.Header().Set("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲
```

### 2. 创建 ChatContext

每个对话会话都有一个 `ChatContext`,它负责:
- 管理对话状态
- 维护事件队列
- 协调 Agent 执行
- 推送 SSE 事件

```go
type ChatContext struct {
    sessionId string
    events    chan Event
    done      chan struct{}
}
```

### 3. 启动异步对话

对话在独立的 goroutine 中执行,通过 channel 发送事件:

```go
go func() {
    defer close(chatCtx.events)
    h.harness.Chat(chatCtx, req)
}()
```

### 4. 流式推送事件

服务端循环读取事件并推送给客户端:

```go
for event := range chatCtx.Events() {
    data, _ := json.Marshal(event.Data)
    c.SSEvent(event.Type, string(data))
    c.Writer.Flush()
}
```

## 事件类型

### message

LLM 生成的文本片段:

```go
type Message struct {
    Content  string `json:"content"`
    Role     string `json:"role"` // "assistant"
}
```

**客户端示例:**
```javascript
eventSource.addEventListener('message', (e) => {
    const data = JSON.parse(e.data);
    outputDiv.textContent += data.content;
});
```

### tool_call

Agent 调用工具:

```go
type ToolCall struct {
    ToolName string         `json:"tool_name"`
    Arguments map[string]any `json:"arguments"`
}
```

**客户端示例:**
```javascript
eventSource.addEventListener('tool_call', (e) => {
    const data = JSON.parse(e.data);
    console.log(`Calling tool: ${data.tool_name}`);
    console.log(`Arguments:`, data.arguments);
});
```

### tool_result

工具执行结果:

```go
type ToolResult struct {
    ToolName string      `json:"tool_name"`
    Result   any         `json:"result"`
    Error    string      `json:"error,omitempty"`
}
```

**客户端示例:**
```javascript
eventSource.addEventListener('tool_result', (e) => {
    const data = JSON.parse(e.data);
    if (data.error) {
        console.error(`Tool error: ${data.error}`);
    } else {
        console.log(`Tool result:`, data.result);
    }
});
```

### error

发生错误:

```go
type Error struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

**客户端示例:**
```javascript
eventSource.addEventListener('error', (e) => {
    const data = JSON.parse(e.data);
    alert(`Error: ${data.message}`);
});
```

### done

对话完成:

```go
// done 事件没有额外数据
```

**客户端示例:**
```javascript
eventSource.addEventListener('done', (e) => {
    console.log('Conversation complete');
    eventSource.close();
});
```

## 完整客户端示例

### JavaScript (浏览器)

```javascript
const sessionId = 'session-123';
const message = '你好,请帮我写一个Python程序';

// 发送 POST 请求并处理流式响应
fetch(`/api/sessions/${sessionId}/chat`, {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json',
    },
    body: JSON.stringify({
        content: message,
    }),
})
.then(response => {
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    
    function processStream() {
        reader.read().then(({ done, value }) => {
            if (done) {
                console.log('Stream complete');
                return;
            }
            
            const chunk = decoder.decode(value);
            const lines = chunk.split('\n');
            
            for (const line of lines) {
                if (line.startsWith('event:')) {
                    const eventType = line.substring(6);
                    console.log(`Event type: ${eventType}`);
                } else if (line.startsWith('data:')) {
                    const data = JSON.parse(line.substring(5));
                    
                    switch (eventType) {
                        case 'message':
                            outputDiv.textContent += data.content;
                            break;
                        case 'tool_call':
                            console.log(`Calling tool: ${data.tool_name}`);
                            break;
                        case 'tool_result':
                            console.log(`Tool result:`, data.result);
                            break;
                        case 'error':
                            alert(`Error: ${data.message}`);
                            break;
                        case 'done':
                            console.log('Complete');
                            break;
                    }
                }
            }
            
            processStream();
        });
    }
    
    processStream();
});
```

### Python

```python
import requests
import json

session_id = 'session-123'
message = '你好,请帮我写一个Python程序'

response = requests.post(
    f'http://localhost:8080/api/sessions/{session_id}/chat',
    json={'content': message},
    stream=True
)

event_type = None
for line in response.iter_lines():
    line = line.decode('utf-8')
    
    if line.startswith('event:'):
        event_type = line[6:]
        print(f'Event type: {event_type}')
    elif line.startswith('data:'):
        data = json.loads(line[5:])
        
        if event_type == 'message':
            print(data['content'], end='', flush=True)
        elif event_type == 'tool_call':
            print(f'\n[Calling tool: {data["tool_name"]}]')
        elif event_type == 'tool_result':
            print(f'\n[Tool result: {data["result"]}]')
        elif event_type == 'error':
            print(f'\n[Error: {data["message"]}]')
        elif event_type == 'done':
            print('\n[Complete]')
```

### Go (客户端)

```go
package main

import (
    "bufio"
    "fmt"
    "net/http"
    "strings"
)

func main() {
    sessionID := "session-123"
    message := "你好,请帮我写一个Python程序"
    
    resp, err := http.Post(
        fmt.Sprintf("http://localhost:8080/api/sessions/%s/chat", sessionID),
        "application/json",
        strings.NewReader(fmt.Sprintf(`{"content": %q}`, message)),
    )
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
    
    scanner := bufio.NewScanner(resp.Body)
    var eventType string
    
    for scanner.Scan() {
        line := scanner.Text()
        
        if strings.HasPrefix(line, "event:") {
            eventType = strings.TrimPrefix(line, "event:")
            fmt.Printf("Event type: %s\n", eventType)
        } else if strings.HasPrefix(line, "data:") {
            data := strings.TrimPrefix(line, "data:")
            
            switch eventType {
            case "message":
                fmt.Print(data)
            case "tool_call":
                fmt.Printf("\n[Calling tool: %s]\n", data)
            case "tool_result":
                fmt.Printf("\n[Tool result: %s]\n", data)
            case "error":
                fmt.Printf("\n[Error: %s]\n", data)
            case "done":
                println("\n[Complete]")
            }
        }
    }
}
```

### curl (命令行)

```bash
curl -N http://localhost:8080/api/sessions/session-123/chat \
  -H "Content-Type: application/json" \
  -d '{"content": "你好,请帮我写一个Python程序"}'
```

## 服务端实现

### Gin 框架

```go
func (h *Handler) Chat(c *gin.Context) {
    var req ChatRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid request"})
        return
    }
    
    // 设置 SSE 响应头
    c.Writer.Header().Set("Content-Type", "text/event-stream")
    c.Writer.Header().Set("Cache-Control", "no-cache")
    c.Writer.Header().Set("Connection", "keep-alive")
    c.Writer.Header().Set("X-Accel-Buffering", "no")
    
    // 创建对话上下文
    chatCtx := h.harness.NewChatContext(c.Request.Context(), sessionID)
    
    // 启动异步对话
    go func() {
        defer close(chatCtx.Events())
        h.harness.Chat(chatCtx, &core.Request{
            Content: req.Content,
        })
    }()
    
    // 流式推送事件
    for event := range chatCtx.Events() {
        data, _ := json.Marshal(event.Data)
        c.SSEvent(event.Type, string(data))
        c.Writer.Flush()
    }
}
```

## 重连与恢复

### 客户端重连

```javascript
const eventSource = new EventSource('/api/stream/session-123');

eventSource.onerror = (err) => {
    console.error('Connection error, reconnecting...');
    setTimeout(() => {
        // 重新建立连接
    }, 1000);
};
```

### 服务端会话恢复

```go
// 检查是否存在活跃的 ChatContext
if chatCtx, ok := h.activeSessions[sessionID]; ok {
    // 连接已有的 ChatContext
    for event := range chatCtx.Events() {
        c.SSEvent(event.Type, event.Data)
        c.Writer.Flush()
    }
}
```

## 性能优化

### 1. 禁用响应缓冲

```go
c.Writer.Header().Set("X-Accel-Buffering", "no") // Nginx
c.Writer.Header().Set("Cache-Control", "no-cache")
```

### 2. 批量发送事件

```go
// 累积多个小的 message 事件
var buffer strings.Builder
for i := 0; i < 10; i++ {
    buffer.WriteString(event.Content)
}
c.SSEvent("message", buffer.String())
c.Writer.Flush()
```

### 3. 使用 gzip 压缩

```go
c.Writer.Header().Set("Content-Encoding", "gzip")
gz := gzip.NewWriter(c.Writer)
defer gz.Close()
// 写入到 gz 而不是 c.Writer
```

## 常见问题

### Q: 为什么使用 SSE 而不是 WebSocket?

A: SSE 更简单,单向流式传输,不需要双向通信。适合 LLM 场景。

### Q: 如何处理网络断开?

A: 客户端自动重连,服务端支持会话恢复。

### Q: 如何限制并发连接?

A: 使用 `semaphore` 或 `rate limiter`。

### Q: 如何监控性能?

A: 使用 Prometheus 指标,详见 [监控指南](../07-deployment/monitoring.md)。

## 下一步

- [内置工具概览](../05-built-in-capabilities/tools/overview.md)
- [内置 Hooks 概览](../05-built-in-capabilities/hooks/overview.md)
- [HTTP API 参考](../08-reference/http-api.md)
- [SSE 事件参考](../08-reference/sse-events.md)
