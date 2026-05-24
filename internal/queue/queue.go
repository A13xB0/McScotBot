package queue

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"meshcore-bots/internal/remoteterm"
)

const queueCapacity = 64

// ScopedSendRequest is an atomic send: set scope, send message, clear scope.
type ScopedSendRequest struct {
	Scope      string
	ChannelKey string
	ChannelName string // display only
	Text       string
}

// Queue is a rate-limited, single-worker send queue.
// All bots enqueue via Enqueue(); the drain goroutine serialises
// scope-set → send → scope-clear with a minimum interval between sends.
type Queue struct {
	client       *remoteterm.Client
	interval     time.Duration
	dryRun       bool
	ch           chan ScopedSendRequest
	wg           sync.WaitGroup
	channelNames map[string]string // key → "#name" for display
}

func New(client *remoteterm.Client, maxPerMinute int, dryRun bool, channelNames map[string]string) *Queue {
	interval := time.Minute / time.Duration(maxPerMinute)
	return &Queue{
		client:       client,
		interval:     interval,
		dryRun:       dryRun,
		ch:           make(chan ScopedSendRequest, queueCapacity),
		channelNames: channelNames,
	}
}

// Enqueue adds a scoped send to the queue.
// In dry-run mode it prints immediately and returns without queuing.
// If the queue is full (shouldn't happen in normal operation) the message is dropped.
func (q *Queue) Enqueue(scope, channelKey, text string) {
	name := q.channelNames[channelKey]
	if name == "" {
		name = channelKey[:8] + "…"
	}

	if q.dryRun {
		fmt.Printf("[DRY RUN] scope=%-5s  channel=%-10s  text=%q\n", scope, name, text)
		return
	}

	req := ScopedSendRequest{Scope: scope, ChannelKey: channelKey, ChannelName: name, Text: text}
	q.wg.Add(1)
	select {
	case q.ch <- req:
	default:
		log.Printf("WARN queue full, dropping: scope=%s channel=%s text=%.40s…", scope, name, text)
		q.wg.Done()
	}
}

// Start runs the drain goroutine. Call as go q.Start(ctx).
func (q *Queue) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-q.ch:
			q.process(req)
			q.wg.Done()
			// Rate-limit: sleep the remainder of the interval
			select {
			case <-ctx.Done():
				return
			case <-time.After(q.interval):
			}
		}
	}
}

// Drain blocks until all enqueued messages have been sent.
// Use with --run-now to wait before exiting.
func (q *Queue) Drain() {
	q.wg.Wait()
}

func (q *Queue) process(req ScopedSendRequest) {
	log.Printf("Sending: scope=%s channel=%s text=%.60s…", req.Scope, req.ChannelName, req.Text)

	if err := q.client.SetScopeOverride(req.ChannelKey, req.Scope); err != nil {
		log.Printf("ERROR SetScopeOverride scope=%s channel=%s: %v", req.Scope, req.ChannelName, err)
		return
	}

	if err := q.client.SendMessage(req.ChannelKey, req.Text); err != nil {
		log.Printf("ERROR SendMessage scope=%s channel=%s: %v", req.Scope, req.ChannelName, err)
		// Still clear the scope override even on send failure.
	}

	if err := q.client.ClearScopeOverride(req.ChannelKey); err != nil {
		log.Printf("ERROR ClearScopeOverride channel=%s: %v", req.ChannelName, err)
	}
}
