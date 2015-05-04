package geomi

import (
	"fmt"
	"net/url"
	"sort"
	"testing"
	"time"
)

// testFetcher is Fetcher for testing
type testFetcher map[string]*testResult

type testResult struct {
	body string
	urls []string
}

func (t *testFetcher) Fetch(url string) (string, ResponseInfo, []string) {
	if res, ok := (*t)[url]; ok {
		return res.body, ResponseInfo{}, res.urls
	}
	return "", ResponseInfo{Err: fmt.Errorf("not found: %s", url)}, nil
}

// tester is a populated testFetcher.
var tester = &testFetcher{
	"http://golang.org/": &testResult{
		"The Go Programming Language",
		[]string{
			"http://golang.org/pkg/",
			"http://golang.org/cmd/",
		},
	},
	"http://golang.org/pkg/": &testResult{
		"Packages",
		[]string{
			"http://golang.org/",
			"http://golang.org/cmd/",
			"http://golang.org/pkg/fmt/",
			"http://golang.org/pkg/os/",
		},
	},
	"http://golang.org/pkg/fmt/": &testResult{
		"Package fmt",
		[]string{
			"http://golang.org/",
			"http://golang.org/pkg/",
		},
	},
	"http://golang.org/pkg/os/": &testResult{
		"Package os",
		[]string{
			"http://golang.org/",
			"http://golang.org/pkg/",
		},
	},
	"http://golang.org/cmd/": &testResult{
		"Commands",
		[]string{
			"http://golang.org/",
			"http://golang.org/pkg/",
			"http://golang.org/cmd/gofmt/",
			"http://golang.org/cmd/pprof/",
		},
	},
	"http://golang.org/cmd/gofmt/": &testResult{
		"Command gofmt",
		[]string{
			"http://golang.org/",
			"http://golang.org/cmd/",
		},
	},
	"http://golang.org/cmd/pprof/": &testResult{
		"Command pprof",
		[]string{
			"http://golang.org/",
			"http://golang.org/cmd/",
		},
	},
}

func TestNewSpider(t *testing.T) {
	tests := []struct {
		url         string
		expectedURL string
		expectedErr string
	}{
		{"", "", "newSpider: the start url cannot be empty"},
		{":golang", "", "parse :golang: missing protocol scheme"},
		{"http://golang.org/", "http://golang.org/", ""},
		{"http://golang.org/cmd/", "http://golang.org/cmd/", ""},
	}

	for _, test := range tests {
		s, err := NewSpider(test.url)
		if err != nil && test.expectedErr == "" {
			t.Errorf("Expected no error, got %q", err.Error())
			continue
		}
		if test.expectedErr != "" {
			if err == nil {
				t.Errorf("Expected error to be %q; got none", test.expectedErr)
			} else {
				if test.expectedErr != err.Error() {
					t.Errorf("Expected error to be %q; got %q", test.expectedErr, err.Error())
				}
			}
			continue
		}
		if s.URL.String() != test.expectedURL {
			t.Errorf("Expected basePath to be %q; %q", test.expectedURL, s.URL.String())
		}
	}
}

func TestCrawl(t *testing.T) {
	tests := []struct {
		depth       int
		url         []string
		expected    []Page
		expectedErr string
	}{
		{0, []string{"http://golang.org/"}, []Page{Page{distance: 0, body: "The Go Programming Language", links: []string{"http://golang.org/pkg/", "http://golang.org/cmd/"}}}, ""},
		{1, []string{"http://golang.org/", "http://golang.org/pkg/", "http://golang.org/cmd/"},
			[]Page{Page{distance: 0, body: "The Go Programming Language", links: []string{"http://golang.org/pkg/", "http://golang.org/cmd/"}},
				Page{distance: 1, body: "Packages", links: []string{"http://golang.org/", "http://golang.org/cmd/", "http://golang.org/pkg/fmt/", "http://golang.org/pkg/os/"}},
				Page{distance: 1, body: "Commands", links: []string{"http://golang.org/", "http://golang.org/pkg/", "http://golang.org/cmd/gofmt/", "http://golang.org/cmd/pprof/"}}},
			""},
		{2, []string{"http://golang.org/", "http://golang.org/pkg/", "http://golang.org/cmd/", "http://golang.org/pkg/fmt/", "http://golang.org/pkg/os/", "http://golang.org/cmd/gofmt/", "http://golang.org/cmd/pprof/"},
			[]Page{Page{distance: 0, body: "The Go Programming Language", links: []string{"http://golang.org/pkg/", "http://golang.org/cmd/"}},
				Page{distance: 1, body: "Packages", links: []string{"http://golang.org/", "http://golang.org/cmd/", "http://golang.org/pkg/fmt/", "http://golang.org/pkg/os/"}},
				Page{distance: 1, body: "Commands", links: []string{"http://golang.org/", "http://golang.org/pkg/", "http://golang.org/cmd/gofmt/", "http://golang.org/cmd/pprof/"}},
				Page{distance: 2, body: "Package fmt", links: []string{"http://golang.org/", "http://golang.org/pkg/"}},
				Page{distance: 2, body: "Package os", links: []string{"http://golang.org/", "http://golang.org/pkg/"}},
				Page{distance: 2, body: "Command gofmt", links: []string{"http://golang.org/", "http://golang.org/cmd/"}},
				Page{distance: 2, body: "Command pprof", links: []string{"http://golang.org/", "http://golang.org/cmd/"}}},
			""},
	}
	// set upo the page url
	for _, test := range tests {
		for i, u := range test.url {
			test.expected[i].URL, _ = url.Parse(u)
		}
	}
	for _, test := range tests {
		s, _ := NewSpider("http://golang.org/")
		u, _ := url.Parse("http://golang.org/")
		s.Queue.Enqueue(Page{URL: u})
		s.maxDepth = test.depth
		err := s.crawl(tester)
		if test.expectedErr == "" && err != nil {
			t.Errorf("Expected error to be nil, got %q", err)
			continue
		}
		if test.expectedErr != "" {
			if err == nil {
				t.Errorf("Expected error to be %q, got none", test.expectedErr)
				continue
			}
			if err.Error() != test.expectedErr {
				t.Errorf("Expected error to be %q, got %q", test.expectedErr, err.Error())
			}
			continue
		}
		// eheck the crawled information
		if len(test.expected) != len(s.Pages) {
			t.Errorf("Expected %d pages to be retrieved, got %d", len(test.expected), len(s.Pages))
			continue
		}
		for _, page := range test.expected {
			// see if the url info exists
			var found bool
			for u, p := range s.Pages {
				if page.URL.String() == u {
					found = true
					if p.URL.String() != page.URL.String() {
						t.Errorf("Expected url to be %q, got %q", p.URL.String(), page.URL.String())
						// nothing else is expected to be valid
						continue
					}
					if p.distance != page.distance {
						t.Errorf("Expected distance to be %d, got %d", p.distance, page.distance)
					}
					if p.body != page.body {
						t.Errorf("Expected body to be %d, got %d", p.body, page.body)
					}
					if len(p.links) != len(page.links) {
						t.Errorf("Expected %d links, got %d", len(p.links), len(page.links))
						continue //nothing else is valid
					}
					// check to see all expected are in the result
					for _, link := range p.links {
						var exists bool
						for _, l := range page.links {
							if l == link {
								exists = true
								break
							}
						}
						if !exists {
							t.Errorf("Expected to find link %q, not found", link)
						}
					}
					break
				}
			}
			if !found {
				t.Errorf("Expected %q to exist in the results, not found", page.URL.String())
			}
		}
	}

}

// check Spider.skip()
func TestSkip(t *testing.T) {
	tests := []struct {
		s string
		*url.URL
		RestrictToScheme bool
		expected         bool
	}{
		{s: "http://golang.org/", RestrictToScheme: false, expected: true},
		{s: "https://golang.org/", RestrictToScheme: true, expected: true},
		{s: "http://golang.org/cmd/", RestrictToScheme: false, expected: false},
		{s: "https://golang.org/cmd/", RestrictToScheme: true, expected: true},
		{s: "http://golang.org/cmd/gofmt/", RestrictToScheme: false, expected: false},
		{s: "http://golang.org/cmd/gofmt/", RestrictToScheme: true, expected: false},
		{s: "https://golang.org/cmd/gofmt/", RestrictToScheme: false, expected: false},
		{s: "https://golang.org/cmd/gofmt/", RestrictToScheme: true, expected: true},
		{s: "http://golang.org/pkg/", RestrictToScheme: false, expected: true},
		{s: "https://golang.org/pkg/", RestrictToScheme: false, expected: true},
		{s: "http://golang.org/pkg/", RestrictToScheme: true, expected: true},
		{s: "https://golang.org/pkg/", RestrictToScheme: true, expected: true},
		{s: "http://google.com/", RestrictToScheme: false, expected: true},
		{s: "https://google.com/", RestrictToScheme: true, expected: true},
	}
	for i, test := range tests {
		test.URL, _ = url.Parse(test.s)
		tests[i] = test
	}
	s, _ := NewSpider("http://golang.org/cmd/")
	for _, test := range tests {
		s.RestrictToScheme = test.RestrictToScheme
		page := &Page{URL: test.URL}
		skip := s.skip(page.URL)
		if skip != test.expected {
			t.Errorf("Expected skip of %q to be %t, got %t", test.URL, test.expected, skip)
		}
	}
}

func TestFetchInterval(t *testing.T) {
	s, _ := NewSpider("http://golang.org/")
	u, _ := url.Parse("http://golang.org/")
	s.Queue.Enqueue(Page{URL: u})
	s.SetFetchInterval(200) // 200ms
	if s.fetchInterval != 200 {
		fmt.Errorf("Expected fetchInterval to be 200, got %d", s.fetchInterval)
	}
	if s.intervalJitter != 40 {
		fmt.Errorf("Expected intervalJitter to be 40, got %d", s.intervalJitter)
	}
	t1 := time.Now().UTC().UnixNano() / 1000000
	s.maxDepth = 1
	s.crawl(tester)
	t2 := time.Now().UTC().UnixNano() / 1000000
	// the time it took should be in the range of 3 * (fetchInterval) - 3 * (fetchInterval + intervalJitter)
	if t2-t1 < (3*s.fetchInterval) || t2-t1 > (3*(s.fetchInterval+s.intervalJitter)) {
		t.Errorf("Expected the fecth of 3 urls to take between %dms and %dms, it took %dms", (3 * s.fetchInterval), (3 * (s.fetchInterval + s.intervalJitter)), t2-t1)
	}
}

func TestExternalHosts(t *testing.T) {
	s, _ := NewSpider("http://golang.org")
	hosts := make([]string, 4, 4)
	hosts[0] = "twitter.com"
	hosts[1] = "google.com"
	hosts[2] = "github.com"
	hosts[3] = "linkedin.com"

	for _, v := range hosts {
		s.externalHosts[v] = struct{}{}
	}
	sort.Strings(hosts)
	h := s.ExternalHosts()
	if len(h) != len(hosts) {
		t.Errorf("Expected returned host list to have %d elements, contained %d", len(hosts), len(h))
		return
	}
	for i, v := range h {
		if v != hosts[i] {
			t.Errorf("Expected element %d in returned hosts list to be %q, got %q", i, hosts[i], v)
		}
	}

}
