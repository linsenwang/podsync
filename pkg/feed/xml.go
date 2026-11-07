package feed

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	itunes "github.com/eduncan911/podcast"
	"github.com/pkg/errors"

	"github.com/yangtfu/podsync/pkg/model"
)

// xmlChar implements the XML 1.0 Char production:
//
//	Char ::= #x9 | #xA | #xD | [#x20-#xD7FF] | [#xE000-#xFFFD] | [#x10000-#x10FFFF]
//
// encoding/xml enforces this for ordinary character data via EscapeText, but writes
// `,cdata` fields verbatim. Several podcast fields below are `,cdata` (<itunes:summary>),
// so without this a stray control byte from a Bilibili description (0x0B and 0x05 seen
// in the wild) lands raw inside a CDATA section and makes the whole feed unparseable.
// Returns -1 for characters strings.Map should drop.
func xmlChar(r rune) rune {
	switch {
	case r == 0x9 || r == 0xA || r == 0xD:
		return r
	case r >= 0x20 && r <= 0xD7FF:
		return r
	case r >= 0xE000 && r <= 0xFFFD:
		return r
	case r >= 0x10000 && r <= 0x10FFFF:
		return r
	default:
		return -1
	}
}

// sanitizeXML strips characters that cannot appear in an XML 1.0 document. Invalid
// UTF-8 decodes to U+FFFD, which is legal, so it is kept — same as EscapeText.
func sanitizeXML(s string) string {
	return strings.Map(xmlChar, s)
}

// sort.Interface implementation
type timeSlice []*model.Episode

func (p timeSlice) Len() int {
	return len(p)
}

// In descending order
func (p timeSlice) Less(i, j int) bool {
	return p[i].PubDate.After(p[j].PubDate)
}

func (p timeSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func Build(_ context.Context, feed *model.Feed, cfg *Config, hostname string) (*itunes.Podcast, error) {
	const (
		podsyncGenerator = "Podsync generator (support us at https://github.com/yangtfu/podsync)"
		defaultCategory  = "TV & Film"
	)

	var (
		now         = time.Now().UTC()
		author      = feed.Author
		title       = feed.Title
		description = feed.Description
		feedLink    = feed.ItemURL
	)

	if author == "<notfound>" {
		author = feed.Title
	}

	if cfg.Custom.Author != "" {
		author = cfg.Custom.Author
	}

	if cfg.Custom.Title != "" {
		title = cfg.Custom.Title
	}

	if cfg.Custom.Description != "" {
		description = cfg.Custom.Description
	}

	if cfg.Custom.Link != "" {
		feedLink = cfg.Custom.Link
	}

	title = sanitizeXML(title)
	feedLink = sanitizeXML(feedLink)
	description = sanitizeXML(description)
	author = sanitizeXML(author)

	p := itunes.New(title, feedLink, description, &feed.PubDate, &now)
	p.Generator = podsyncGenerator
	p.AddSubTitle(title)
	p.IAuthor = author
	p.AddSummary(description)

	if feed.PrivateFeed {
		p.IBlock = "yes"
	}

	if cfg.Custom.OwnerName != "" && cfg.Custom.OwnerEmail != "" {
		p.IOwner = &itunes.Author{
			Name:  cfg.Custom.OwnerName,
			Email: cfg.Custom.OwnerEmail,
		}
	}

	if cfg.Custom.CoverArt != "" {
		p.AddImage(cfg.Custom.CoverArt)
	} else {
		p.AddImage(feed.CoverArt)
	}

	if cfg.Custom.Category != "" {
		p.AddCategory(cfg.Custom.Category, cfg.Custom.Subcategories)
	} else {
		p.AddCategory(defaultCategory, cfg.Custom.Subcategories)
	}

	if cfg.Custom.Explicit {
		p.IExplicit = "yes"
	} else {
		p.IExplicit = "no"
	}

	if cfg.Custom.Language != "" {
		p.Language = cfg.Custom.Language
	}

	for _, episode := range feed.Episodes {
		if episode.PubDate.IsZero() {
			// RSS feeds keep the original ordering; falling back to "now" would
			// move undated episodes to the top of the list.
			if feed.Provider == model.ProviderRSS {
				episode.PubDate = feed.PubDate
			} else {
				episode.PubDate = now
			}
		}
	}

	// Sort all episodes in descending order
	sort.Sort(timeSlice(feed.Episodes))

	isRSS := feed.Provider == model.ProviderRSS
	for i, episode := range feed.Episodes {
		if !isRSS && episode.Status != model.EpisodeDownloaded {
			// Skip episodes that are not yet downloaded or have been removed
			continue
		}
		if isRSS && episode.VideoURL == "" {
			continue
		}

		// Episode titles and descriptions reach both escaped character data and CDATA
		// fields (ISubtitle/ISummary), so normalize them once here.
		episodeTitle := sanitizeXML(episode.Title)
		episodeDescription := sanitizeXML(episode.Description)

		item := itunes.Item{
			GUID:        episode.ID,
			Link:        episode.VideoURL,
			Title:       episodeTitle,
			Description: episodeDescription,
			ISubtitle:   episodeTitle,
			// Some app prefer 1-based order
			IOrder: strconv.Itoa(i + 1),
		}

		item.AddPubDate(&episode.PubDate)
		item.AddSummary(episodeDescription)
		item.AddImage(episode.Thumbnail)
		item.AddDuration(episode.Duration)

		enclosureType := itunes.MP4
		if feed.Format == model.FormatAudio {
			enclosureType = itunes.MP3
		}
		if feed.Format == model.FormatCustom {
			enclosureType = EnclosureFromExtension(cfg)
		}

		if isRSS {
			// RSS feeds are cached, not mirrored: keep the original media URL.
			item.AddEnclosure(episode.VideoURL, EnclosureFromURL(episode.VideoURL), episode.Size)
		} else {
			var (
				episodeName = EpisodeName(cfg, episode)
				downloadURL = fmt.Sprintf("%s/%s/%s", strings.TrimRight(hostname, "/"), cfg.ID, episodeName)
			)

			item.AddEnclosure(downloadURL, enclosureType, episode.Size)
		}

		// p.AddItem requires description to be not empty, use workaround
		if item.Description == "" {
			item.Description = " "
		}

		if cfg.Custom.Explicit {
			item.IExplicit = "yes"
		} else {
			item.IExplicit = "no"
		}

		if _, err := p.AddItem(item); err != nil {
			return nil, errors.Wrapf(err, "failed to add item to podcast (id %q)", episode.ID)
		}
	}

	return &p, nil
}

func EpisodeName(feedConfig *Config, episode *model.Episode) string {
	ext := "mp4"
	if feedConfig.Format == model.FormatAudio {
		ext = "mp3"
	}
	if feedConfig.Format == model.FormatCustom {
		ext = feedConfig.CustomFormat.Extension
	}

	return fmt.Sprintf("%s.%s", episode.ID, ext)
}

func EnclosureFromExtension(feedConfig *Config) itunes.EnclosureType {
	ext := feedConfig.CustomFormat.Extension

	switch ext {
	case "m4a":
		return itunes.M4A
	case "m4v":
		return itunes.M4V
	case "mp4":
		return itunes.MP4
	case "mp3":
		return itunes.MP3
	case "mov":
		return itunes.MOV
	case "pdf":
		return itunes.PDF
	case "epub":
		return itunes.EPUB
	default:
		return -1
	}
}

// EnclosureFromURL guesses the enclosure MIME type from a media URL. RSS feeds
// keep their original URLs, so the extension is the only hint we have.
func EnclosureFromURL(rawURL string) itunes.EnclosureType {
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
		rawURL = parsed.Path
	}

	switch strings.ToLower(path.Ext(rawURL)) {
	case ".m4a":
		return itunes.M4A
	case ".m4v":
		return itunes.M4V
	case ".mp4":
		return itunes.MP4
	case ".mov":
		return itunes.MOV
	case ".pdf":
		return itunes.PDF
	case ".epub":
		return itunes.EPUB
	default:
		return itunes.MP3
	}
}
