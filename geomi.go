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
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mohae/utilitybelt/queue"
	"github.com/temoto/robotstxt.go"
	"golang.org/x/net/html"
)

// Defaults
var (
	DefaultFetchInterval  time.Duration = time.Second                                                                                            // default min. time between fetches
	DefaultJitter         time.Duration = time.Second                                                                                            // default max additional, random, fetch delay
	DefaultRobotUserAgent string        = "Googlebot (geomi)"                                                                                    // default user agent identifier for the bot.
	DefaultUserAgent      string        = "Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/41.0.2228.0 Safari/537.36" // the default user agent
)

// Fetcher is an interface that makes it easier to test. In the future, it may be
// useful for other purposes, but that would require exporting the method that uses
// it first.
type Fetcher interface {
	// Fetch returns the body of URL and
	// a slice of URLs found on that page
	Fetch(url string) (body string, r ResponseInfo, urls []string)
}

type Config struct {
	CheckExternalLinks bool          // Whether a HEAD should be performed on external links
	FetchInterval      time.Duration // The minimum time between fetching URLS
	Jitter             time.Duration // The max amount of jitter to add to the FetchInterval, the actual jitter is random.
	RespectRobots      bool          // Whether the robots.txt should be respected
	RestrictToScheme   bool          // Whether the crawl should be restricted to the base URL's scheme
	RobotUserAgent     string        // The user agent for the robot
	UserAgent          string        // The user agent to use.
}

// NewConfig returns a Config struct with Geomi defaults applied.
func NewConfig() *Config {
	return &Config{
		CheckExternalLinks: true,
		FetchInterval:      DefaultFetchInterval,
		Jitter:             DefaultJitter,
		RespectRobots:      true,
		RestrictToScheme:   false,
		RobotUserAgent:     DefaultRobotUserAgent,
		UserAgent:          DefaultUserAgent,
	}
}

// SetFetchInterval sets both the FetchInterval and Jitter to the passed value.
// Min FetchInterval will always be == t while the Max Fetchinterval will always be
// 2t, with most fetches being a random value between t and 2t.
func (c *Config) SetFetchInterval(t time.Duration) {
	c.FetchInterval = t
	c.Jitter = t
}

// a page is a url. This is usually some content wiht a number of elements, but it
// could be something as simple as the url for cat.jpg on your site and represent that
// image.
type Page struct {
	*url.URL
	distance int
	body     string
	links    []string // immediate children
}

// ResponseInfo contains the status and error information from a get
// TODO:
//	Add Expire date
//	Add time for request to return
//	Should the body be in here?
type ResponseInfo struct {
	Status     string
	StatusCode int
	Err        error
}

// Site is a type that implements fetcher
type Site struct {
	*url.URL
}

// Implements fetcher.
// TODO: make the design cleaner
func (s Site) Fetch(url string) (body string, r ResponseInfo, urls []string) {
	// see if the passed url is outside of the baseURL
	resp, err := http.Get(url)
	if err != nil {
		r.Err = err
		return "", r, nil
	}
	defer resp.Body.Close()
	r.Status = resp.Status
	r.StatusCode = resp.StatusCode
	buff := &bytes.Buffer{}
	tee := io.TeeReader(resp.Body, buff)
	tokens := getTokens(tee)
	if len(tokens) == 0 {
		r.Err = fmt.Errorf("%s: nothing in body", url)
		return "", r, nil
	}
	urls, err = s.linksFromTokens(tokens)
	if err != nil {
		r.Err = err
		return "", r, nil
	}
	return buff.String(), r, urls
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
	wg            sync.WaitGroup
	*url.URL      // the start url
	Config        *Config
	robots        *robotstxt.Group
	maxDepth      int
	Pages         map[string]Page
	foundURLs     map[string]struct{}     // keeps track of urls found to prevent recrawling
	fetchedURLs   map[string]ResponseInfo // urls that have been fetched with their status
	skippedURLs   map[string]struct{}     // urls within the same domain that are not retrieved
	externalHosts map[string]struct{}     // list of external hosts TODO: elide?
	externalLinks map[string]ResponseInfo // list of external links; if fetched,
}

// returns a Spider with the its site's baseUrl set. The baseUrl is the start point for
// the crawl. It is also the restriction on the crawl:
func NewSpider(start string) (*Spider, error) {
	if start == "" {
		return nil, errors.New("newSpider: the start url cannot be empty")
	}
	var err error
	spider := &Spider{
		Queue:         queue.New(128, 0),
		Config:        NewConfig(),
		Pages:         make(map[string]Page),
		foundURLs:     make(map[string]struct{}),
		fetchedURLs:   make(map[string]ResponseInfo),
		skippedURLs:   make(map[string]struct{}),
		externalHosts: make(map[string]struct{}),
		externalLinks: make(map[string]ResponseInfo),
	}
	spider.URL, err = url.Parse(start)
	if err != nil {
		return nil, err
	}
	return spider, nil
}

// ExternalHosts returns a sorted list of external hosts
func (s *Spider) ExternalHosts() []string {
	hosts := make([]string, len(s.externalHosts), len(s.externalHosts))
	i := 0
	for k, _ := range s.externalHosts {
		hosts[i] = k
		i++
	}
	sort.Strings(hosts)
	return hosts
}

// ExternalLinks returns a sorted list of external Links
func (s *Spider) ExternalLinks() []string {
	links := make([]string, len(s.externalLinks), len(s.externalLinks))
	i := 0
	for k, _ := range s.externalLinks {
		links[i] = k
		i++
	}
	sort.Strings(links)
	return links
}

// Crawl is the exposed method for starting a crawl at baseURL. The crawl private method
// does the actual work. The depth is the maximum depth, or distance, to crawl from the
// baseURL. If depth == -1, no limits are set and it is expected that the entire site
// will be crawled.
func (s *Spider) Crawl(depth int) (message string, err error) {
	s.maxDepth = depth
	S := Site{URL: s.URL}
	// if we are to respect the robots.txt, set up the info
	if s.Config.RespectRobots {
		s.getRobotsTxt()
	}
	s.Queue.Enqueue(Page{URL: s.URL})
	err = s.crawl(S)
	return fmt.Sprintf("%d nodes were processed; %d external links linking to %d external hosts were not processed", len(s.Pages), len(s.externalLinks), len(s.externalHosts)), err
}

// This crawl does all the work.
func (s *Spider) crawl(fetcher Fetcher) error {
	for !s.Queue.IsEmpty() {
		// get next item from queue
		page := s.Queue.Dequeue().(Page)
		// if a depth value was passed and the distance is > depth, we are done
		// depth of 0 means no limit
		if s.maxDepth != -1 && page.distance > s.maxDepth {
			return nil
		}
		// see if this is an external url
		if s.externalURL(page.URL) {
			if s.Config.CheckExternalLinks {
				s.fetchExternalLink(page.URL)
			}
			continue
		}
		// check to see if this url should be skipped for other reasons
		if s.skip(page.URL) {
			// see if the skipped is external and process accordingly
			continue
		}
		s.foundURLs[page.URL.String()] = struct{}{}
		// get the url
		r := ResponseInfo{}
		page.body, r, page.links = fetcher.Fetch(page.URL.String())
		// add the page and status to the map. map isn't checked for membership becuase we don't
		// fetch found urls.
		s.Lock()
		s.Pages[page.URL.String()] = page
		s.fetchedURLs[page.URL.String()] = r
		s.Unlock()
		// add the urls that the node contains to the queue
		for _, l := range page.links {
			u, _ := url.Parse(l)
			s.Queue.Enqueue(Page{URL: u, distance: page.distance + 1})
		}
		// if there is a wait between fetches, sleep for that + random jitter
		if s.Config.FetchInterval > 0 {
			wait := s.Config.FetchInterval
			// if there is a value for jitter, add a random jitter
			if s.Config.Jitter > 0 {
				n := s.Config.Jitter.Nanoseconds()
				n = rand.Int63n(n)
				wait += time.Duration(n) * time.Nanosecond
			}
			time.Sleep(wait)
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
	_, ok := s.foundURLs[u.String()]
	s.Unlock()
	if ok { // if it was found, skip it
		s.addSkippedURL(u)
		return true
	}

	// skip if we are restricted to current scheme
	if s.Config.RestrictToScheme {
		if u.Scheme != s.URL.Scheme {
			s.addSkippedURL(u)
			return true
		}
	}
	if s.Config.RespectRobots {
		ok := s.robotsAllowed(u)
		if !ok {
			return false
		}
	}
	// skip if the url is outside of base
	// remove the scheme + schemePrefix so just the rest of the url is being compared
	if !strings.HasPrefix(u.Path, s.URL.Path) {
		s.addSkippedURL(u)
		return true
	}
	return false
}

// addSkippedURL add's the url info too the skipped info
func (s *Spider) addSkippedURL(u *url.URL) {
	s.Lock()
	s.skippedURLs[u.Path] = struct{}{}
	s.Unlock()
}

// externalURL check's to see if the url is external to the site, host isn't the same.
// and add's that info to the ext structs.
func (s *Spider) externalURL(u *url.URL) bool {
	if u.Host != s.URL.Host {
		s.Lock()
		// see if the host is already in the map
		_, ok := s.externalHosts[u.Host]
		if !ok {
			s.externalHosts[u.Host] = struct{}{}
		}
		// same with url
		_, ok = s.externalLinks[u.String()]
		if !ok {
			s.externalLinks[u.String()] = ResponseInfo{}
		}
		s.Unlock()
		return true
	}
	return false
}

// fetchExternalLink: fetches an external link's HEAD and check's it status. Note, this
// does not implement fetcher
func (s *Spider) fetchExternalLink(u *url.URL) error {
	// if this has already benn fetched, don't
	var ri ResponseInfo
	s.Lock()
	r, _ := s.externalLinks[u.String()]
	s.Unlock()
	if r != ri { // if !0 value, it's been retrieved
		return nil
	}
	resp, err := http.Head(u.String())
	if err != nil {
		r.Err = err
		s.Lock()
		s.externalLinks[u.String()] = r
		s.Unlock()
		return err
	}
	r.Status = resp.Status
	r.StatusCode = resp.StatusCode
	s.Lock()
	s.externalLinks[u.String()] = r
	s.Unlock()
	return nil
}

// getRobotsTxt retrieves and processes the site's robot.txt. If the robots.txt doesn't
// exist, it is assumed that everything is allowed.
func (s *Spider) getRobotsTxt() error {
	resp, err := http.Get("http://" + s.URL.Host)
	if err != nil {
		return err
	}
	robots, err := robotstxt.FromResponse(resp)
	if err != nil {
		return err
	}
	s.robots = robots.FindGroup(s.Config.RobotUserAgent)
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

func init() {
	// We just use math/rand because it's good enough for our purpose.
	rand.Seed(time.Now().UTC().UnixNano())
}
