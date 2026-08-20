package queue

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

type Queue struct {
	client *redis.Client
	name   string
}

func New(addr, name string) *Queue {
	return &Queue{client: redis.NewClient(&redis.Options{Addr: addr}), name: name}
}
func (q *Queue) Close() error                   { return q.client.Close() }
func (q *Queue) Ping(ctx context.Context) error { return q.client.Ping(ctx).Err() }
func (q *Queue) Enqueue(ctx context.Context, id string) error {
	return q.client.RPush(ctx, q.name, id).Err()
}
func (q *Queue) Dequeue(ctx context.Context) (string, error) {
	values, err := q.client.BLPop(ctx, time.Second, q.name).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(values) < 2 {
		return "", nil
	}
	return values[1], nil
}
func (q *Queue) Depth(ctx context.Context) (int64, error) { return q.client.LLen(ctx, q.name).Result() }
