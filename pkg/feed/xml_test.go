package feed

import (
	"context"
	"encoding/xml"
	"testing"

	itunes "github.com/eduncan911/podcast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yangtfu/podsync/pkg/model"
)

func TestBuildXML(t *testing.T) {
	feed := model.Feed{
		Episodes: []*model.Episode{
			{
				ID:          "1",
				Status:      model.EpisodeDownloaded,
				Title:       "title",
				Description: "description",
			},
		},
	}

	cfg := Config{
		ID:     "test",
		Custom: Custom{Description: "description", Category: "Technology", Subcategories: []string{"Gadgets", "Podcasting"}},
	}

	out, err := Build(context.Background(), &feed, &cfg, "http://localhost/")
	assert.NoError(t, err)

	assert.EqualValues(t, "description", out.Description)
	assert.EqualValues(t, "Technology", out.Category)

	require.Len(t, out.ICategories, 1)
	category := out.ICategories[0]
	assert.EqualValues(t, "Technology", category.Text)

	require.Len(t, category.ICategories, 2)
	assert.EqualValues(t, "Gadgets", category.ICategories[0].Text)
	assert.EqualValues(t, "Podcasting", category.ICategories[1].Text)

	require.Len(t, out.Items, 1)
	require.NotNil(t, out.Items[0].Enclosure)
	assert.EqualValues(t, out.Items[0].Enclosure.URL, "http://localhost/test/1.mp4")
	assert.EqualValues(t, out.Items[0].Enclosure.Type, itunes.MP4)
}

func TestBuildSanitizesInvalidXMLCharacters(t *testing.T) {
	feed := model.Feed{
		Title:       "title\x0b",
		Description: "description\x05",
		Author:      "author\x0c",
		Episodes: []*model.Episode{
			{
				ID:          "1",
				Status:      model.EpisodeDownloaded,
				Title:       "episode\x0b",
				Description: "summary\x1f",
			},
		},
	}

	cfg := Config{ID: "test", Custom: Custom{Category: "Technology"}}

	out, err := Build(context.Background(), &feed, &cfg, "http://localhost/")
	require.NoError(t, err)

	doc := out.String()

	// CDATA fields (itunes:summary) are emitted verbatim by encoding/xml, so a raw
	// control character there makes the whole document unparseable.
	assert.NoError(t, xml.Unmarshal([]byte(doc), new(struct{ XMLName xml.Name })))
	assert.NotContains(t, doc, "\x05")
	assert.NotContains(t, doc, "\x0b")
	assert.NotContains(t, doc, "\x0c")
	assert.NotContains(t, doc, "\x1f")
	assert.Contains(t, doc, "description")
}
