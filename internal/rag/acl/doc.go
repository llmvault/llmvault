// Package acl builds and applies the document-level access-control strings
// that Qdrant stores alongside each chunk and filters by at query time.
//
// It provides the prefix helpers (PrefixUserEmail, PrefixUserGroup,
// PrefixExternalGroup, BuildExtGroupName) and the ACL assembly used to
// tag each document.
//
// Invariant: prefix strings are applied on read and on write. Off-by-one on
// the prefix = zero search results, so this package is pure-logic + tested
// byte-exactly against pinned expected strings.
package acl
