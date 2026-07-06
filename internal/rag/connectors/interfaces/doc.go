// Package interfaces defines the connector contract every data-source
// integration implements. It is the single source of truth for:
//
//   - The trait hierarchy (Connector, CheckpointedConnector[T],
//     SlimConnector).
//   - The neutral document shapes (Document, Section, SlimDocument).
//   - The failure-propagation union types
//     (ConnectorFailure, DocumentOrFailure, SlimDocOrFailure).
//   - The factory registry (Register, Lookup, RegisteredKinds).
//
// This package is pure — no external service dependencies, no database,
// no network. It is consumed by the scheduler (Tranche 3C) and the
// concrete connector packages (github in Tranche 3D; notion / slack in
// later phases).
//
// The design is Go-idiomatic (channels + generics + interface
// constraints), with each trait — Connector, CheckpointedConnector[T],
// SlimConnector — carrying a precise semantic contract that the
// concrete connectors implement.
package interfaces
