package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Flags
var (
	bindAddr      = flag.String("bindAddr", "localhost", `Local interface address to bind to. "localhost" only allows access from the local host. "0.0.0.0" binds to all network interfaces.`)
	port          = flag.Int("port", 8080, "Port to listen on")
	streamURLaddr = flag.String("streamURLaddr", "http://localhost:8080", "Address to be used in a stream URL that's delivered to Stremio and later used to redirect to RealDebrid")
	cachePath     = flag.String("cachePath", "", "Path for loading a persisted cache on startup and persisting the current cache in regular intervals. An empty value will lead to `os.UserCacheDir()+\"/deflix-stremio/\"`")
	// 128*1024*1024 Byte = 128 MB
	// We split these on 4 caches à 32 MB
	// Note: fastcache uses 32 MB as minimum, that's why we use `4*32 MB = 128 MB` as minimum.
	cacheMaxBytes = flag.Int("cacheMaxBytes", 128*1024*1024, "Max number of bytes to be used for the in-memory cache. Default (and minimum!) is 128 MB.")
	baseURLyts    = flag.String("baseURLyts", "https://yts.mx", "Base URL for YTS")
	baseURLtpb    = flag.String("baseURLtpb", "https://thepiratebay.org", "Base URL for TPB")
	baseURL1337x  = flag.String("baseURL1337x", "https://1337x.to", "Base URL for 1337x")
	baseURLibit   = flag.String("baseURLibit", "https://ibit.am", "Base URL for ibit")
	logLevel      = flag.String("logLevel", "debug", `Log level to show only logs with the given and more severe levels. Can be "trace", "debug", "info", "warn", "error", "fatal", "panic"`)
	rootURL       = flag.String("rootURL", "https://www.deflix.tv", "Redirect target for the root")
	envPrefix     = flag.String("envPrefix", "", "Prefix for environment variables")
)

func parseConfig(ctx context.Context) {
	flag.Parse()

	if *envPrefix != "" && !strings.HasSuffix(*envPrefix, "_") {
		*envPrefix += "_"
	}

	// Only overwrite the values by their env var counterparts that have not been set (and that *are* set via env var).
	var err error
	if !isArgSet(ctx, "bindAddr") {
		if val, ok := os.LookupEnv(*envPrefix + "BIND_ADDR"); ok {
			*bindAddr = val
		}
	}
	if !isArgSet(ctx, "port") {
		if val, ok := os.LookupEnv(*envPrefix + "PORT"); ok {
			if *port, err = strconv.Atoi(val); err != nil {
				log.WithError(err).WithField("envVar", "PORT").Fatal("Couldn't convert environment variable from string to int")
			}
		}
	}
	if !isArgSet(ctx, "streamURLaddr") {
		if val, ok := os.LookupEnv(*envPrefix + "STREAM_URL_ADDR"); ok {
			*streamURLaddr = val
		}
	}
	if !isArgSet(ctx, "cachePath") {
		if val, ok := os.LookupEnv(*envPrefix + "CACHE_PATH"); ok {
			*cachePath = val
		}
	}
	if !isArgSet(ctx, "cacheMaxBytes") {
		if val, ok := os.LookupEnv(*envPrefix + "CACHE_MAX_BYTES"); ok {
			if *cacheMaxBytes, err = strconv.Atoi(val); err != nil {
				log.WithError(err).WithField("envVar", "CACHE_MAX_BYTES").Fatal("Couldn't convert environment variable from string to int")
			}
		}
	}
	if !isArgSet(ctx, "baseURLyts") {
		if val, ok := os.LookupEnv(*envPrefix + "BASE_URL_YTS"); ok {
			*baseURLyts = val
		}
	}
	if !isArgSet(ctx, "baseURLtpb") {
		if val, ok := os.LookupEnv(*envPrefix + "BASE_URL_TPB"); ok {
			*baseURLtpb = val
		}
	}
	if !isArgSet(ctx, "baseURL1337x") {
		if val, ok := os.LookupEnv(*envPrefix + "BASE_URL_1337X"); ok {
			*baseURL1337x = val
		}
	}
	if !isArgSet(ctx, "logLevel") {
		if val, ok := os.LookupEnv(*envPrefix + "LOG_LEVEL"); ok {
			*logLevel = val
		}
	}
	if !isArgSet(ctx, "rootURL") {
		if val, ok := os.LookupEnv(*envPrefix + "ROOT_URL"); ok {
			*rootURL = val
		}
	}
}

// isArgSet returns true if the argument you're looking for is actually set as command line argument.
// Pass without "-" prefix.
func isArgSet(ctx context.Context, arg string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == arg {
			found = true
		}
	})
	return found
}
