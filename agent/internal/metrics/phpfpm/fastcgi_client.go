package phpfpm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const (
	fcgiVersion1          = 1
	fcgiBeginRequest      = 1
	fcgiEndRequest        = 3
	fcgiParams            = 4
	fcgiStdin             = 5
	fcgiStdout            = 6
	fcgiStderr            = 7
	fcgiResponder         = 1
	fcgiRequestID    uint = 1
)

type fastCGIClient struct {
	network     string
	address     string
	statusPath  string
	queryString string
	dialTimeout time.Duration
}

func newDefaultFastCGIClient() *fastCGIClient {
	return &fastCGIClient{
		network:     "tcp",
		address:     "127.0.0.1:9000",
		statusPath:  "/status",
		queryString: "json",
		dialTimeout: 2 * time.Second,
	}
}

func (c *fastCGIClient) GetStats() (*FPMStatus, error) {
	conn, err := net.DialTimeout(c.network, c.address, c.dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to php-fpm via fastcgi: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(c.dialTimeout)); err != nil {
		return nil, fmt.Errorf("failed to set fastcgi deadline: %w", err)
	}

	if err := c.writeRequest(conn); err != nil {
		return nil, err
	}

	stdout, stderr, err := c.readResponse(conn)
	if err != nil {
		return nil, err
	}

	body, statusCode, err := parseFastCGIHTTPResponse(stdout)
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("php-fpm status request returned HTTP %d: %s", statusCode, strings.TrimSpace(stderr))
	}

	var stats FPMStatus
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("failed to decode php-fpm status json: %w", err)
	}

	return &stats, nil
}

func (c *fastCGIClient) writeRequest(w io.Writer) error {
	beginRequestBody := []byte{0, fcgiResponder, 0, 0, 0, 0, 0, 0}
	if err := writeFastCGIRecord(w, fcgiBeginRequest, fcgiRequestID, beginRequestBody); err != nil {
		return fmt.Errorf("failed to write fastcgi begin request: %w", err)
	}

	params := c.requestParams()
	if len(params) > 0 {
		if err := writeFastCGIRecord(w, fcgiParams, fcgiRequestID, params); err != nil {
			return fmt.Errorf("failed to write fastcgi params: %w", err)
		}
	}
	if err := writeFastCGIRecord(w, fcgiParams, fcgiRequestID, nil); err != nil {
		return fmt.Errorf("failed to terminate fastcgi params: %w", err)
	}
	if err := writeFastCGIRecord(w, fcgiStdin, fcgiRequestID, nil); err != nil {
		return fmt.Errorf("failed to terminate fastcgi stdin: %w", err)
	}

	return nil
}

func (c *fastCGIClient) requestParams() []byte {
	requestURI := c.statusPath
	if c.queryString != "" {
		requestURI += "?" + c.queryString
	}

	serverPort := "0"
	if c.network == "tcp" {
		if _, port, err := net.SplitHostPort(c.address); err == nil {
			serverPort = port
		}
	}

	params := [][2]string{
		{"GATEWAY_INTERFACE", "CGI/1.1"},
		{"QUERY_STRING", c.queryString},
		{"REQUEST_METHOD", "GET"},
		{"REQUEST_URI", requestURI},
		{"SCRIPT_FILENAME", c.statusPath},
		{"SCRIPT_NAME", c.statusPath},
		{"DOCUMENT_URI", c.statusPath},
		{"SERVER_PROTOCOL", "HTTP/1.1"},
		{"SERVER_SOFTWARE", "simob-agent"},
		{"REMOTE_ADDR", "127.0.0.1"},
		{"REMOTE_PORT", "0"},
		{"SERVER_ADDR", "127.0.0.1"},
		{"SERVER_NAME", "localhost"},
		{"SERVER_PORT", serverPort},
	}

	var encoded bytes.Buffer
	for _, pair := range params {
		encoded.Write(encodeNameValuePair(pair[0], pair[1]))
	}

	return encoded.Bytes()
}

func encodeNameValuePair(name, value string) []byte {
	var out bytes.Buffer
	writeFastCGILength(&out, len(name))
	writeFastCGILength(&out, len(value))
	out.WriteString(name)
	out.WriteString(value)
	return out.Bytes()
}

func writeFastCGILength(w io.Writer, length int) {
	if length < 128 {
		_, _ = w.Write([]byte{byte(length)})
		return
	}

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(length)|0x80000000)
	_, _ = w.Write(buf)
}

func writeFastCGIRecord(w io.Writer, recordType uint8, requestID uint, content []byte) error {
	contentLength := len(content)
	if contentLength > math.MaxUint16 {
		return fmt.Errorf("fastcgi content too large: %d", contentLength)
	}

	paddingLength := byte((8 - (contentLength % 8)) % 8)
	header := []byte{
		fcgiVersion1,
		recordType,
		byte(requestID >> 8),
		byte(requestID),
		byte(contentLength >> 8),
		byte(contentLength),
		paddingLength,
		0,
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	if contentLength > 0 {
		if _, err := w.Write(content); err != nil {
			return err
		}
	}
	if paddingLength > 0 {
		if _, err := w.Write(make([]byte, paddingLength)); err != nil {
			return err
		}
	}

	return nil
}

func (c *fastCGIClient) readResponse(r io.Reader) ([]byte, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	for {
		recordType, content, err := readFastCGIRecord(r)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read fastcgi response: %w", err)
		}

		switch recordType {
		case fcgiStdout:
			stdout.Write(content)
		case fcgiStderr:
			stderr.Write(content)
		case fcgiEndRequest:
			if stdout.Len() == 0 && stderr.Len() > 0 {
				return nil, stderr.String(), fmt.Errorf("php-fpm returned fastcgi stderr: %s", strings.TrimSpace(stderr.String()))
			}
			return stdout.Bytes(), stderr.String(), nil
		}
	}
}

func readFastCGIRecord(r io.Reader) (uint8, []byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	if header[0] != fcgiVersion1 {
		return 0, nil, fmt.Errorf("unsupported fastcgi version %d", header[0])
	}

	contentLength := int(binary.BigEndian.Uint16(header[4:6]))
	paddingLength := int(header[6])

	content := make([]byte, contentLength)
	if _, err := io.ReadFull(r, content); err != nil {
		return 0, nil, err
	}

	if paddingLength > 0 {
		if _, err := io.CopyN(io.Discard, r, int64(paddingLength)); err != nil {
			return 0, nil, err
		}
	}

	return header[1], content, nil
}

func parseFastCGIHTTPResponse(raw []byte) ([]byte, int, error) {
	headerEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	separatorLen := 4
	if headerEnd == -1 {
		headerEnd = bytes.Index(raw, []byte("\n\n"))
		separatorLen = 2
	}
	if headerEnd == -1 {
		return nil, 0, fmt.Errorf("invalid fastcgi response: missing headers")
	}

	headerReader := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw[:headerEnd+separatorLen])))
	mimeHeader, err := headerReader.ReadMIMEHeader()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse fastcgi response headers: %w", err)
	}

	statusCode := 200
	if status := mimeHeader.Get("Status"); status != "" {
		fields := strings.Fields(status)
		if len(fields) > 0 {
			parsed, err := strconv.Atoi(fields[0])
			if err != nil {
				return nil, 0, fmt.Errorf("invalid fastcgi status header %q: %w", status, err)
			}
			statusCode = parsed
		}
	}

	return raw[headerEnd+separatorLen:], statusCode, nil
}
