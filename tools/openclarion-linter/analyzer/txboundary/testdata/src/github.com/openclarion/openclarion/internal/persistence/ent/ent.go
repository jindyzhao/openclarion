package ent

import "context"

type Client struct{}

type Tx struct{}

func (c *Client) Tx(context.Context) (*Tx, error) {
	return &Tx{}, nil
}

func (t *Tx) Commit() error {
	return nil
}

func (t *Tx) Rollback() error {
	return nil
}
