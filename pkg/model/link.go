package model

type Type string

const (
	TypeChannel  = Type("channel")
	TypePlaylist = Type("playlist")
	TypeSeason   = Type("season")
	TypeSeries   = Type("series")
	TypeUser     = Type("user")
	TypeGroup    = Type("group")
	TypeHandle   = Type("handle")
	TypeFeed     = Type("feed")
)

type Provider string

const (
	ProviderBilibili   = Provider("bilibili")
	ProviderYoutube    = Provider("youtube")
	ProviderVimeo      = Provider("vimeo")
	ProviderSoundcloud = Provider("soundcloud")
	ProviderTwitch     = Provider("twitch")
	// ProviderRSS caches an existing podcast RSS feed and serves a local copy.
	// Media files stay on the original host; nothing is downloaded by podsync.
	ProviderRSS = Provider("rss")
)

// Info represents data extracted from URL
type Info struct {
	LinkType Type     // Either group, channel or user
	Provider Provider // Youtube, Vimeo, SoundCloud or Twitch
	ItemID   string
}
