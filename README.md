# geomi
거미: Korean for spider, geomi is a spider that restricts itself to the provided web.

__In progress__

Currently, the spider will crawl any links it finds. This will be modified in future versions.

## About
The purpose of geomi is to prived a package that crawls a specific site or subset of a site, indexing its links and content. This is accomplished by creating a spider with a base url, including scheme.

The base url is the start point from which the spider will start crawling. Any links that are either external to the site, or outside of the baseURL, will be indexed but not crawled by geomi.

Any node that geomi crawls will have its response body saved, along with all links on the page. The distance of the node from the base url will also be recorded.

The depth to which geomi will crawl is configurable. If there are no limits, the spider should be passed a depth value of `-1`. This will result in all children of the base url that have links to be indexed.

## Usage
TODO


## Possible functionality
This is a list of functionality that may be added to geomi, but not guaranteed. This list is in addition to the core functionality that geomi would have once it is completed.

* optional support for alternate scheme. Since geomi restricts itself to the set base url, urls in other schemes, e.g. `https` vs `http` will not be crawled. Enabling support for using an alternate scheme would enable geomi to crawl both `http` and `https` urls. These are the only schemes that would be supported.
* optional support for retrieving links outside of base. If enabled, geomi would retrieve the otherwise excluded link and save both the response body and response code. This would enable detection of changes to linked content, content that has moved, and dead links. Links on the retrieved page would not be extracted and geomi would not do additional crawling from the node in question.

## Licensing
Copyright 2015 by Joel Scoble, rights reserved.
Geomi is provided under the MIT license with no additional warranty or support provided, implicitly or explicitly. For more information, please refer to the included LICENSE file.
