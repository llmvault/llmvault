// ACL prefix helpers and the public-doc sentinel string.
//
// These four prefix helpers and the public-doc sentinel string form the
// security boundary of the RAG subsystem: at write time the indexer
// stamps the same prefixed strings into each chunk's ACL list; at read
// time the query filter reconstructs the same prefixed strings to match
// them. Any byte-level drift between the two sides = 0 results or, worse,
// wrong results (cross-user / cross-group leakage).
//
// Tests in prefix_test.go pin every output string exactly, so changes to
// this file MUST be made in lockstep with the test fixtures.
package acl

import (
	"strings"

	"github.com/usehivy/hivy/internal/rag/model"
)

// PublicDocPat is the sentinel ACL string stamped on documents that are
// readable by every member of an org.
//
// Deliberately uppercase. Queries that run on behalf of an authenticated
// org member add this constant to their ACL allow-list; queries run as
// a non-member (e.g. an external share) do not.
const PublicDocPat = "PUBLIC"

// PrefixUserEmail namespaces a user email inside an ACL list. The
// "user_email:" prefix prevents collision with group names inside the
// same ACL list and lets the vector-store filter short-circuit on
// prefix at query time.
func PrefixUserEmail(email string) string {
	return "user_email:" + email
}

// PrefixUserGroup namespaces an internal user-group name. The "group:"
// prefix separates internal user groups from user emails and external
// groups within the same ACL list.
//
// NOTE: Hivy does not maintain an internal UserGroup table. This helper
// is kept because indexing code paths that compute ACLs may encounter
// older group rows during migration and because the helper is cheap to
// keep around.
func PrefixUserGroup(name string) string {
	return "group:" + name
}

// PrefixExternalGroup namespaces a source-synced group name. The
// "external_group:" prefix separates source-synced groups (e.g. GitHub
// teams, Google Drive shared-drive members) from user emails and
// internal user groups.
func PrefixExternalGroup(name string) string {
	return "external_group:" + name
}

// BuildExtGroupName builds a source-scoped external group name. The
// source value prefixes the raw group name so two sources emitting the
// same group slug (e.g. "engineering" in GitHub and in Google Drive) do
// not collide inside the ACL list.
//
// The result is lowercased because case-preserving would make ACL
// matching case-sensitive, which is inconsistent with how most sources
// return group names. This lowercase
// is a load-bearing invariant: the indexer and the query path both call
// this function, so both paths produce the same byte sequence.
//
// NOTE: this function is NOT idempotent. Calling it twice with the same
// source double-prefixes: `BuildExtGroupName(BuildExtGroupName("x", github), github)`
// → "github_github_x". The test `TestBuildExtGroupName_Idempotent` pins
// that behavior so callers do not accidentally layer prefixes.
func BuildExtGroupName(extGroupName string, source model.DocumentSource) string {
	return strings.ToLower(string(source) + "_" + extGroupName)
}
