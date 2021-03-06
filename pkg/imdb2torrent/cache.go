package imdb2torrent

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"time"
)

type cacheEntry struct {
	Created time.Time
	Results []Result
}

// NewCacheEntry turns data into a single cacheEntry and returns the cacheEntry's gob-encoded bytes.
func NewCacheEntry(ctx context.Context, data []Result) ([]byte, error) {
	entry := cacheEntry{
		Created: time.Now(),
		Results: data,
	}
	writer := bytes.Buffer{}
	encoder := gob.NewEncoder(&writer)
	if err := encoder.Encode(entry); err != nil {
		return nil, fmt.Errorf("Couldn't encode cacheEntry: %v", err)
	}
	return writer.Bytes(), nil
}

// FromCacheEntry turns data via gob-decoding into a cacheEntry and returns its results and creation time.
func FromCacheEntry(ctx context.Context, data []byte) ([]Result, time.Time, error) {
	reader := bytes.NewReader(data)
	decoder := gob.NewDecoder(reader)
	var entry cacheEntry
	if err := decoder.Decode(&entry); err != nil {
		return nil, time.Time{}, fmt.Errorf("Couldn't decode cacheEntry: %v", err)
	}
	return entry.Results, entry.Created, nil
}
