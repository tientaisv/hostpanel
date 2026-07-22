package ws

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Conn struct {
	net.Conn
	mu sync.Mutex
}

func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("not a websocket upgrade request")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}

	h := sha1.New()
	h.Write([]byte(key + magicGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("webserver doesn't support hijacking")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	
	bufrw.WriteString(resp)
	bufrw.Flush()

	return &Conn{Conn: conn}, nil
}

func (c *Conn) WriteText(text string) error {
	return c.WriteFrame(0x1, []byte(text))
}

func (c *Conn) WriteBinary(data []byte) error {
	return c.WriteFrame(0x2, []byte(data))
}

func (c *Conn) WriteFrame(opcode byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var header []byte
	length := len(payload)

	firstByte := byte(0x80) | (opcode & 0x0f) // FIN bit + Opcode
	header = append(header, firstByte)

	if length <= 125 {
		header = append(header, byte(length))
	} else if length <= 65535 {
		header = append(header, 126)
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(length))
		header = append(header, b...)
	} else {
		header = append(header, 127)
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(length))
		header = append(header, b...)
	}

	if _, err := c.Conn.Write(header); err != nil {
		return err
	}
	if length > 0 {
		_, err := c.Conn.Write(payload)
		return err
	}
	return nil
}

func (c *Conn) ReadMessage() (int, []byte, error) {
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(c.Conn, header); err != nil {
			return 0, nil, err
		}

		opcode := int(header[0] & 0x0f)
		masked := (header[1] & 0x80) != 0
		payloadLen := uint64(header[1] & 0x7f)

		if payloadLen == 126 {
			var l uint16
			if err := binary.Read(c.Conn, binary.BigEndian, &l); err != nil {
				return 0, nil, err
			}
			payloadLen = uint64(l)
		} else if payloadLen == 127 {
			var l uint64
			if err := binary.Read(c.Conn, binary.BigEndian, &l); err != nil {
				return 0, nil, err
			}
			payloadLen = l
		}

		var maskKey [4]byte
		if masked {
			if _, err := io.ReadFull(c.Conn, maskKey[:]); err != nil {
				return 0, nil, err
			}
		}

		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			if _, err := io.ReadFull(c.Conn, payload); err != nil {
				return 0, nil, err
			}
			if masked {
				for i := uint64(0); i < payloadLen; i++ {
					payload[i] ^= maskKey[i%4]
				}
			}
		}

		// Ping -> send Pong
		if opcode == 0x9 {
			_ = c.WriteFrame(0xA, payload)
			continue
		}
		// Connection Close
		if opcode == 0x8 {
			_ = c.Close()
			return opcode, nil, io.EOF
		}

		return opcode, payload, nil
	}
}
