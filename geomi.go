// geomi, 거미, is a basic spider for crawling specific webs. It was designed to crawl
// either a site, or a subset of a site, indexing the pages and where they link to.
// Any links that go outside of the provided base URL are noted but not followed.
//
//
// Even though geomi was designed with the intended use being crawling ones own site,
// or a client's site, it does come with some basic behavior configuration to make it
// a friendly bot:
//   * respects ROBOTS.txt TODO
//   * configurable concurrent walkers TODO
//   * configurable wait interval range TODO
//   * configurable max requests per: TODO
//     * min
//     * hour
//     * day
//     * month
//
// To start, go get the geomi package:
//    go get github.com/mohae/geomi
//
// Import it into your code:
//    import github.com/mohae/geomi
//
// Set up the site for the spider:
//   s := geomi.NewSite("http://example.com")
//
// The url that is passed to the NewSite() function is the base case, after which
// a BFS traversal is done.
//
package geomi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/mohae/utilitybelt/queue"
	"github.com/temoto/robotstxt-go"
	"golang.org/x/net/html"
)

// Fetcher is an interface that makes it easier to test. In the future, it may be
// useful for other purposes, but that would require exporting the method that uses
// it first.
type Fetcher interface {
	// Fetch returns the body of URL and
	// a slice of URLs found on that page
	Fetch(url string) (body string, urls []string, err error)
}

//}
// fetched tracks URLs that have been (or are being) fetched.
// The lock must be held while reading from or writing to the map.
var fetched = struct {
	m map[string]error
	sync.Mutex
}{m: make(map[string]error)}

var loading = errors.New("url load in progress") // sentinel value

// a page is a url. This is usually some content wiht a number of elements, but it
// could be something as simple as the url for cat.jpg on your site and represent that
// image.
type Page struct {
	*url.URL
	distance int
	body     string
	links    []string // immediate children
}

// Site is a type that implements fetcher
type Site struct {
	*url.URL
}

// Implements fetcher. This seems wierd to me, but the design (including the crawl
// function) enables testing.
// TODO: make the design cleaner
func (s Site) Fetch(url string) (body string, urls []string, err error) {
	// see if the passed url is outside of the baseURL
	resp, err := http.Get(url)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	buff := &bytes.Buffer{}
	tee := io.TeeReader(resp.Body, buff)
	tokens := getTokens(tee)
	if len(tokens) == 0 {
		return "", nil, fmt.Errorf("%s: nothing in body", url)
	}
	urls, err = s.linksFromTokens(tokens)
	if err != nil {
		return "", nil, err
	}
	return buff.String(), urls, nil
}

// linksFromTokens returns a list of links (href a) found in the token slice
// TODO should internal links be tracked separatly? i.e. record them in a
// separate var (so they don't get fetched)
func (s *Site) linksFromTokens(tokens []html.Token) ([]string, error) {
	var links []string
	for _, token := range tokens {
		if token.Type == html.StartTagToken && token.DataAtom.String() == "a" {
			for _, attr := range token.Attr {
				// We only care about links that aren't #
				if attr.Key == "href" && !strings.HasPrefix(attr.Val, "#") {
					// parse the url
					url, err := url.Parse(attr.Val)
					if err != nil {
						return nil, err
					}
					link := s.URL.ResolveReference(url)
					links = append(links, link.String())
				}
			}
		}
	}
	return links, nil
}

// Spider crawls the target. It contains all information needed to manage the crawl
// including keeping track of work to do, work done, and the results.
type Spider struct {
	*queue.Queue
	sync.Mutex
	wg               sync.WaitGroup
	*url.URL                // the start url
	UserAgent        string // the user agent to use
	concurrency      int    // concurrency level of crawling
	getWait          int64  // if > 0, interval, in milliseconds to wait between gets
	getJitter        int    // prevents thundering herd or fetcher synchronization
	RestrictToScheme bool   // if true, only crawls urls with the same scheme as baseURL
	RespectRobots    bool   // whether or not to respect the robots.txt
	robots           *robotstxt.Group
	maxDepth         int
	Pages            map[string]Page
	foundURLs        map[string]struct{} // keeps track of urls found to prevent recrawling
	fetchedURLs      map[string]error    // urls that have been fetched with their status
	skippedURLs      map[string]struct{} // urls within the same domain that are not retrieved
	extHosts         map[string]struct{} // list of external hosts TODO: elide?
	extLinks         map[string]struct{} // list of external links; not fetched
}

// returns a Spider with the its site's baseUrl set. The baseUrl is the start point for
// the crawl. It is also the restriction on the crawl:
func NewSpider(start string) (*Spider, error) {
	if start == "" {
		return nil, errors.New("newSpider: the start url cannot be empty")
	}
	var err error
	spider := &Spider{Queue: queue.New(128, 0),
		UserAgent:     "geomi",
		RespectRobots: true,
		Pages:         make(map[string]Page),
		foundURLs:     make(map[string]struct{}),
		fetchedURLs:   make(map[string]error),
		skippedURLs:   make(map[string]struct{}),
		extHosts:      make(map[string]struct{}),
		extLinks:      make(map[string]struct{}),
	}
	spider.URL, err = url.Parse(start)
	if err != nil {
		return nil, err
	}
	return spider, nil
}

// Crawl is the exposed method for starting a crawl at baseURL. The crawl private method
// does the actual work. The depth is the maximum depth, or distance, to crawl from the
// baseURL. If depth == -1, no limits are set and it is expected that the entire site
// will be crawled.
func (s *Spider) Crawl(depth int) (message string, err error) {
	s.maxDepth = depth
	S := Site{URL: s.URL}
	// if we are to respect the robots.txt, set up the info
	if s.RespectRobots {
		s.getRobotsTxt()
	}
	s.Queue.Enqueue(Page{URL: s.URL})
	err = s.crawl(S)
	return fmt.Sprintf("%d nodes were processed; %d external links linking to %d external hosts were not processed", len(s.Pages), len(s.extLinks), len(s.extHosts)), err
}

// This crawl does all the work.
func (s *Spider) crawl(fetcher Fetcher) error {
	var err error
	for !s.Queue.IsEmpty() {
		// get next item from queue
		page := s.Queue.Dequeue().(Page)
		// if a depth value was passed and the distance is > depth, we are done
		// depth of 0 means no limit
		if s.maxDepth != -1 && page.distance > s.maxDepth {
			return nil
		}
		// check to see if this url should be skipped
		if s.skip(page.URL) {
			s.skippedURLs[page.URL.String()] = struct{}{}
			continue
		}
		s.foundURLs[page.URL.String()] = struct{}{}
		// get the url
		page.body, page.links, err = fetcher.Fetch(page.URL.String())
		// add the page and status to the map. map isn't checked for membership becuase we don't
		// fetch found urls.
		s.Lock()
		s.Pages[page.URL.String()] = page
		s.fetchedURLs[page.URL.String()] = err
		s.Unlock()
		// add the urls that the node contains to the queue
		for _, l := range page.links {
			u, _ := url.Parse(l)
			s.Queue.Enqueue(Page{URL: u, distance: page.distance + 1})
		}
	}
	return nil
}

// skip determines whether the url should be skipped.
//   * skip urls that have already been fetched
//   * skip urls that are outside of the basePath
//   * conditionally skip urls that are outside of the current scheme, even if
//     they are within the current pasePath
//   * skip if not allowed by robots
func (s *Spider) skip(u *url.URL) bool {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.foundURLs[u.String()]; ok {
		return true
	}
	// skip if we are restricted to current scheme
	if s.RestrictToScheme {
		if u.Scheme != s.URL.Scheme {
			return true
		}
	}
	//	if s.RespectRobots {
	//		ok := s.robotsAllowed(p.URL)
	//	}
	// skip if the url is outside of base
	// remove the scheme + schemePrefix so just the rest of the url is being compared
	if !strings.HasPrefix(u.Path, s.URL.Path) {
		return true
	}
	return false
}

// getRobotsTxt retrieves and processes the site's robot.txt. If the robots.txt doesn't
// exist, it is assumed that everything is allowed.
func (s *Spider) getRobotsTxt() error {
	// spider url is
	return nil
}

// robotsAllowed checks to see if the passed path is allowed by Robots.txt. If the
// robots isn't set, it's always true
func (s *Spider) robotsAllowed(u *url.URL) bool {
	if s.robots != nil {
		return s.robots.Test(u.Path)
	}
	return true
}

// getTokens returns all tokens in the body
func getTokens(body io.Reader) []html.Token {
	tokens := make([]html.Token, 0)
	page := html.NewTokenizer(body)
	for {
		typ := page.Next()
		if typ == html.ErrorToken {
			return tokens
		}
		tokens = append(tokens, page.Token())
	}
}
