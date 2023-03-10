package http_logger

import "github.com/gin-gonic/gin"

// LeechedGinResponseWriter is an extension of gin.ResponseWriter that logs the first bytes of the
// response body in a bytes buffer
type LeechedGinResponseWriter struct {
	gin.ResponseWriter

	data             []byte
	maxBodyLogSize   int64
	loggedBytesCount int64
}

// NewLeechedGinResponseWriter builds an returns a LeechedGinResponseWriter
func NewLeechedGinResponseWriter(source gin.ResponseWriter, maxSize int64) (newWriter *LeechedGinResponseWriter) {
	return &LeechedGinResponseWriter{
		data:           make([]byte, 0, maxSize),
		maxBodyLogSize: maxSize,
		ResponseWriter: source,
	}
}

// Write stores up to maxSize bites that go through the original writer while writing them on the
// orginal writer
func (l *LeechedGinResponseWriter) Write(b []byte) (int, error) {
	spaceLeft := l.maxBodyLogSize - l.loggedBytesCount
	if spaceLeft > 0 {
		l.data = append(l.data, b[:min(spaceLeft, int64(len(b)))]...)
		l.loggedBytesCount += int64(len(b))
	}

	return l.ResponseWriter.Write(b)
}
