package resolver

import (
	"context"
	"github.com/go-slark/slark/errors"
	"github.com/go-slark/slark/pkg/subset"
	"github.com/go-slark/slark/registry"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
	"net/url"
	"time"
)

type parser struct {
	watcher registry.Watcher
	ctx     context.Context
	cancel  context.CancelFunc
	cc      resolver.ClientConn
	subset  int
}

func (p *parser) ResolveNow(opts resolver.ResolveNowOptions) {}

func (p *parser) Close() {
	p.cancel()
	_ = p.watcher.Stop()
}

func (p *parser) watch() {
	for {
		select {
		case <-p.ctx.Done():
			return

		default:
			svc, err := p.watcher.List()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				time.Sleep(time.Second)
				continue
			}
			p.update(svc)
		}
	}
}

func (p *parser) update(svc []*registry.Service) {
	mp := map[string]struct{}{}
	set := make([]*registry.Service, 0, len(svc))
	var ok bool
	// filter
	for _, s := range svc {
		u, err := url.Parse(s.Endpoint[0])
		if err != nil {
			continue
		}
		_, ok = mp[u.Host]
		if ok {
			continue
		}
		mp[u.Host] = struct{}{}
		set = append(set, s)
	}
	if p.subset > 0 {
		set = subset.Subset(set, p.subset)
	}
	addresses := make([]resolver.Address, 0, len(svc))
	for _, s := range set {
		u, _ := url.Parse(s.Endpoint[0])
		addr := resolver.Address{
			ServerName: s.Name,
			//BalancerAttributes 字段可以用来保存负载均衡策略所使用的信息，比如权重信息
			Attributes: attributes.New("attributes", s),
			Addr:       u.Host,
		}
		addresses = append(addresses, addr)
	}
	if len(addresses) == 0 {
		return
	}
	_ = p.cc.UpdateState(resolver.State{Addresses: addresses})
}
