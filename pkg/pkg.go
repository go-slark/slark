package utils

import (
	"context"
	"encoding/json"
	"github.com/go-slark/slark/errors"
	"github.com/google/uuid"
	"net"
)

const (
	LogName       = "log-dumper"
	RayID         = "x-request-id"
	Authorization = "x-authorization"
	Token         = "x-token"
	Claims        = "x-jwt"
	UserAgent     = "User-Agent"
	Target      = "x-target"
	Method      = "x-method"
	RequestVars = "x-request-vars"

	ContentType = "Content-Type"
	Accept      = "Accept"
	Application = "application"

	Discovery = "discovery"
)

func BuildRequestID() string {
	return uuid.New().String()
}

type Config struct {
	Builder   func() string
	RequestID string
}

type Option func(*Config)

func WithBuilder(b func() string) Option {
	return func(cfg *Config) {
		cfg.Builder = b
	}
}

func WithRequestId(requestID string) Option {
	return func(cfg *Config) {
		cfg.RequestID = requestID
	}
}

func MustParseToken(ctx context.Context, v interface{}) {
	token, ok := ctx.Value(Token).(string)
	if !ok {
		panic(errors.TokenError)
	}
	err := json.Unmarshal([]byte(token), v)
	if err != nil {
		panic(err)
	}
}

func SnakeCase(s string) string {
	l := len(s)
	b := make([]byte, 0, l)
	for i := 0; i < l; i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			b = append(b, '_')
			c += 'a' - 'A'
		}
		b = append(b, c)
	}
	return string(b)
}
func FilterValidIP() ([]net.IP, error) {
	is, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	index := int(^uint(0) >> 1)
	ips := make([]net.IP, 0)
	for _, i := range is {
		if (i.Flags & net.FlagUp) == 0 {
			continue
		}
		if i.Index >= index && len(ips) != 0 {
			continue
		}

		addr, e := i.Addrs()
		if e != nil {
			continue
		}
		for _, a := range addr {
			var ip net.IP
			switch at := a.(type) {
			case *net.IPAddr:
				ip = at.IP
			case *net.IPNet:
				ip = at.IP
			default:
				continue
			}

			ipBytes := net.ParseIP(ip.String())
			if !ipBytes.IsGlobalUnicast() || ipBytes.IsInterfaceLocalMulticast() {
				continue
			}
			index = i.Index
			ips = append(ips, ip)
			if ip.To4() != nil {
				break
			}
		}
	}
	return ips, nil
}