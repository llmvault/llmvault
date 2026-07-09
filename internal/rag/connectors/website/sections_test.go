package website

import "testing"

func TestGroupLinks(t *testing.T) {
	links := []string{
		"https://acme.com/",
		"https://acme.com/docs",
		"https://acme.com/docs/getting-started",
		"https://acme.com/docs/getting-started/", // dup of above (trailing slash)
		"https://acme.com/docs/api",
		"https://acme.com/docs/api/auth",
		"https://acme.com/blog/hello",
		"https://acme.com/blog/world",
		"https://acme.com/blog/again",
		"https://acme.com/pricing",    // singleton -> page
		"https://acme.com/about#team", // fragment kept in raw, path is /about
		"https://other.com/docs/x",    // different host -> ignored
	}

	got, err := GroupLinks("https://acme.com", links, 3)
	if err != nil {
		t.Fatalf("GroupLinks: %v", err)
	}

	sectionByPrefix := map[string]DiscoveredSection{}
	for _, s := range got.Sections {
		sectionByPrefix[s.PathPrefix] = s
	}
	docs, ok := sectionByPrefix["/docs"]
	if !ok {
		t.Fatalf("expected a /docs section; got sections %+v", got.Sections)
	}
	if docs.URL != "https://acme.com/docs" || docs.Label != "Docs" {
		t.Fatalf("docs section url/label = %q/%q", docs.URL, docs.Label)
	}
	if docs.PageCount != 4 { // /docs, /docs/getting-started, /docs/api, /docs/api/auth
		t.Fatalf("docs PageCount = %d, want 4", docs.PageCount)
	}
	// URLs holds the full page list (not just the sample) so a section selection
	// expands to exactly these pages.
	if len(docs.URLs) != 4 {
		t.Fatalf("docs URLs = %d, want 4 (full page list): %v", len(docs.URLs), docs.URLs)
	}
	foundAuth := false
	for _, u := range docs.URLs {
		if u == "https://acme.com/docs/api/auth" {
			foundAuth = true
		}
	}
	if !foundAuth {
		t.Fatalf("docs URLs missing https://acme.com/docs/api/auth: %v", docs.URLs)
	}
	blog, ok := sectionByPrefix["/blog"]
	if !ok || blog.PageCount != 3 {
		t.Fatalf("expected /blog section with 3 pages; got %+v", blog)
	}

	// Largest section first.
	if got.Sections[0].PathPrefix != "/docs" {
		t.Fatalf("sections not sorted by count desc: %+v", got.Sections)
	}

	// pricing + about are singletons -> individual pages; other.com ignored.
	pagePaths := map[string]bool{}
	for _, p := range got.Pages {
		pagePaths[p.Path] = true
		if p.URL[:16] != "https://acme.com" {
			t.Fatalf("page from wrong host: %s", p.URL)
		}
	}
	if !pagePaths["/pricing"] || !pagePaths["/about"] {
		t.Fatalf("expected /pricing and /about as pages; got %+v", got.Pages)
	}
	if pagePaths["/docs"] {
		t.Fatalf("/docs should be a section, not a page")
	}
}

func TestGroupLinksWWWEquivalence(t *testing.T) {
	links := []string{
		"https://example.com/",
		"https://www.example.com/listings",
		"https://www.example.com/listings/1",
		"https://WWW.Example.Com/listings/2",
		"https://example.com/listings/1",
		"https://www.example.com/pricing",
	}

	got, err := GroupLinks("https://example.com", links, 3)
	if err != nil {
		t.Fatalf("GroupLinks: %v", err)
	}

	var listings *DiscoveredSection
	for i := range got.Sections {
		if got.Sections[i].PathPrefix == "/listings" {
			listings = &got.Sections[i]
		}
	}
	if listings == nil {
		t.Fatalf("expected a /listings section; got sections %+v, pages %+v", got.Sections, got.Pages)
	}
	if listings.PageCount != 3 {
		t.Fatalf("listings PageCount = %d, want 3: %v", listings.PageCount, listings.URLs)
	}
	for _, u := range listings.URLs {
		if u[:19] != "https://example.com" {
			t.Fatalf("section URL not re-rooted on base origin: %s", u)
		}
	}

	pagePaths := map[string]bool{}
	for _, p := range got.Pages {
		pagePaths[p.Path] = true
	}
	if !pagePaths["/pricing"] {
		t.Fatalf("expected www-hosted /pricing to be kept as a page; got %+v", got.Pages)
	}
}

func TestFirstSegmentAndHumanize(t *testing.T) {
	cases := map[string]string{
		"/docs/getting-started": "docs",
		"/blog":                 "blog",
		"/":                     "",
		"":                      "",
	}
	for path, want := range cases {
		if got := firstSegment(path); got != want {
			t.Fatalf("firstSegment(%q) = %q, want %q", path, got, want)
		}
	}
	if got := humanizeSegment("getting-started"); got != "Getting Started" {
		t.Fatalf("humanize = %q", got)
	}
	if got := humanizeSegment("api_reference"); got != "Api Reference" {
		t.Fatalf("humanize = %q", got)
	}
}
