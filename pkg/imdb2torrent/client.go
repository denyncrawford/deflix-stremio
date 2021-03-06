package imdb2torrent

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/doingodswork/deflix-stremio/pkg/cinemata"
	log "github.com/sirupsen/logrus"
)

var (
	magnet2InfoHashRegex = regexp.MustCompile(`btih:.+?&`)     // The "?" makes the ".+" non-greedy
	regexMagnet          = regexp.MustCompile(`'magnet:?.+?'`) // The "?" makes the ".+" non-greedy
)

type MagnetSearcher interface {
	Check(ctx context.Context, imdbID string) ([]Result, error)
}

type Client struct {
	timeout     time.Duration
	ytsClient   ytsClient
	tpbClient   tpbClient
	leetxClient leetxClient
	ibitClient  ibitClient
	tpbRetries  int
}

func NewClient(ctx context.Context, baseURLyts, baseURLtpb, baseURL1337x, baseURLibit string, socksProxyAddrTPB string, timeout time.Duration, tpbRetries int, torrentCache *fastcache.Cache, cinemataCache *fastcache.Cache, cacheAge time.Duration) (Client, error) {
	cinemataClient := cinemata.NewClient(ctx, timeout, cinemataCache)
	tpbClient, err := newTPBclient(ctx, baseURLtpb, socksProxyAddrTPB, timeout, torrentCache, cacheAge)
	if err != nil {
		return Client{}, fmt.Errorf("Couldn't create TPB client: %v", err)
	}
	return Client{
		timeout:     timeout,
		ytsClient:   newYTSclient(ctx, baseURLyts, timeout, torrentCache, cacheAge),
		tpbClient:   tpbClient,
		leetxClient: newLeetxclient(ctx, baseURL1337x, timeout, torrentCache, cinemataClient, cacheAge),
		ibitClient:  newIbitClient(ctx, baseURLibit, timeout, torrentCache, cacheAge),
		tpbRetries:  tpbRetries,
	}, nil
}

// FindMagnets tries to find magnet URLs for the given IMDb ID.
// It only returns 720p, 1080p, 1080p 10bit, 2160p and 2160p 10bit videos.
// It caches results once they're found.
// It can return an empty slice and no error if no actual error occurred (for example if torrents where found but no >=720p videos).
func (c Client) FindMagnets(ctx context.Context, imdbID string) ([]Result, error) {
	logger := log.WithContext(ctx).WithField("imdbID", imdbID)

	torrentSiteCount := 3
	resChan := make(chan []Result, torrentSiteCount)
	errChan := make(chan error, torrentSiteCount)

	// YTS
	go func() {
		logger.WithField("torrentSite", "YTS").Debug("Started searching torrents...")
		results, err := c.ytsClient.Check(ctx, imdbID)
		if err != nil {
			logger.WithError(err).WithField("torrentSite", "YTS").Warn("Couldn't find torrents")
			errChan <- err
		} else {
			fields := log.Fields{
				"torrentSite":  "YTS",
				"torrentCount": len(results),
			}
			logger.WithFields(fields).Debug("Found torrents")
			resChan <- results
		}
	}()

	// TPB
	go func() {
		logger.WithField("torrentSite", "TPB").Debug("Started searching torrents...")
		results, err := c.tpbClient.checkAttempts(ctx, imdbID, 1+c.tpbRetries)
		if err != nil {
			logger.WithError(err).WithField("torrentSite", "TPB").Warn("Couldn't find torrents")
			errChan <- err
		} else {
			fields := log.Fields{
				"torrentSite":  "TPB",
				"torrentCount": len(results),
			}
			logger.WithFields(fields).Debug("Found torrents")
			resChan <- results
		}
	}()

	// 1337x
	go func() {
		logger.WithField("torrentSite", "1337x").Debug("Started searching torrents...")
		results, err := c.leetxClient.Check(ctx, imdbID)
		if err != nil {
			logger.WithError(err).WithField("torrentSite", "1337x").Warn("Couldn't find torrents")
			errChan <- err
		} else {
			fields := log.Fields{
				"torrentSite":  "1337x",
				"torrentCount": len(results),
			}
			logger.WithFields(fields).Debug("Found torrents")
			resChan <- results
		}
	}()

	// ibit
	// Note: An initial movie search takes long, because multiple requests need to be made, but ibit uses rate limiting, so we can't do them concurrently.
	// So let's treat this special: Make the request, but only wait for 1 second (in case the cache is filled), then don't cancel the operation, but let it run in the background so the cache gets filled.
	// With the next movie search for the same IMDb ID the cache is used.
	ibitResChan := make(chan []Result)
	ibitErrChan := make(chan error)
	go func() {
		logger.WithField("torrentSite", "ibit").Debug("Started searching torrents...")
		ibitResults, err := c.ibitClient.Check(ctx, imdbID)
		if err != nil {
			logger.WithError(err).WithField("torrentSite", "ibit").Warn("Couldn't find torrents")
			ibitErrChan <- err
		} else {
			fields := log.Fields{
				"torrentSite":  "ibit",
				"torrentCount": len(ibitResults),
			}
			logger.WithFields(fields).Debug("Found torrents")
			ibitResChan <- ibitResults
		}
	}()

	// Collect results from all except ibit.
	var combinedResults []Result
	var errs []error
	dupRemovalRequired := false
	for i := 0; i < torrentSiteCount; i++ {
		// No timeout for the goroutines because their HTTP client has a timeout already
		select {
		case err := <-errChan:
			errs = append(errs, err)
		case results := <-resChan:
			if !dupRemovalRequired && len(combinedResults) > 0 && len(results) > 0 {
				dupRemovalRequired = true
			}
			combinedResults = append(combinedResults, results...)
		}
	}
	close(resChan)
	close(errChan)

	returnErrors := len(errs) == torrentSiteCount

	// Now collect result from ibit if it's there.
	var closeChansOk bool
	select {
	case err := <-ibitErrChan:
		errs = append(errs, err)
		closeChansOk = true
	case results := <-ibitResChan:
		if !dupRemovalRequired && len(combinedResults) > 0 && len(results) > 0 {
			dupRemovalRequired = true
		}
		combinedResults = append(combinedResults, results...)
		returnErrors = false
		closeChansOk = true
	case <-time.After(1 * time.Second):
		logger.WithField("torrentSite", "ibit").Info("torrent search hasn't finished yet, we'll let it run in the background")
	}
	if closeChansOk {
		close(ibitErrChan)
		close(ibitResChan)
	}

	// Return error (only) if all torrent sites returned actual errors (and not just empty results)
	if returnErrors {
		errsMsg := "Couldn't find torrents on any site: "
		for i := 1; i <= torrentSiteCount; i++ {
			errsMsg += fmt.Sprintf("%v.: %v; ", i, errs[i-1])
		}
		errsMsg = strings.TrimSuffix(errsMsg, "; ")
		return nil, fmt.Errorf(errsMsg)
	}

	// Remove duplicates.
	// Only necessary if we got non-empty results from more than one torrent site.
	var noDupResults []Result
	if dupRemovalRequired {
		infoHashes := map[string]struct{}{}
		for _, result := range combinedResults {
			if _, ok := infoHashes[result.InfoHash]; !ok {
				noDupResults = append(noDupResults, result)
				infoHashes[result.InfoHash] = struct{}{}
			}
		}
	} else {
		noDupResults = combinedResults
	}

	if len(noDupResults) == 0 {
		logger.Warn("Couldn't find ANY torrents")
	}

	return noDupResults, nil
}

func (c Client) GetMagnetSearchers() map[string]MagnetSearcher {
	return map[string]MagnetSearcher{
		"YTS":   c.ytsClient,
		"TPB":   c.tpbClient,
		"1337x": c.leetxClient,
		"ibit":  c.ibitClient,
	}
}

type Result struct {
	Title string
	// For example "720p" or "720p (web)"
	Quality   string
	InfoHash  string
	MagnetURL string
}

func replaceURL(origURL, newBaseURL string) (string, error) {
	// Replace by configured URL, which could be a proxy that we want to go through
	url, err := url.Parse(origURL)
	if err != nil {
		return "", fmt.Errorf("Couldn't parse URL. URL: %v; error: %v", origURL, err)
	}
	origBaseURL := url.Scheme + "://" + url.Host
	return strings.Replace(origURL, origBaseURL, newBaseURL, 1), nil
}
