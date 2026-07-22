package ai

import (
	"bufio"
	"os"
	"strings"
	"sync/atomic"
)

type KeyRotater struct {
	geminiKeys []string
	groqKeys   []string
	geminiIdx  uint64
	groqIdx    uint64
}

var GlobalRotater *KeyRotater

func InitRotater(paths ...string) {
	if len(paths) == 0 {
		paths = []string{
			"/root/hostcontrol/.env",
			".env",
			"/home/data/appck/.env",
			"/home/data/taissh/.env",
		}
	}

	r := &KeyRotater{
		geminiKeys: make([]string, 0),
		groqKeys:   make([]string, 0),
	}

	geminiSeen := make(map[string]bool)
	groqSeen := make(map[string]bool)

	for _, p := range paths {
		file, err := os.Open(p)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			if key == "GEMINI_KEYS" || key == "GEMINI_API_KEYS" {
				keys := strings.Split(val, ",")
				for _, k := range keys {
					kTrim := strings.TrimSpace(k)
					if kTrim != "" && !geminiSeen[kTrim] {
						geminiSeen[kTrim] = true
						r.geminiKeys = append(r.geminiKeys, kTrim)
					}
				}
			} else if key == "GROQ_KEYS" || key == "GROQ_API_KEYS" {
				keys := strings.Split(val, ",")
				for _, k := range keys {
					kTrim := strings.TrimSpace(k)
					if kTrim != "" && !groqSeen[kTrim] {
						groqSeen[kTrim] = true
						r.groqKeys = append(r.groqKeys, kTrim)
					}
				}
			}
		}
		file.Close()
	}

	GlobalRotater = r
}

func (r *KeyRotater) GetGeminiKey() string {
	if len(r.geminiKeys) == 0 {
		return ""
	}
	idx := atomic.AddUint64(&r.geminiIdx, 1)
	return r.geminiKeys[idx%uint64(len(r.geminiKeys))]
}

func (r *KeyRotater) GetGroqKey() string {
	if len(r.groqKeys) == 0 {
		return ""
	}
	idx := atomic.AddUint64(&r.groqIdx, 1)
	return r.groqKeys[idx%uint64(len(r.groqKeys))]
}

func (r *KeyRotater) GeminiKeysCount() int {
	return len(r.geminiKeys)
}

func (r *KeyRotater) GroqKeysCount() int {
	return len(r.groqKeys)
}
