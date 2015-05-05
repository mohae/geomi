# geomi
거미: Korean for spider, geomi is a spider that restricts itself to the provided web.

__In progress__

Geomi currently will crawl all linked children of the provided URL that are not external sites. Any found links that are part of the domain being crawled but whose path is outside of the provided URL for the root node will not be crawled, e.g. for a start point of `http://golang.org/cmd/`, if a link to `http://golang.org/pkg/` is found, it will not be crawled as its path is not within `http://golang.org/comd/`.

Geomi currently supports waiting between fetchs and respecting the site's `robot.txt`, if it exists. Currently, geomi does not support concurrent fetchers for crawling a site. At the moment, all crawling is done by 1 process.

## About
The purpose of geomi is to provide a package that crawls a specific site or subset of a site, indexing its links and content. This is accomplished by creating a spider with a base url, including scheme. 

The base url is the start point from which the spider will start crawling. Any links that are either external to the site, or outside of the baseURL, will be indexed but not crawled by geomi.

Any node that geomi crawls will have its response body saved, along with all links on the page. The distance of the node from the base url will also be recorded.

The depth to which geomi will crawl is configurable. If there are no limits, the spider should be passed a depth value of `-1`. This will result in all children of the base url that have links to be indexed.

The amount of time geomi should wait between fetches is configurable. By default, geomi does not wait between fetches. To set an amount of time geomi should wait after fetching a url before fetching another, use Spider.SetFetchInterval(n), where n is an int64 integer representing the amount of time in milliseconds that geomi should wait. Geomi also adds a random amount of jitter to the wait with a maximum additional wait time equal to 20% of the passed fetch interval value, e.g. setting the fetch interval to 1000ms (1 second) will result in a random additional wait of 0-200ms, so the max wait between fetches would be 1200ms (1.2 seconds). This is mainly for concurrent fetching situations to prevent a thundering herd. Currently, geomi does not support concurrent fetchers.

Geomi will respect the a site's `robot.txt` unless it is explicitely told not to.

Geomi tracks what URLs have been fetched, the error code, if any, the content body, and any non `#` links found in the body.

For an example of an implementation, see [kraul](https://github.com/mohae/kraul). It's implementation may not be totally up to date, but I do my best to keep it current. Kraul may not use all of geomi's functionality.

## Usage
TODO

* add support for recording how long the response took.
* record crawl time for historical purposes
* add support for getting information out of the spider. Supplying a custom fetcher may be the route taken instead. Not sure as I haven't yet pondered this. In general, support needs to be added for making the information that the spider gathers useful. __In Process__
* add concurrent fetching support with a configurable limit on the number of concurrent fetchers. The fetchInterval and jitter functionality that geomi currenlty has was done with concurrency in mind.

## Possible functionality
This is a list of functionality that may be added to geomi, but not guaranteed. This list is in addition to the core functionality that geomi would have once it is completed.

* optional support for retrieving links outside of base. If enabled, geomi would retrieve the otherwise excluded link and save both the response body and response code. This would enable detection of changes to linked content, content that has moved, and dead links. Links on the retrieved page would not be extracted and geomi would not do additional crawling from the node in question.

## Licensing
Copyright 2015 by Joel Scoble, rights reserved.
Geomi is provided under the MIT license with no additional warranty or support provided, implicitly or explicitly. For more information, please refer to the included LICENSE file.
