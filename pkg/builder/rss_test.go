package builder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yangtfu/podsync/pkg/feed"
	"github.com/yangtfu/podsync/pkg/model"
)

func TestRSSBuilder_Build(t *testing.T) {
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"
     xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"
     xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test Podcast</title>
    <link>https://example.com/</link>
    <description>Feed description</description>
    <itunes:author>Feed Author</itunes:author>
    <itunes:image href="https://example.com/cover.jpg"/>
    <item>
      <title>Episode 1</title>
      <guid>ep-1</guid>
      <pubDate>Tue, 06 Oct 2020 12:00:00 +0800</pubDate>
      <description>short description</description>
      <content:encoded><![CDATA[<p>full description</p>]]></content:encoded>
      <itunes:duration>1:02:03</itunes:duration>
      <itunes:image href="https://example.com/ep1.jpg"/>
      <enclosure url="https://cdn.example.com/ep1.mp3" length="12345" type="audio/mpeg"/>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	b, err := NewRSSBuilder()
	require.NoError(t, err)

	result, err := b.Build(context.Background(), &feed.Config{URL: srv.URL, Format: model.FormatAudio})
	require.NoError(t, err)

	require.Equal(t, model.ProviderRSS, result.Provider)
	require.Equal(t, model.TypeFeed, result.LinkType)
	require.Equal(t, "Test Podcast", result.Title)
	require.Equal(t, "Feed Author", result.Author)
	require.Equal(t, "https://example.com/cover.jpg", result.CoverArt)
	require.Len(t, result.Episodes, 1)

	ep := result.Episodes[0]
	require.Equal(t, "ep-1", ep.ID)
	require.Equal(t, "Episode 1", ep.Title)
	require.Equal(t, int64(3723), ep.Duration)
	require.Equal(t, "https://cdn.example.com/ep1.mp3", ep.VideoURL)
	require.Equal(t, int64(12345), ep.Size)
	require.Equal(t, model.EpisodeAvailable, ep.Status)
	require.Equal(t, "https://example.com/ep1.jpg", ep.Thumbnail)
	require.Contains(t, ep.Description, "full description")
}

func TestParseRSSDuration(t *testing.T) {
	require.Equal(t, int64(3723), parseRSSDuration("1:02:03"))
	require.Equal(t, int64(125), parseRSSDuration("2:05"))
	require.Equal(t, int64(90), parseRSSDuration("90"))
	require.Equal(t, int64(0), parseRSSDuration(""))
}
