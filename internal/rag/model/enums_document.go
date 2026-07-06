// Package model houses gorm models for the RAG
// subsystem.
//
// Enums are split across multiple files by the tables they describe
// (enums_document.go: DocumentSource + HierarchyNodeType; enums_index_attempt.go:
// indexing + sync status; enums_sync_state.go: connection lifecycle +
// access type) so each file reads as a small, self-contained enum
// group.
package model

// DocumentSource enumerates every upstream source a document can come
// from. Values MUST stay stable byte-for-byte because they are persisted
// to the DB and because the ACL helper BuildExtGroupName lowercases the
// source name into ACL strings written to the vector index — any drift
// would produce silent zero-result filters.
type DocumentSource string

// DocumentSource constants. The string values are a fixed contract
// stamped into stored data and ACL strings; do not change them.
const (
	DocumentSourceIngestionAPI       DocumentSource = "ingestion_api"
	DocumentSourceSlack              DocumentSource = "slack"
	DocumentSourceWeb                DocumentSource = "web"
	DocumentSourceGoogleDrive        DocumentSource = "google_drive"
	DocumentSourceGmail              DocumentSource = "gmail"
	DocumentSourceRequestTracker     DocumentSource = "requesttracker"
	DocumentSourceGithub             DocumentSource = "github"
	DocumentSourceGitbook            DocumentSource = "gitbook"
	DocumentSourceGitlab             DocumentSource = "gitlab"
	DocumentSourceGuru               DocumentSource = "guru"
	DocumentSourceBookstack          DocumentSource = "bookstack"
	DocumentSourceOutline            DocumentSource = "outline"
	DocumentSourceConfluence         DocumentSource = "confluence"
	DocumentSourceJira               DocumentSource = "jira"
	DocumentSourceSlab               DocumentSource = "slab"
	DocumentSourceProductboard       DocumentSource = "productboard"
	DocumentSourceFile               DocumentSource = "file"
	DocumentSourceCoda               DocumentSource = "coda"
	DocumentSourceCanvas             DocumentSource = "canvas"
	DocumentSourceNotion             DocumentSource = "notion"
	DocumentSourceZulip              DocumentSource = "zulip"
	DocumentSourceLinear             DocumentSource = "linear"
	DocumentSourceHubspot            DocumentSource = "hubspot"
	DocumentSourceDocument360        DocumentSource = "document360"
	DocumentSourceGong               DocumentSource = "gong"
	DocumentSourceGoogleSites        DocumentSource = "google_sites"
	DocumentSourceZendesk            DocumentSource = "zendesk"
	DocumentSourceLoopio             DocumentSource = "loopio"
	DocumentSourceDropbox            DocumentSource = "dropbox"
	DocumentSourceSharepoint         DocumentSource = "sharepoint"
	DocumentSourceTeams              DocumentSource = "teams"
	DocumentSourceSalesforce         DocumentSource = "salesforce"
	DocumentSourceDiscourse          DocumentSource = "discourse"
	DocumentSourceAxero              DocumentSource = "axero"
	DocumentSourceClickup            DocumentSource = "clickup"
	DocumentSourceMediawiki          DocumentSource = "mediawiki"
	DocumentSourceWikipedia          DocumentSource = "wikipedia"
	DocumentSourceAsana              DocumentSource = "asana"
	DocumentSourceS3                 DocumentSource = "s3"
	DocumentSourceR2                 DocumentSource = "r2"
	DocumentSourceGoogleCloudStorage DocumentSource = "google_cloud_storage"
	DocumentSourceOCIStorage         DocumentSource = "oci_storage"
	DocumentSourceXenforo            DocumentSource = "xenforo"
	DocumentSourceNotApplicable      DocumentSource = "not_applicable"
	DocumentSourceDiscord            DocumentSource = "discord"
	DocumentSourceFreshdesk          DocumentSource = "freshdesk"
	DocumentSourceFireflies          DocumentSource = "fireflies"
	DocumentSourceEgnyte             DocumentSource = "egnyte"
	DocumentSourceAirtable           DocumentSource = "airtable"
	DocumentSourceHighspot           DocumentSource = "highspot"
	DocumentSourceDrupalWiki         DocumentSource = "drupal_wiki"
	DocumentSourceIMAP               DocumentSource = "imap"
	DocumentSourceBitbucket          DocumentSource = "bitbucket"
	DocumentSourceTestrail           DocumentSource = "testrail"
	DocumentSourceMockConnector      DocumentSource = "mock_connector"
	DocumentSourceUserFile           DocumentSource = "user_file"
	DocumentSourceCraftFile          DocumentSource = "craft_file"
)

// allDocumentSources is an internal allowlist used by IsValid. Kept in a
// var (not a generated function) so the set lives next to the const block
// above for auditability.
var allDocumentSources = map[DocumentSource]struct{}{
	DocumentSourceIngestionAPI:       {},
	DocumentSourceSlack:              {},
	DocumentSourceWeb:                {},
	DocumentSourceGoogleDrive:        {},
	DocumentSourceGmail:              {},
	DocumentSourceRequestTracker:     {},
	DocumentSourceGithub:             {},
	DocumentSourceGitbook:            {},
	DocumentSourceGitlab:             {},
	DocumentSourceGuru:               {},
	DocumentSourceBookstack:          {},
	DocumentSourceOutline:            {},
	DocumentSourceConfluence:         {},
	DocumentSourceJira:               {},
	DocumentSourceSlab:               {},
	DocumentSourceProductboard:       {},
	DocumentSourceFile:               {},
	DocumentSourceCoda:               {},
	DocumentSourceCanvas:             {},
	DocumentSourceNotion:             {},
	DocumentSourceZulip:              {},
	DocumentSourceLinear:             {},
	DocumentSourceHubspot:            {},
	DocumentSourceDocument360:        {},
	DocumentSourceGong:               {},
	DocumentSourceGoogleSites:        {},
	DocumentSourceZendesk:            {},
	DocumentSourceLoopio:             {},
	DocumentSourceDropbox:            {},
	DocumentSourceSharepoint:         {},
	DocumentSourceTeams:              {},
	DocumentSourceSalesforce:         {},
	DocumentSourceDiscourse:          {},
	DocumentSourceAxero:              {},
	DocumentSourceClickup:            {},
	DocumentSourceMediawiki:          {},
	DocumentSourceWikipedia:          {},
	DocumentSourceAsana:              {},
	DocumentSourceS3:                 {},
	DocumentSourceR2:                 {},
	DocumentSourceGoogleCloudStorage: {},
	DocumentSourceOCIStorage:         {},
	DocumentSourceXenforo:            {},
	DocumentSourceNotApplicable:      {},
	DocumentSourceDiscord:            {},
	DocumentSourceFreshdesk:          {},
	DocumentSourceFireflies:          {},
	DocumentSourceEgnyte:             {},
	DocumentSourceAirtable:           {},
	DocumentSourceHighspot:           {},
	DocumentSourceDrupalWiki:         {},
	DocumentSourceIMAP:               {},
	DocumentSourceBitbucket:          {},
	DocumentSourceTestrail:           {},
	DocumentSourceMockConnector:      {},
	DocumentSourceUserFile:           {},
	DocumentSourceCraftFile:          {},
}

// IsValid reports whether s is a known DocumentSource. Pure function,
// intended for admin-API input validation so typo'd source names are
// rejected before any DB write — unknown strings are refused.
func (s DocumentSource) IsValid() bool {
	_, ok := allDocumentSources[s]
	return ok
}
