package builder

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/yangtfu/podsync/pkg/feed"
	"github.com/yangtfu/podsync/pkg/model"
)

const (
	rssFetchTimeout = 90 * time.Second
	rssMaxBodySize  = 32 << 20 // 32 MiB, plenty for any podcast feed
)

// RSSBuilder fetches an existing podcast RSS feed and turns it into a podsync
// feed. Unlike the other builders it does not download media: episodes keep
// their original enclosure URL and only the feed XML is hosted locally.
type RSSBuilder struct {
	client *http.Client
}

func NewRSSBuilder() (*RSSBuilder, error) {
	return &RSSBuilder{
		client: &http.Client{Timeout: rssFetchTimeout},
	}, nil
}

func (b *RSSBuilder) Build(ctx context.Context, cfg *feed.Config) (*model.Feed, error) {
	body, err := b.fetch(ctx, cfg.URL)
	if err != nil {
		return nil, err
	}

	var doc rssDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, errors.Wrapf(err, "failed to parse RSS feed: %s", cfg.URL)
	}

	channel := doc.Channel
	if channel.Title == "" && len(channel.Items) == 0 {
		return nil, errors.Errorf("not an RSS feed: %s", cfg.URL)
	}

	now := time.Now().UTC()
	coverArt := channel.ITunesImage.Href
	if coverArt == "" {
		coverArt = channel.Image.URL
	}
	author := channel.ITunesAuthor
	if author == "" {
		author = channel.Author
	}
	if author == "" {
		author = channel.ManagingEditor
	}
	if author == "" {
		author = channel.Title
	}

	itemURL := channel.Link
	if itemURL == "" {
		itemURL = cfg.URL
	}

	pubDate := parseRSSDate(channel.PubDate)
	if pubDate.IsZero() {
		pubDate = parseRSSDate(channel.LastBuildDate)
	}
	if pubDate.IsZero() {
		pubDate = now
	}

	result := &model.Feed{
		ItemID:      cfg.URL,
		Provider:    model.ProviderRSS,
		LinkType:    model.TypeFeed,
		Format:      cfg.Format,
		Quality:     cfg.Quality,
		PageSize:    cfg.PageSize,
		Title:       strings.TrimSpace(channel.Title),
		Description: strings.TrimSpace(channel.Description),
		CoverArt:    coverArt,
		Author:      strings.TrimSpace(author),
		ItemURL:     itemURL,
		PubDate:     pubDate,
		PrivateFeed: cfg.PrivateFeed,
		UpdatedAt:   now,
	}

	seen := make(map[string]struct{}, len(channel.Items))
	for _, item := range channel.Items {
		episode := buildEpisode(item, coverArt)
		if episode.VideoURL == "" {
			continue
		}
		if _, ok := seen[episode.ID]; ok {
			continue
		}
		seen[episode.ID] = struct{}{}

		if episode.PubDate.After(result.PubDate) {
			result.PubDate = episode.PubDate
		}
		result.Episodes = append(result.Episodes, episode)
	}

	if len(result.Episodes) == 0 {
		return nil, errors.Errorf("no episodes with enclosure found in RSS feed: %s", cfg.URL)
	}

	return result, nil
}

func (b *RSSBuilder) fetch(ctx context.Context, feedURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create request for %s", feedURL)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Podsync RSS cache)")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, */*")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch RSS feed: %s", feedURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.Errorf("RSS feed returned HTTP %d: %s", resp.StatusCode, feedURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, rssMaxBodySize))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read RSS feed: %s", feedURL)
	}

	return body, nil
}

func buildEpisode(item rssItem, feedCoverArt string) *model.Episode {
	var (
		mediaURL = strings.TrimSpace(item.Enclosure.URL)
		size     = parseInt64(item.Enclosure.Length)
	)

	// Some feeds use media:content instead of (or in addition to) enclosure.
	if mediaURL == "" && item.MediaContent.URL != "" {
		mediaURL = strings.TrimSpace(item.MediaContent.URL)
		size = parseInt64(item.MediaContent.FileSize)
	}

	id := strings.TrimSpace(item.GUID.Value)
	if id == "" {
		id = strings.TrimSpace(item.Link)
	}
	if id == "" {
		id = mediaURL
	}

	thumbnail := item.ITunesImage.Href
	if thumbnail == "" {
		thumbnail = feedCoverArt
	}

	description := strings.TrimSpace(item.Content)
	if description == "" {
		description = strings.TrimSpace(item.Description)
	}

	return &model.Episode{
		ID:          id,
		Title:       strings.TrimSpace(item.Title),
		Description: description,
		Thumbnail:   thumbnail,
		Duration:    parseRSSDuration(item.ITunesDuration),
		VideoURL:    mediaURL,
		PubDate:     parseRSSDate(item.PubDate),
		Size:        size,
		Status:      model.EpisodeAvailable,
	}
}

func parseInt64(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseRSSDuration(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	if !strings.Contains(s, ":") {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		if d, err := time.ParseDuration(s); err == nil {
			return int64(d.Seconds())
		}
		return 0
	}

	var total int64
	for _, part := range strings.Split(s, ":") {
		n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}

func parseRSSDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}

	return time.Time{}
}

type rssDocument struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title          string `xml:"title"`
	Link           string `xml:"link"`
	Description    string `xml:"description"`
	Author         string `xml:"author"`
	ManagingEditor string `xml:"managingEditor"`
	PubDate        string `xml:"pubDate"`
	LastBuildDate  string `xml:"lastBuildDate"`
	// ITunesImage must come before Image: an unqualified tag matches any
	// namespace, so otherwise <itunes:image> would be consumed by rssImage.
	ITunesImage  rssITunesImage `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	Image        rssImage       `xml:"image"`
	ITunesAuthor string         `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd author"`
	Items        []rssItem      `xml:"item"`
}

type rssImage struct {
	URL string `xml:"url"`
}

type rssITunesImage struct {
	Href string `xml:"href,attr"`
}

type rssItem struct {
	GUID           rssGUID         `xml:"guid"`
	Title          string          `xml:"title"`
	Link           string          `xml:"link"`
	Description    string          `xml:"description"`
	Content        string          `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	PubDate        string          `xml:"pubDate"`
	Enclosure      rssEnclosure    `xml:"enclosure"`
	ITunesDuration string          `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd duration"`
	ITunesImage    rssITunesImage  `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	MediaContent   rssMediaContent `xml:"http://search.yahoo.com/mrss/ content"`
}

type rssGUID struct {
	Value string `xml:",chardata"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length string `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type rssMediaContent struct {
	URL      string `xml:"url,attr"`
	Type     string `xml:"type,attr"`
	FileSize string `xml:"fileSize,attr"`
}
