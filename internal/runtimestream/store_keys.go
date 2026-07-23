package runtimestream

func (s *Store) IsCluster() bool {
	return s != nil && s.cluster
}

func (s *Store) StreamKey(shard int) string {
	if s.IsCluster() {
		return ClusterStreamKey(shard)
	}
	return StreamKey(shard)
}

func (s *Store) LastSeqKey(sessionID string) string {
	if s.IsCluster() {
		shard := ShardForSession(sessionID, s.ShardCount())
		return clusterSessionKey(shard, sessionID, "last_seq")
	}
	return LastSeqKey(sessionID)
}

func (s *Store) EventIndexKey(sessionID string) string {
	if s.IsCluster() {
		shard := ShardForSession(sessionID, s.ShardCount())
		return clusterSessionKey(shard, sessionID, "event_index")
	}
	return EventIndexKey(sessionID)
}

func (s *Store) ProjectedSeqKey(sessionID string) string {
	if s.IsCluster() {
		shard := ShardForSession(sessionID, s.ShardCount())
		return clusterSessionKey(shard, sessionID, "projected_seq")
	}
	return ProjectedSeqKey(sessionID)
}

func (s *Store) ShardLeaseKey(shard int) string {
	return s.StreamKey(shard) + ":lease"
}
