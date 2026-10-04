package builder

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yangtfu/podsync/pkg/feed"
)

func isTemporaryBilibiliIntegrationError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "request was banned") ||
		strings.Contains(msg, "Temporary failure in name resolution") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "connection reset")
}

func TestBilibili_BuildFeed(t *testing.T) {
	builder, err := NewBilibiliBuilder()
	require.NoError(t, err)

	urls := []string{
		"https://space.bilibili.com/1302298364",
		"https://space.bilibili.com/7458285/lists/1067956?type=series",
		"https://space.bilibili.com/7380321/lists/678635?type=season",
	}

	for _, addr := range urls {
		t.Run(addr, func(t *testing.T) {
			fmt.Print(addr)

			_feed, err := builder.Build(testCtx, &feed.Config{URL: addr})
			if isTemporaryBilibiliIntegrationError(err) {
				t.Skipf("skipping Bilibili integration test due to temporary API/network error: %v", err)
			}
			require.NoError(t, err)

			assert.NotEmpty(t, _feed.Title)
			assert.NotNil(t, _feed.Description)
			assert.NotEmpty(t, _feed.Author)
			assert.NotEmpty(t, _feed.ItemURL)

			assert.NotZero(t, len(_feed.Episodes))

			for _, item := range _feed.Episodes {
				assert.NotEmpty(t, item.Title)
				assert.NotEmpty(t, item.VideoURL)
				assert.NotZero(t, item.Duration)
				assert.NotEmpty(t, item.Title)
				assert.NotEmpty(t, item.Thumbnail)
			}
		})
	}
}

func TestBilibili_EpisodesFromArchive(t *testing.T) {
	archive := Archive{
		Bvid:     "BV1xx411c7mD",
		Title:    "合集标题",
		Desc:     "合集描述",
		Pic:      "http://example.com/pic.jpg",
		PubDate:  1700000000,
		Duration: 300,
	}

	t.Run("single page keeps original episode", func(t *testing.T) {
		pages := []VideoPage{{Page: 1, Part: "合集标题", Duration: 300}}

		episodes := episodesFromArchive(archive, pages)
		require.Len(t, episodes, 1)
		assert.Equal(t, "BV1xx411c7mD", episodes[0].ID)
		assert.Equal(t, "https://www.bilibili.com/video/BV1xx411c7mD", episodes[0].VideoURL)
		assert.Equal(t, int64(300), episodes[0].Duration)
	})

	t.Run("multi page expands every part", func(t *testing.T) {
		pages := []VideoPage{
			{Page: 1, Part: "第一期", Duration: 120},
			{Page: 2, Part: "第二期", Duration: 180},
			{Page: 3, Part: "第三期", Duration: 240},
		}

		episodes := episodesFromArchive(archive, pages)
		require.Len(t, episodes, 3)

		assert.Equal(t, "BV1xx411c7mD", episodes[0].ID)
		assert.Equal(t, "BV1xx411c7mD_p2", episodes[1].ID)
		assert.Equal(t, "BV1xx411c7mD_p3", episodes[2].ID)

		assert.Equal(t, "第一期", episodes[0].Title)
		assert.Equal(t, "第三期", episodes[2].Title)

		assert.Equal(t, "https://www.bilibili.com/video/BV1xx411c7mD?p=1", episodes[0].VideoURL)
		assert.Equal(t, "https://www.bilibili.com/video/BV1xx411c7mD?p=3", episodes[2].VideoURL)

		assert.Equal(t, int64(120), episodes[0].Duration)
		assert.Equal(t, int64(240), episodes[2].Duration)

		// 各分P共享投稿时间时按页序号错开，保证 feed 里 P1→PN 顺序稳定
		assert.True(t, episodes[0].PubDate.Before(episodes[2].PubDate))
	})

	t.Run("empty part title falls back to video title", func(t *testing.T) {
		pages := []VideoPage{
			{Page: 1, Part: "", Duration: 100},
			{Page: 2, Part: "", Duration: 200},
		}

		episodes := episodesFromArchive(archive, pages)
		require.Len(t, episodes, 2)
		assert.Equal(t, "合集标题", episodes[0].Title)
		assert.Equal(t, "合集标题", episodes[1].Title)
	})
}
