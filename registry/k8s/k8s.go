package k8s

import (
	"context"
	"fmt"
	"github.com/go-slark/slark/errors"
	"github.com/go-slark/slark/registry"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"strconv"
	"strings"
	"time"
)

type Registry struct {
	clientSet *kubernetes.Clientset
	interval  time.Duration
	token     string
}

type Option func(*Registry)

func Interval(interval time.Duration) Option {
	return func(r *Registry) {
		r.interval = interval
	}
}

func Token(token string) Option {
	return func(r *Registry) {
		r.token = token
	}
}

func NewRegistry(opts ...Option) *Registry {
	r := &Registry{
		interval: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(r)
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}
	config.TLSClientConfig = rest.TLSClientConfig{Insecure: true}
	config.BearerToken = r.token
	config.BearerTokenFile = ""
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	r.clientSet = cs
	return r
}

func (r *Registry) Register(ctx context.Context, svc *registry.Service) error {
	return nil
}

func (r *Registry) Unregister(ctx context.Context, svc *registry.Service) error {
	return nil
}

// name(k8s集群中的服务地址) : service-name.namespace.svc.cluster_name:8080

func parse(name string) ([]string, int, error) {
	str := strings.FieldsFunc(name, func(r rune) bool {
		return r == ':'
	})
	var (
		port int
		err  error
	)
	if len(str) == 2 {
		port, err = strconv.Atoi(str[1])
		if err != nil {
			return nil, port, err
		}
	}
	str = strings.FieldsFunc(name, func(r rune) bool {
		return r == '.'
	})
	if len(str) < 2 {
		return nil, port, errors.InternalServer("k8s target url path invalid", "K8S_TARGET_URL_PATH_INVALID")
	}
	return str, port, nil
}

func service(endpoints *coreV1.Endpoints, port int) []*registry.Service {
	svc := make([]*registry.Service, 0, len(endpoints.Subsets))
	// meta.name --> len(endpoints.Subsets) == 1
	for _, set := range endpoints.Subsets {
		if len(set.Ports) == 0 {
			break
		}
		if port == 0 {
			port = int(set.Ports[0].Port)
		}
		for _, addr := range set.Addresses {
			s := &registry.Service{
				Name:     endpoints.Name,
				Version:  endpoints.ResourceVersion,
				Endpoint: fmt.Sprintf("%s:%d", addr.IP, port),
			}
			if addr.TargetRef != nil {
				s.ID = string(addr.TargetRef.UID)
			}
			svc = append(svc, s)
		}
	}
	return svc
}

func (r *Registry) Service(ctx context.Context, name string) ([]*registry.Service, error) {
	str, port, err := parse(name)
	if err != nil {
		return nil, err
	}
	endpoints, err := r.clientSet.CoreV1().Endpoints(str[1]).Get(context.Background(), str[0], metaV1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return service(endpoints, port), nil
}

func (r *Registry) Discover(_ context.Context, name string) (registry.Watcher, error) {
	str, port, err := parse(name)
	if err != nil {
		return nil, err
	}
	name = str[0]
	ns := str[1]

	if err != nil {
		return nil, err
	}
	inf := informers.NewSharedInformerFactoryWithOptions(r.clientSet, r.interval,
		informers.WithNamespace(ns),
		informers.WithTweakListOptions(func(options *metaV1.ListOptions) {
			options.FieldSelector = "metadata.name=" + name
		}))
	in := inf.Core().V1().Endpoints()
	notify := make(chan struct{}, 1)
	_, err = in.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			_, ok := obj.(*coreV1.Endpoints)
			if !ok {
				return
			}
			notify <- struct{}{}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oEndpoints, ok := oldObj.(*coreV1.Endpoints)
			if !ok {
				return
			}
			nEndpoints, ok := newObj.(*coreV1.Endpoints)
			if !ok {
				return
			}
			if oEndpoints.ResourceVersion == nEndpoints.ResourceVersion {
				return
			}
			notify <- struct{}{}
		},
		DeleteFunc: func(obj interface{}) {
			_, ok := obj.(*coreV1.Endpoints)
			if !ok {
				return
			}
			notify <- struct{}{}
		},
	})
	if err != nil {
		return nil, err
	}
	go inf.Start(context.Background().Done())
	w := &watcher{
		clientSet: r.clientSet,
		notify:    notify,
		ns:        ns,
		name:      name,
		port:      port,
	}
	return w, nil
}

type watcher struct {
	clientSet *kubernetes.Clientset
	ns        string
	name      string
	port      int
	notify    chan struct{}
}

func (w *watcher) List() ([]*registry.Service, error) {
	<-w.notify
	endpoints, err := w.clientSet.CoreV1().Endpoints(w.ns).Get(context.Background(), w.name, metaV1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return service(endpoints, w.port), nil
}

func (w *watcher) Stop() error {
	return nil
}
