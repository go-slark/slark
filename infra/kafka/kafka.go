package kafka

import (
	"context"
	"github.com/IBM/sarama"
	"github.com/go-slark/slark/logger"
	"github.com/go-slark/slark/pkg"
	"github.com/go-slark/slark/pkg/routine"
	tracing "github.com/go-slark/slark/pkg/trace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
	"time"
)

type KafkaProducer struct {
	sarama.SyncProducer
	sarama.AsyncProducer
	logger.Logger
	*tracing.Tracer
}

type ProducerConf struct {
	Brokers       []string `mapstructure:"brokers"`
	Retry         int      `mapstructure:"retry"`
	Ack           int16    `mapstructure:"ack"`
	ReturnSuccess bool     `mapstructure:"return_success"`
	ReturnErrors  bool     `mapstructure:"return_errors"`
	Version       string   `mapstructure:"version"`
	TraceEnable   bool     `mapstructure:"trace_enable"`
}

type ConsumerGroupConf struct {
	Brokers      []string      `mapstructure:"brokers"`
	GroupID      string        `mapstructure:"group_id"`
	Topics       []string      `mapstructure:"topics"`
	Initial      int64         `mapstructure:"initial"`
	ReturnErrors bool          `mapstructure:"return_errors"`
	AutoCommit   bool          `mapstructure:"auto_commit"`
	Interval     time.Duration `mapstructure:"interval"`
	WorkerNum    uint          `mapstructure:"worker_num"`
	Version      string        `mapstructure:"version"`
	Worker       int           `mapstructure:"worker"`
	TraceEnable  bool          `mapstructure:"trace_enable"`
}

type KafkaConf struct {
	Producer      *ProducerConf      `mapstructure:"producer"`
	ConsumerGroup *ConsumerGroupConf `mapstructure:"consumer_group"`
}

func (kp *KafkaProducer) Close() {
	_ = kp.SyncProducer.Close()
	kp.AsyncClose()
}

func (kp *KafkaProducer) SyncSend(ctx context.Context, topic, key string, msg []byte) error {
	traceID, ok := ctx.Value(utils.RayID).(string)
	var spanID string
	if !ok {
		traceID = tracing.ExtractTraceID(ctx)
		spanID = tracing.ExtractSpanID(ctx)
	}

	if kp.Tracer != nil {
		opt := []trace.SpanStartOption{
			trace.WithSpanKind(kp.Kind()),
			trace.WithAttributes(attribute.String("mq_topic", topic), attribute.String("mq_key", key), attribute.String("mq_msg", string(msg))),
		}
		_, span := kp.Start(ctx, "kafka sync send", &tracing.Carrier{MD: make(metadata.MD)}, opt...)
		defer span.End()
	}

	pm := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(msg),
		Key:   sarama.StringEncoder(key),
		Headers: []sarama.RecordHeader{
			{
				Key:   sarama.ByteEncoder(utils.RayID),
				Value: sarama.ByteEncoder(traceID),
			},
			{
				Key:   sarama.ByteEncoder(utils.SpanID),
				Value: sarama.ByteEncoder(spanID),
			},
		},
	}

	_, _, err := kp.SyncProducer.SendMessage(pm)
	return err
}

func (kp *KafkaProducer) AsyncSend(ctx context.Context, topic, key string, msg []byte) error {
	traceID, ok := ctx.Value(utils.RayID).(string)
	var spanID string
	if !ok {
		traceID = tracing.ExtractTraceID(ctx)
		spanID = tracing.ExtractSpanID(ctx)
	}

	if kp.Tracer != nil {
		opt := []trace.SpanStartOption{
			trace.WithSpanKind(kp.Kind()),
			trace.WithAttributes(attribute.String("mq_topic", topic), attribute.String("mq_key", key), attribute.String("mq_msg", string(msg))),
		}
		_, span := kp.Start(ctx, "kafka async send", &tracing.Carrier{MD: make(metadata.MD)}, opt...)
		defer span.End()
	}

	pm := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(msg),
		Key:   sarama.StringEncoder(key),
		Headers: []sarama.RecordHeader{
			{
				Key:   sarama.ByteEncoder(utils.RayID),
				Value: sarama.ByteEncoder(traceID),
			},
			{
				Key:   sarama.ByteEncoder(utils.SpanID),
				Value: sarama.ByteEncoder(spanID),
			},
		},
	}

	kp.AsyncProducer.Input() <- pm
	return nil
}

func (kp *KafkaProducer) monitor() {
	var (
		msg *sarama.ProducerMessage
		e   *sarama.ProducerError
	)
	go func(ap sarama.AsyncProducer) {
		for msg = range ap.Successes() {
			if msg != nil {
				kp.Log(context.TODO(), logger.DebugLevel, map[string]interface{}{"topic": msg.Topic, "key": msg.Key, "value": msg.Value}, "kafka async produce msg succ")
			}
		}
	}(kp.AsyncProducer)

	go func(ap sarama.AsyncProducer) {
		for e = range ap.Errors() {
			if e == nil {
				continue
			}

			if e.Msg != nil {
				kp.Log(context.TODO(), logger.ErrorLevel, map[string]interface{}{"error": e.Err, "topic": e.Msg.Topic, "key": e.Msg.Key, "value": e.Msg.Value}, "kafka async produce msg fail")
			} else {
				kp.Log(context.TODO(), logger.ErrorLevel, map[string]interface{}{"error": e.Err}, "kafka async produce msg fail")
			}
		}
	}(kp.AsyncProducer)
}

func InitKafkaProducer(conf *ProducerConf, opts ...tracing.Option) *KafkaProducer {
	kp := &KafkaProducer{
		SyncProducer:  newSyncProducer(conf),
		AsyncProducer: newAsyncProducer(conf),
		Logger:        logger.GetLogger(),
	}
	if conf.TraceEnable {
		kp.Tracer = tracing.NewTracer(trace.SpanKindProducer, opts...)
	}
	kp.monitor()
	return kp
}

func newSyncProducer(conf *ProducerConf) sarama.SyncProducer {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.RequiredAcks(conf.Ack) // WaitForAll
	config.Producer.Partitioner = sarama.NewHashPartitioner
	config.Producer.Retry.Max = conf.Retry
	config.Producer.Return.Successes = true // true
	//config.Producer.Return.Errors = true     // default true
	version, err := sarama.ParseKafkaVersion(conf.Version)
	if err != nil {
		panic(err)
	}
	config.Version = version
	if err = config.Validate(); err != nil {
		panic(err)
	}

	producer, err := sarama.NewSyncProducer(conf.Brokers, config)
	if err != nil {
		panic(err)
	}
	return producer
}

func newAsyncProducer(conf *ProducerConf) sarama.AsyncProducer {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.RequiredAcks(conf.Ack) // WaitForAll
	config.Producer.Partitioner = sarama.NewHashPartitioner
	config.Producer.Retry.Max = conf.Retry
	config.Producer.Return.Successes = conf.ReturnSuccess // true / false
	config.Producer.Return.Errors = conf.ReturnErrors     // true / false
	version, err := sarama.ParseKafkaVersion(conf.Version)
	if err != nil {
		panic(err)
	}
	config.Version = version
	if err = config.Validate(); err != nil {
		panic(err)
	}

	producer, err := sarama.NewAsyncProducer(conf.Brokers, config)
	if err != nil {
		panic(err)
	}
	return producer
}

type Consume interface {
	Handler(context.Context, *sarama.ConsumerMessage) error
}

type KafkaConsumerGroup struct {
	sarama.ConsumerGroup
	sarama.ConsumerGroupHandler
	Topics []string
	context.Context
	context.CancelFunc
	logger.Logger
	*tracing.Tracer
	handlers map[string]Consume
	worker   chan struct{}
}

func InitKafkaConsumer(conf *ConsumerGroupConf, opts ...tracing.Option) *KafkaConsumerGroup {
	k := &KafkaConsumerGroup{
		ConsumerGroup: newConsumerGroup(conf),
		Topics:        conf.Topics,
		Logger:        logger.GetLogger(),
		handlers:      make(map[string]Consume),
		worker:        make(chan struct{}, conf.Worker),
	}
	if conf.TraceEnable {
		k.Tracer = tracing.NewTracer(trace.SpanKindConsumer, opts...)
	}
	k.Context, k.CancelFunc = context.WithCancel(context.TODO())
	return k
}

func newConsumerGroup(conf *ConsumerGroupConf) sarama.ConsumerGroup {
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = conf.Initial              // -2:sarama.OffsetOldest -1:sarama.OffsetNewest
	config.Consumer.Offsets.AutoCommit.Enable = conf.AutoCommit // false
	config.Consumer.Offsets.AutoCommit.Interval = conf.Interval * time.Millisecond
	config.Consumer.Return.Errors = conf.ReturnErrors // true
	version, err := sarama.ParseKafkaVersion(conf.Version)
	if err != nil {
		panic(err)
	}
	config.Version = version
	if err = config.Validate(); err != nil {
		panic(err)
	}

	consumerGroup, err := sarama.NewConsumerGroup(conf.Brokers, conf.GroupID, config)
	if err != nil {
		panic(err)
	}

	return consumerGroup
}

func (kc *KafkaConsumerGroup) Register(topic string, handler Consume) {
	kc.handlers[topic] = handler
}

func (*KafkaConsumerGroup) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (*KafkaConsumerGroup) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (kc *KafkaConsumerGroup) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	handler, ok := kc.handlers[claim.Topic()]
	if !ok {
		return nil
	}

	for msg := range claim.Messages() {
		kc.worker <- struct{}{}
		m := msg
		ctx := context.Background()
		var span trace.Span
		routine.Go(ctx, func() {
			defer func() {
				<-kc.worker
			}()
			if kc.Tracer != nil {
				opt := []trace.SpanStartOption{
					trace.WithSpanKind(kc.Kind()),
					trace.WithAttributes(attribute.String("mq_topic", msg.Topic), attribute.String("mq_key", string(msg.Key)), attribute.String("mq_msg", string(msg.Value))),
				}
				ctx, span = kc.Tracer.Start(ctx, "kafka group consume", &tracing.Carrier{MD: make(metadata.MD)}, opt...)
				defer span.End()
			}
			err := handler.Handler(ctx, m)
			if err != nil {
				if span != nil {
					// TODO
				}
			}
		})
		sess.MarkMessage(msg, "")
	}
	return nil
}

func (kc *KafkaConsumerGroup) Consume() {
	for {
		err := kc.ConsumerGroup.Consume(kc.Context, kc.Topics, kc.ConsumerGroupHandler)
		if err != nil {
			kc.Log(kc.Context, logger.WarnLevel, map[string]interface{}{"error": err}, "consumer group consume fail")
		}
		if kc.Context.Err() != nil {
			kc.Log(kc.Context, logger.ErrorLevel, map[string]interface{}{"error": kc.Context.Err()}, "consumer group exit")
			return
		}
		time.Sleep(time.Second)
	}
}

func (kc *KafkaConsumerGroup) Start() error {
	kc.Consume()
	return nil
}

func (kc *KafkaConsumerGroup) Stop(_ context.Context) error {
	kc.CancelFunc()
	return kc.Close()
}

var (
	kafkaProducer      *KafkaProducer
	kafkaConsumerGroup *KafkaConsumerGroup
)

func GetKafkaProducer() *KafkaProducer {
	return kafkaProducer
}

func GetKafkaConsumerGroup() *KafkaConsumerGroup {
	return kafkaConsumerGroup
}
