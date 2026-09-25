"""
Monkey-patch yt-dlp's YouTube extractor so that a failed player-JS download is retried.

YouTube's player JS (/s/player/<id>/player_es6.vflset/en_US/base.js, ~2.5 MB) is
fetched with a *single* attempt: YoutubeIE._load_player() calls _download_webpage(...,
fatal=False) and gives up on the first network error. Behind our proxy that request
fails intermittently with `[SSL: UNEXPECTED_EOF_WHILE_READING] EOF occurred in
violation of protocol` - roughly 5 out of 10 attempts from this host.

When the player JS is missing, yt-dlp cannot compute nsig, so the web/web_safari
clients yield no usable formats, and the remaining JS-less clients (web_embedded,
tv_downgraded) answer UNPLAYABLE for many videos. yt-dlp then exits with a bogus
"Video unavailable" instead of retrying, and podsync drops the episode.

The patch wraps _load_player() in a retry/backoff loop, so a transient proxy hiccup
no longer fails the whole extraction. Everything else (webpage, player API JSON,
media) is already covered by yt-dlp's own --extractor-retries / --retries handling.
"""
import time

# The player JS download is the single-shot step; 4 tries push the failure rate of
# a 60%-lossy request path from ~0.6 down to ~0.1%.
RETRIES = 4
DELAY_SECONDS = 2.0


def apply_youtube_player_retry_patch():
    """
    Monkey-patch YoutubeIE._load_player. Idempotent.
    Returns True if the patch is active, False if yt-dlp internals could not be
    patched (logged but never raised).
    """
    try:
        from yt_dlp.extractor.youtube import YoutubeIE
    except Exception as exc:  # pragma: no cover
        print(f'[youtube_player_retry] import failed: {exc}', flush=True)
        return False

    original = YoutubeIE._load_player
    if getattr(original, '_podsync_player_retry_patched', False):
        return True

    def _load_player(self, video_id, player_url, fatal=True):
        for attempt in range(1, RETRIES + 1):
            try:
                code = original(self, video_id, player_url, fatal=False)
            except Exception as exc:
                code = None
                print(f'[youtube_player_retry] {video_id}: attempt {attempt} raised {exc!r}', flush=True)
            if code:
                if attempt > 1:
                    print(
                        f'[youtube_player_retry] {video_id}: player JS loaded on attempt {attempt}',
                        flush=True,
                    )
                return code
            if attempt < RETRIES:
                time.sleep(DELAY_SECONDS * attempt)

        # All retries exhausted: keep upstream behaviour (raise when fatal, else None).
        return original(self, video_id, player_url, fatal=fatal)

    _load_player._podsync_player_retry_patched = True
    YoutubeIE._load_player = _load_player
    return True
