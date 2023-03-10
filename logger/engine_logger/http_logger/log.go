package http_logger

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// Log structure passed through the log forwarding channel
type Log struct {
	context               *gin.Context
	startDate             time.Time
	latency               time.Duration
	requestBody           string
	responseHeaders       http.Header
	responseBody          string
	responseContentLength int64
}

// HTTPContent describes the format of a Request body and it's metadata
type HTTPContent struct {
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type,omitempty"`
	Content  string `json:"value,omitempty"`
}

// RequestLogEntry describes the incoming requests log format
type RequestLogEntry struct {
	Method      string            `json:"method"`
	URI         string            `json:"uri"`
	HTTPVersion string            `json:"http_version"`
	Headers     map[string]string `json:"headers"`
	HeaderSize  int               `json:"headers_size"`
	Content     HTTPContent       `json:"content"`
}

// ResponseLogEntry describes the server response log format
type ResponseLogEntry struct {
	Status     int               `json:"status,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	HeaderSize int               `json:"headers_size"`
	Content    HTTPContent       `json:"content"`
}

// AccessLog describes the complete log entry format
type AccessLog struct {
	TimeStarted   string           `json:"start_time"`
	ClientAddress string           `json:"x_client_address,omitempty"`
	Time          int64            `json:"duration"`
	Request       RequestLogEntry  `json:"request"`
	Response      ResponseLogEntry `json:"response"`
	Errors        string           `json:"errors,omitempty"`
}
