package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"time"
)

type GeminiContent struct {
	Parts []map[string]string `json:"parts"`
}

type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

type GroqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GroqRequest struct {
	Model    string        `json:"model"`
	Messages []GroqMessage `json:"messages"`
}

type GroqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type CommandResult struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

func ExecuteBashCommand(cmdStr string) (*CommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	err := cmd.Run()
	duration := time.Since(start).String()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return &CommandResult{
		Command:  cmdStr,
		Output:   outBuf.String(),
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

func QueryAI(prompt string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	geminiCount := GlobalRotater.GeminiKeysCount()
	groqCount := GlobalRotater.GroqKeysCount()

	geminiModels := []string{"gemini-flash-latest", "gemini-2.0-flash", "gemini-1.5-flash-latest"}

	// Step 1: Try Gemini Keys
	if geminiCount > 0 {
		for i := 0; i < geminiCount; i++ {
			apiKey := GlobalRotater.GetGeminiKey()
			if apiKey == "" {
				continue
			}

			reqObj := GeminiRequest{
				Contents: []GeminiContent{
					{
						Parts: []map[string]string{
							{"text": prompt},
						},
					},
				},
			}
			bodyBytes, _ := json.Marshal(reqObj)

			for _, model := range geminiModels {
				url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
				req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
				if err != nil {
					continue
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if err == nil {
					respBody, _ := ioutil.ReadAll(resp.Body)
					resp.Body.Close()

					if resp.StatusCode == 200 {
						var gemResp GeminiResponse
						if errJson := json.Unmarshal(respBody, &gemResp); errJson == nil {
							if len(gemResp.Candidates) > 0 && len(gemResp.Candidates[0].Content.Parts) > 0 {
								return gemResp.Candidates[0].Content.Parts[0].Text, nil
							}
						}
					}
				}
			}
		}
	}

	// Step 2: Fallback to Groq Keys
	if groqCount > 0 {
		for i := 0; i < groqCount; i++ {
			groqKey := GlobalRotater.GetGroqKey()
			if groqKey == "" {
				continue
			}

			reqObj := GroqRequest{
				Model: "llama-3.3-70b-versatile",
				Messages: []GroqMessage{
					{Role: "system", Content: "Bạn là Chuyên gia DevOps & Linux Administrator hàng đầu. Khi đề xuất câu lệnh thực thi, hãy luôn đặt trong khối code ```bash ... ``` để hệ thống hỗ trợ 1-Click Run Command."},
					{Role: "user", Content: prompt},
				},
			}
			bodyBytes, _ := json.Marshal(reqObj)

			req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions", bytes.NewBuffer(bodyBytes))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+groqKey)

			resp, err := client.Do(req)
			if err == nil {
				respBody, _ := ioutil.ReadAll(resp.Body)
				resp.Body.Close()

				if resp.StatusCode == 200 {
					var groqResp GroqResponse
					if errJson := json.Unmarshal(respBody, &groqResp); errJson == nil {
						if len(groqResp.Choices) > 0 {
							return groqResp.Choices[0].Message.Content, nil
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("không thể gọi AI (đã thử %d Gemini keys & %d Groq keys nhưng bị lỗi rate limit)", geminiCount, groqCount)
}

func DiagnoseContainer(containerInfo string, logs string) (string, error) {
	prompt := fmt.Sprintf("Bạn là Chuyên gia DevOps & Linux System Administrator. Hãy phân tích sự cố cho Docker Container dưới đây:\n\n--- THÔNG TIN CONTAINER & CHỈ SỐ LIVE ---\n%s\n\n--- LOGS NỘI DUNG LỖI GẦN ĐÂY ---\n%s\n\n--- YÊU CẦU TRẢ LỜI (Định dạng Markdown tiếng Việt) ---\n1. 🔍 **Nguyên nhân sự cố (Root Cause Analysis)**: Chỉ ra chính xác lý do container lỗi/crash.\n2. 💡 **Các bước khắc phục chi tiết**: Hướng dẫn giải quyết.\n3. ⚡ **Lệnh đề xuất**: Luôn đặt các câu lệnh Linux/Docker trong khối ```bash ... ``` để kích hoạt nút 1-Click Run.", containerInfo, logs)

	return QueryAI(prompt)
}

func AuditSystemHealth(systemInfo string) (string, error) {
	prompt := fmt.Sprintf("Bạn là Chuyên gia Khắc phục Sự cố Linux Server & Docker Senior. Hãy đánh giá toàn bộ sức khỏe hệ thống:\n\n--- DỮ LIỆU THỰC TẾ LIVE HOST SERVER & CONTAINERS ---\n%s\n\n--- YÊU CẦU BÁO CÁO (Định dạng Markdown tiếng Việt) ---\n1. 📊 **Đánh giá sức khỏe hiện tại**: Phân tích CPU, RAM, Swap, Disk, Network I/O.\n2. ⚠️ **Cảnh báo rủi ro & Sự cố phát hiện**: Các container dừng, tiến trình treo.\n3. 🚀 **Hành động đề xuất**: Đặt các câu lệnh Linux/Docker trong khối ```bash ... ``` để người dùng có thể bấm 1-Click Run.", systemInfo)

	return QueryAI(prompt)
}
