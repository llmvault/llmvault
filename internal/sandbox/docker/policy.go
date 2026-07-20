package docker

import (
	"context"
	"time"
)

func (d *Driver) SetAutoStop(context.Context, string, time.Duration) error {
	return nil
}

func (d *Driver) SetAutoArchive(context.Context, string, int) error {
	return nil
}
