package rabbitmq

import (
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Config holds the settings needed to open a connection.
type Config struct {
	// URL is the full AMQP connection string, e.g.
	// "amqp://guest:guest@localhost:5672/"
	URL string

	// Prefetch controls how many unacknowledged messages a consumer may hold.
	// Defaults to 10 if zero.
	Prefetch int
}

// Connection wraps an amqp.Connection and provides a factory for channels.
type Connection struct {
	conn     *amqp.Connection
	prefetch int
}

// Connect dials the broker and returns a ready Connection.
// The caller must call Close() when done.
func Connect(cfg Config) (*Connection, error) {
	if cfg.Prefetch == 0 {
		cfg.Prefetch = 10
	}

	conn, err := dialWithRetry(cfg.URL, 5, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: connect: %w", err)
	}

	return &Connection{conn: conn, prefetch: cfg.Prefetch}, nil
}

// Channel opens a new AMQP channel and applies the configured QoS prefetch.
func (c *Connection) Channel() (*amqp.Channel, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: open channel: %w", err)
	}
	if err := ch.Qos(c.prefetch, 0, false); err != nil {
		ch.Close()
		return nil, fmt.Errorf("rabbitmq: set qos: %w", err)
	}
	return ch, nil
}

// Close shuts the underlying AMQP connection.
func (c *Connection) Close() error {
	return c.conn.Close()
}

// dialWithRetry attempts to connect up to maxAttempts times before giving up.
func dialWithRetry(url string, maxAttempts int, delay time.Duration) (*amqp.Connection, error) {
	var lastErr error
	for i := range maxAttempts {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		// Avoid sleeping after the last attempt.
		if i < maxAttempts-1 {
			time.Sleep(delay)
		}
	}
	return nil, fmt.Errorf("all %d dial attempts failed: %w", maxAttempts, lastErr)
}
