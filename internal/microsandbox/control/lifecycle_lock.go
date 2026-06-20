package control

import (
	"context"
	"fmt"
)

func (s *Server) acquireDistributedSandboxLock(ctx context.Context, sandboxID string) (func(), error) {
	if s.db == nil || s.db.Dialector == nil || s.db.Dialector.Name() != "postgres" {
		return func() {}, nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	key := "microsandbox:sandbox:" + sandboxID
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext($1))`, key); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire sandbox lifecycle lock: %w", err)
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, key)
		_ = conn.Close()
	}, nil
}
