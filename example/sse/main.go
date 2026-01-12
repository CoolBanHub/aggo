package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/CoolBanHub/aggo/agent"
	"github.com/CoolBanHub/aggo/memory"
	"github.com/CoolBanHub/aggo/memory/storage"
	"github.com/CoolBanHub/aggo/model"
	"github.com/CoolBanHub/aggo/pkg/sse"
	"github.com/CoolBanHub/aggo/utils"
	model2 "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	schemaGrom "gorm.io/gorm/schema"
)

type ChatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"sessionId,omitempty"`
}

type ChatResponse struct {
	Content   string `json:"content"`
	SessionID string `json:"sessionId"`
	Done      bool   `json:"done"`
}

var globalBot *agent.Agent
var globalMemoryManager *memory.MemoryManager
var cm model2.ToolCallingChatModel

func main() {
	ctx := context.Background()

	if err := initializeBot(ctx); err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}
	defer globalMemoryManager.Close()

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/api/chat", chatHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func initializeBot(ctx context.Context) error {
	baseUrl := os.Getenv("BaseUrl")
	apiKey := os.Getenv("APIKey")
	if baseUrl == "" || apiKey == "" {
		return fmt.Errorf("BaseUrl and APIKey environment variables must be set")
	}
	var err error
	cm, err = model.NewChatModel(model.WithBaseUrl(baseUrl),
		model.WithAPIKey(apiKey),
		model.WithModel("gpt-5-nano"),
	)
	if err != nil {
		return fmt.Errorf("new chat model fail,err:%s", err)
	}

	gormSql, err := NewMysqlGrom("root:123456@tcp(127.0.0.1:3306)/aggo", logger.Silent)
	if err != nil {
		return fmt.Errorf("创建数据库连接失败: %v", err)
	}

	s, err := storage.NewGormStorage(gormSql)
	if err != nil {
		return fmt.Errorf("new sql store fail,err:%s", err)
	}

	globalMemoryManager, err = memory.NewMemoryManager(cm, s, &memory.MemoryConfig{
		EnableSessionSummary: false,
		EnableUserMemories:   false,
		MemoryLimit:          8,
		Retrieval:            memory.RetrievalLastN,
		AsyncProcessing:      true,
	})
	if err != nil {
		return fmt.Errorf("new manager fail,err:%s", err)
	}

	globalBot, err = agent.NewAgent(ctx, cm,
		agent.WithMemoryManager(globalMemoryManager),
		agent.WithSystemPrompt("你是一名28岁的厦门女孩,名叫mary,是一名短视频剪辑师，主要剪辑电影类。喜欢养狗，家里有一只金毛，它叫小黄。喜欢宅在家里刷剧。不喜欢吃水果。性格内向高冷，别人不问的时候，一般都不回答自己的信息"),
	)
	if err != nil {
		return fmt.Errorf("new agent fail,err:%s", err)
	}

	return nil
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AI聊天助手 - Mary</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
        }
        
        .chat-container {
            width: 90%;
            max-width: 800px;
            height: 90vh;
            background: white;
            border-radius: 20px;
            box-shadow: 0 20px 40px rgba(0,0,0,0.1);
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }
        
        .chat-header {
            background: #4f46e5;
            color: white;
            padding: 20px;
            text-align: center;
            font-size: 1.2em;
            font-weight: 600;
        }
        
        .chat-messages {
            flex: 1;
            padding: 20px;
            overflow-y: auto;
            display: flex;
            flex-direction: column;
            gap: 15px;
        }
        
        .message {
            max-width: 80%;
            padding: 12px 16px;
            border-radius: 18px;
            word-wrap: break-word;
            animation: fadeIn 0.3s ease-in;
        }
        
        .user-message {
            background: #4f46e5;
            color: white;
            align-self: flex-end;
            border-bottom-right-radius: 4px;
        }
        
        .bot-message {
            background: #f3f4f6;
            color: #374151;
            align-self: flex-start;
            border-bottom-left-radius: 4px;
        }
        
        .chat-input-container {
            padding: 20px;
            background: #f9fafb;
            border-top: 1px solid #e5e7eb;
        }
        
        .input-group {
            display: flex;
            gap: 10px;
            align-items: flex-end;
        }
        
        .chat-input {
            flex: 1;
            padding: 12px 16px;
            border: 2px solid #e5e7eb;
            border-radius: 20px;
            outline: none;
            resize: none;
            font-family: inherit;
            font-size: 16px;
            min-height: 48px;
            max-height: 120px;
        }
        
        .chat-input:focus {
            border-color: #4f46e5;
        }
        
        .send-button {
            background: #4f46e5;
            color: white;
            border: none;
            padding: 12px 20px;
            border-radius: 20px;
            cursor: pointer;
            font-size: 16px;
            font-weight: 600;
            transition: background-color 0.2s;
            min-width: 80px;
        }
        
        .send-button:hover {
            background: #4338ca;
        }
        
        .send-button:disabled {
            background: #9ca3af;
            cursor: not-allowed;
        }
        
        .typing-indicator {
            display: none;
            padding: 12px 16px;
            background: #f3f4f6;
            border-radius: 18px;
            align-self: flex-start;
            border-bottom-left-radius: 4px;
            max-width: 80%;
        }
        
        .typing-dots {
            display: flex;
            gap: 4px;
        }
        
        .typing-dot {
            width: 8px;
            height: 8px;
            background: #6b7280;
            border-radius: 50%;
            animation: typingAnimation 1.4s infinite;
        }
        
        .typing-dot:nth-child(2) {
            animation-delay: 0.2s;
        }
        
        .typing-dot:nth-child(3) {
            animation-delay: 0.4s;
        }
        
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
        }
        
        @keyframes typingAnimation {
            0%, 60%, 100% { transform: scale(1); opacity: 0.5; }
            30% { transform: scale(1.2); opacity: 1; }
        }
        
        .welcome-message {
            text-align: center;
            color: #6b7280;
            font-style: italic;
            margin: 20px 0;
        }
    </style>
</head>
<body>
    <div class="chat-container">
        <div class="chat-header">
            💬 AI聊天助手 - Mary
        </div>
        
        <div class="chat-messages" id="chatMessages">
            <div class="welcome-message">
                你好！我是Mary，一名来自厦门的短视频剪辑师。有什么想聊的吗？
            </div>
        </div>
        
        <div class="typing-indicator" id="typingIndicator">
            <div class="typing-dots">
                <div class="typing-dot"></div>
                <div class="typing-dot"></div>
                <div class="typing-dot"></div>
            </div>
        </div>
        
        <div class="chat-input-container">
            <div class="input-group">
                <textarea 
                    id="chatInput" 
                    class="chat-input" 
                    placeholder="输入你的消息..." 
                    rows="1"
                ></textarea>
                <button id="sendButton" class="send-button">发送</button>
            </div>
        </div>
    </div>

    <script>
        const chatMessages = document.getElementById('chatMessages');
        const chatInput = document.getElementById('chatInput');
        const sendButton = document.getElementById('sendButton');
        const typingIndicator = document.getElementById('typingIndicator');
        
        let sessionId = generateSessionId();
        
        function generateSessionId() {
            return 'session_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
        }
        
        function addMessage(content, isUser = false) {
            const messageDiv = document.createElement('div');
            messageDiv.className = 'message ' + (isUser ? 'user-message' : 'bot-message');
            messageDiv.textContent = content;
            chatMessages.appendChild(messageDiv);
            chatMessages.scrollTop = chatMessages.scrollHeight;
        }
        
        function showTyping() {
            typingIndicator.style.display = 'block';
            chatMessages.appendChild(typingIndicator);
            chatMessages.scrollTop = chatMessages.scrollHeight;
        }
        
        function hideTyping() {
            typingIndicator.style.display = 'none';
            if (typingIndicator.parentNode) {
                typingIndicator.parentNode.removeChild(typingIndicator);
            }
        }
        
        async function sendMessage() {
            const message = chatInput.value.trim();
            if (!message) return;
            
            addMessage(message, true);
            chatInput.value = '';
            sendButton.disabled = true;
            showTyping();
            
            try {
                const response = await fetch('/api/chat', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({
                        message: message,
                        sessionId: sessionId
                    })
                });
                
                if (!response.ok) {
                    throw new Error('Network response was not ok');
                }
                
                const reader = response.body.getReader();
                const decoder = new TextDecoder();
                let botMessage = '';
                let currentBotMessageDiv = null;
                
                hideTyping();
                
                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    
                    const chunk = decoder.decode(value);
                    const lines = chunk.split('\n');
                    
                    for (const line of lines) {
                        if (line.startsWith('data: ')) {
                            const data = line.slice(6);
                            if (data === '[DONE]') {
                                break;
                            }

                            try {
                                const parsed = JSON.parse(data);
                                // 支持OpenAI格式: choices[0].delta.content
                                const content = parsed.content || (parsed.choices && parsed.choices[0] && parsed.choices[0].delta && parsed.choices[0].delta.content);

                                if (content) {
                                    botMessage += content;

                                    if (!currentBotMessageDiv) {
                                        currentBotMessageDiv = document.createElement('div');
                                        currentBotMessageDiv.className = 'message bot-message';
                                        chatMessages.appendChild(currentBotMessageDiv);
                                    }

                                    currentBotMessageDiv.textContent = botMessage;
                                    chatMessages.scrollTop = chatMessages.scrollHeight;
                                }

                                // 检查是否完成
                                const finishReason = parsed.choices && parsed.choices[0] && parsed.choices[0].finish_reason;
                                if (parsed.done || finishReason) {
                                    sessionId = parsed.sessionId || sessionId;
                                }
                            } catch (e) {
                                console.error('Error parsing JSON:', e, 'Data:', data);
                            }
                        }
                    }
                }
                
            } catch (error) {
                hideTyping();
                addMessage('抱歉，发生了错误: ' + error.message, false);
            } finally {
                sendButton.disabled = false;
                chatInput.focus();
            }
        }
        
        sendButton.addEventListener('click', sendMessage);
        
        chatInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                sendMessage();
            }
        });
        
        chatInput.addEventListener('input', () => {
            chatInput.style.height = 'auto';
            chatInput.style.height = Math.min(chatInput.scrollHeight, 120) + 'px';
        });
        
        chatInput.focus();
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = utils.GetULID()
	}

	ctx := r.Context()
	input := []*schema.Message{
		schema.UserMessage(req.Message),
	}

	stream, err := cm.Stream(ctx, input)
	if err != nil {
		log.Printf("Error creating stream: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	writer := sse.NewWriter(sessionID, w)
	if writer == nil {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	defer writer.Close()

	err = writer.Stream(ctx, stream, func(output *schema.Message, index int) any {
		return model.OutStreamMessageEinoToOpenai(output, index)
	})
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Printf("Stream error: %v", err)
	}
}

func NewMysqlGrom(source string, logLevel logger.LogLevel) (*gorm.DB, error) {
	if !strings.Contains(source, "parseTime") {
		source += "?charset=utf8mb4&parseTime=True&loc=Local"
	}
	gdb, err := gorm.Open(mysql.Open(source), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		NamingStrategy: schemaGrom.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		panic("数据库连接失败: " + err.Error())
	}

	// 配置GORM日志
	var gormLogger logger.Interface
	if logLevel > 0 {
		gormLogger = logger.Default.LogMode(logLevel)
	} else {
		gormLogger = logger.Default.LogMode(logger.Silent)
	}

	gdb.Logger = gormLogger

	return gdb, nil
}
