package handler

import "time"

// SetAppLiveHeartbeatForTest overrides the live-stream heartbeat interval and
// returns a restore func, so tests can observe a heartbeat without waiting the
// full production cadence.
func SetAppLiveHeartbeatForTest(d time.Duration) func() {
	prev := appLiveHeartbeat
	appLiveHeartbeat = d
	return func() { appLiveHeartbeat = prev }
}

// SetAppVersionSizeCapsForTest overrides the publish size caps and returns a
// restore func, so oversize rejection can be exercised with tiny fixtures.
func SetAppVersionSizeCapsForTest(source, bundle int64) func() {
	prevSource, prevBundle := maxAppSourceBytes, maxAppBundleBytes
	maxAppSourceBytes, maxAppBundleBytes = source, bundle
	return func() { maxAppSourceBytes, maxAppBundleBytes = prevSource, prevBundle }
}
