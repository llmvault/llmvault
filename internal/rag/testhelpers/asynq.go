package testhelpers

import (
	"testing"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/testdb"
)

func AsynqRedisOpt() asynq.RedisConnOpt {
	if testdb.RedisClusterEnabled() {
		return asynq.RedisClusterClientOpt{
			Addrs:    testdb.RedisClusterAddrs(),
			Password: testdb.RedisPassword(),
		}
	}
	return asynq.RedisClientOpt{
		Addr:     RedisAddr(),
		Password: testdb.RedisPassword(),
		DB:       RedisDB(),
	}
}

func NewTestAsynqClient(t *testing.T) *enqueue.Client {
	t.Helper()
	cli := enqueue.NewClient(AsynqRedisOpt())
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

func NewTestAsynqInspector(t *testing.T) *asynq.Inspector {
	t.Helper()
	insp := asynq.NewInspector(AsynqRedisOpt())
	t.Cleanup(func() { _ = insp.Close() })
	return insp
}
