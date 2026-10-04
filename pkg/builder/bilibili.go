package builder

import (
	"context"
	"fmt"
	"strings"

	"time"

	log "github.com/sirupsen/logrus"

	"github.com/yangtfu/podsync/pkg/feed"

	"github.com/yangtfu/podsync/pkg/model"
)

type BilibiliBuilder struct {
	client *APIClient
}

const (
	maxSeriesResults = 100
)

func (b *BilibiliBuilder) Build(_ context.Context, cfg *feed.Config) (*model.Feed, error) {
	info, err := ParseURL(cfg.URL)

	if err != nil {
		return nil, err
	}

	_feed := &model.Feed{
		ItemID:          info.ItemID,
		Provider:        info.Provider,
		LinkType:        info.LinkType,
		Format:          cfg.Format,
		Quality:         cfg.Quality,
		CoverArtQuality: cfg.Custom.CoverArtQuality,
		PageSize:        cfg.PageSize,
		PlaylistSort:    cfg.PlaylistSort,
		PrivateFeed:     cfg.PrivateFeed,
		UpdatedAt:       time.Now().UTC(),
		ItemURL:         cfg.URL,
	}

	var archives []Archive

	switch info.LinkType {
	case model.TypeSeason:
		mid, seasonId := strings.Split(info.ItemID, ":")[0], strings.Split(info.ItemID, ":")[1]
		userResponse, err := b.client.GetUserInfo(mid)
		if err != nil {
			return nil, err
		}
		_feed.Author = userResponse.Data.Card.Name
		seasonArchivesResponse, err := b.client.GetSeasonEpisodesByPage(mid, seasonId, 1, _feed.PageSize)
		if err != nil {
			return nil, err
		}
		_feed.CoverArt = seasonArchivesResponse.Data.Meta.Cover
		_feed.Title = seasonArchivesResponse.Data.Meta.Name
		_feed.Description = seasonArchivesResponse.Data.Meta.Description
		_feed.ItemURL = fmt.Sprintf("https://space.bilibili.com/%s/lists/%s?type=season", mid, seasonId)
		archives = seasonArchivesResponse.Data.Archives[:]
	case model.TypeSeries:
		mid, seriesId := strings.Split(info.ItemID, ":")[0], strings.Split(info.ItemID, ":")[1]
		seriesInfoResponse, err := b.client.GetSeriesInfo(seriesId)
		if err != nil {
			return nil, err
		}
		userResponse, err := b.client.GetUserInfo(mid)
		if err != nil {
			return nil, err
		}
		_feed.Author = userResponse.Data.Card.Name
		_feed.CoverArt = userResponse.Data.Card.Face
		_feed.Title = seriesInfoResponse.Data.Meta.Name
		_feed.Description = seriesInfoResponse.Data.Meta.Description
		_feed.ItemURL = fmt.Sprintf("https://space.bilibili.com/%s/lists/%s?type=series", mid, seriesId)

		seriesArchivesResponse, err := b.client.GetSeriesEpisodesByPage(mid, seriesId, 1, maxSeriesResults)
		if err != nil {
			return nil, err
		}
		archives = seriesArchivesResponse.Data.Archives[:]
	default:
		// case model.TypeUser:
		userResponse, err := b.client.GetUserInfo(info.ItemID)
		if err != nil {
			return nil, err
		}
		_feed.Author = userResponse.Data.Card.Name
		_feed.CoverArt = userResponse.Data.Card.Face
		_feed.Title = userResponse.Data.Card.Name
		_feed.Description = userResponse.Data.Card.Sign
		_feed.ItemURL = fmt.Sprintf("https://space.bilibili.com/%s", info.ItemID)

		// query video collection
		userEpisodesResponse, err := b.client.GetUserEpisodesByPage(info.ItemID, 1, _feed.PageSize)
		if err != nil {
			return nil, err
		}
		archives = userEpisodesResponse.Data.Archives[:]
	}
	var added = 0
	for _, videoInfo := range archives {
		if videoInfo.State != 0 || videoInfo.UgcPay != 0 {
			continue
		}

		_feed.Episodes = append(_feed.Episodes, b.episodesFor(videoInfo)...)

		// page_size 限制的是投稿数量，每个多P投稿按其分P全部展开。
		added++

		if added >= _feed.PageSize {
			break
		}
	}
	return _feed, nil
}

// episodesFor 把一个投稿展开成 episode 列表。B 站的多P投稿（同一投稿下多个分P）里
// 每个 P 是独立的一期内容，必须各生成一集：否则下载 URL 不带 ?p= 时 yt-dlp 会把所有
// 分P下到同一个输出文件上、互相覆盖，最后只剩 P1（且时长标的是整个投稿的总时长）。
func (b *BilibiliBuilder) episodesFor(videoInfo Archive) []*model.Episode {
	pages, err := b.client.GetVideoPages(videoInfo.Bvid)
	if err != nil {
		// 拿不到分P信息时退化成原来的单集行为，避免整条 feed 更新失败。
		log.WithError(err).WithField("bvid", videoInfo.Bvid).
			Warn("failed to get bilibili video pages, falling back to single episode")
		return []*model.Episode{singleEpisodeFromArchive(videoInfo)}
	}

	return episodesFromArchive(videoInfo, pages)
}

// episodesFromArchive 按分P列表生成 episode。单P视频保持原来的 ID（BV 号），
// 多P视频 P1 也用 BV 号（兼容已经下载好的数据），P2 起用 BVxxx_pN。
func episodesFromArchive(videoInfo Archive, pages []VideoPage) []*model.Episode {
	if len(pages) <= 1 {
		return []*model.Episode{singleEpisodeFromArchive(videoInfo)}
	}

	episodes := make([]*model.Episode, 0, len(pages))
	for i, page := range pages {
		pageNum := page.Page
		if pageNum <= 0 {
			pageNum = i + 1
		}

		id := videoInfo.Bvid
		if pageNum > 1 {
			id = fmt.Sprintf("%s_p%d", videoInfo.Bvid, pageNum)
		}

		title := page.Part
		if title == "" {
			title = videoInfo.Title
		}

		episodes = append(episodes, &model.Episode{
			ID:          id,
			Title:       title,
			Description: videoInfo.Desc,
			Duration:    int64(page.Duration),
			Size:        int64(page.Duration * 15000), // very rough estimate
			VideoURL:    fmt.Sprintf("https://www.bilibili.com/video/%s?p=%d", videoInfo.Bvid, pageNum),
			// 各分P共享投稿时间，加页序号（秒）让 feed 里保持 P1→PN 的稳定顺序
			PubDate:   time.Unix(videoInfo.PubDate+int64(pageNum-1), 0),
			Thumbnail: videoInfo.Pic,
			Status:    model.EpisodeNew,
		})
	}

	return episodes
}

func singleEpisodeFromArchive(videoInfo Archive) *model.Episode {
	return &model.Episode{
		ID:          videoInfo.Bvid,
		Title:       videoInfo.Title,
		Description: videoInfo.Desc,
		Duration:    int64(videoInfo.Duration),
		Size:        int64(videoInfo.Duration * 15000), // very rough estimate
		VideoURL:    "https://www.bilibili.com/video/" + videoInfo.Bvid,
		PubDate:     time.Unix(videoInfo.PubDate, 0),
		Thumbnail:   videoInfo.Pic,
		Status:      model.EpisodeNew,
	}
}

func NewBilibiliBuilder() (*BilibiliBuilder, error) {
	return &BilibiliBuilder{
		client: NewAPIClient(),
	}, nil
}
