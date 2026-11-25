// Package audit publishes audit events to Kafka and persists them via a
// consumer-group worker. If Kafka is unreachable, events are written straight
// to Postgres so the audit trail is never silently dropped.
package audit

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"
)

type Event struct {
	ID         uuid.UUID      `json:"id"`
	OrgID      uuid.UUID      `json:"org_id"`
	ProjectID  *uuid.UUID     `json:"project_id,omitempty"`
	ActorType  string         `json:"actor_type"` // user | token | system
	ActorID    string         `json:"actor_id"`
	ActorLabel string         `json:"actor_label"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	IP         string         `json:"ip"`
	At         time.Time      `json:"at"`
}

// Sealer hides personal data in events before they are stored or sent through Kafka.
type Sealer interface {
	Label(string) string
	OpenLabel(string) string
}

type Service struct {
	sealer  Sealer
	subsMu  sync.RWMutex
	subs    []func(Event)
	pool    *pgxpool.Pool
	brokers []string
	topic   string
	writer  *kafka.Writer
}

func New(pool *pgxpool.Pool, brokers []string, topic string) *Service {
	s := &Service{pool: pool, brokers: brokers, topic: topic}
	if len(brokers) > 0 {
		s.writer = &kafka.Writer{
			Addr:                   kafka.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafka.Hash{}, // same org -> same partition -> ordered
			RequiredAcks:           kafka.RequireOne,
			BatchTimeout:           10 * time.Millisecond,
			AllowAutoTopicCreation: true,
		}
	}
	return s
}

func (s *Service) WithSealer(x Sealer) *Service { s.sealer = x; return s }

func (s *Service) seal(e Event) Event {
	if s.sealer != nil {
		e.ActorLabel, e.Resource = s.sealer.Label(e.ActorLabel), s.sealer.Label(e.Resource)
	}
	return e
}

func (s *Service) open(e Event) Event {
	if s.sealer != nil {
		e.ActorLabel, e.Resource = s.sealer.OpenLabel(e.ActorLabel), s.sealer.OpenLabel(e.Resource)
	}
	return e
}

// Subscribe registers a handler called after an event has been persisted.
func (s *Service) Subscribe(fn func(Event)) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	s.subs = append(s.subs, fn)
}

func (s *Service) notify(e Event) {
	s.subsMu.RLock()
	subs := append([]func(Event){}, s.subs...)
	s.subsMu.RUnlock()
	for _, fn := range subs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("audit: subscriber panic: %v", r)
				}
			}()
			fn(e)
		}()
	}
}

// Publish never fails the caller.
func (s *Service) Publish(ctx context.Context, e Event) {
	e.ID = uuid.New()
	e.At = time.Now().UTC()
	if s.writer != nil {
		payload, _ := json.Marshal(s.seal(e))
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		err := s.writer.WriteMessages(wctx, kafka.Message{Key: []byte(e.OrgID.String()), Value: payload})
		if err == nil {
			return
		}
		log.Printf("audit: kafka publish failed (%v); writing directly", err)
	}
	if err := s.insert(context.WithoutCancel(ctx), e); err != nil {
		log.Printf("audit: LOST event %s: %v", e.Action, err)
		return
	}
	s.notify(e)
}

// Run creates the topic and consumes events until ctx is done.
func (s *Service) Run(ctx context.Context) {
	if s.writer == nil {
		log.Println("audit: no KAFKA_BROKERS set, using direct DB writes")
		return
	}
	for ctx.Err() == nil {
		if err := s.ensureTopic(); err != nil {
			log.Printf("audit: waiting for kafka: %v", err)
			sleep(ctx, 3*time.Second)
			continue
		}
		break
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: s.brokers, GroupID: "vaultly-audit-writer", Topic: s.topic,
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: 500 * time.Millisecond,
	})
	defer r.Close()
	log.Printf("audit: consuming topic %q", s.topic)
	for ctx.Err() == nil {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("audit: fetch: %v", err)
				sleep(ctx, time.Second)
			}
			continue
		}
		var e Event
		if err := json.Unmarshal(m.Value, &e); err != nil {
			log.Printf("audit: dropping malformed message: %v", err)
			_ = r.CommitMessages(ctx, m)
			continue
		}
		for ctx.Err() == nil {
			if err := s.insert(ctx, e); err != nil {
				log.Printf("audit: insert failed, retrying: %v", err)
				sleep(ctx, 2*time.Second)
				continue
			}
			_ = r.CommitMessages(ctx, m)
			s.notify(s.open(e))
			break
		}
	}
}

func (s *Service) Close() {
	if s.writer != nil {
		_ = s.writer.Close()
	}
}

func (s *Service) insert(ctx context.Context, e Event) error {
	e = s.seal(e)
	md, _ := json.Marshal(e.Metadata)
	if e.Metadata == nil {
		md = []byte("{}")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (id, org_id, project_id, actor_type, actor_id, actor_label, action, resource, metadata, ip, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (id) DO NOTHING`,
		e.ID, e.OrgID, e.ProjectID, e.ActorType, e.ActorID, e.ActorLabel, e.Action, e.Resource, md, e.IP, e.At)
	return err
}

func (s *Service) ensureTopic() error {
	conn, err := kafka.Dial("tcp", s.brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()
	ctrl, err := conn.Controller()
	if err != nil {
		return err
	}
	cc, err := kafka.Dial("tcp", net.JoinHostPort(ctrl.Host, strconv.Itoa(ctrl.Port)))
	if err != nil {
		return err
	}
	defer cc.Close()
	return cc.CreateTopics(kafka.TopicConfig{Topic: s.topic, NumPartitions: 3, ReplicationFactor: 1})
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
