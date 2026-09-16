package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

type Client struct {
	Conn *nats.Conn
}

func Connect(ctx context.Context, url string) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	if err := ctx.Err(); err != nil {
		conn.Close()
		return nil, err
	}
	return &Client{Conn: conn}, nil
}

func (c *Client) Close() {
	c.Conn.Close()
}

func (c *Client) Drain() error {
	return c.Conn.Drain()
}

func (c *Client) PublishEvent(ctx context.Context, subject string, v interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Conn.Publish(subject, data)
}
