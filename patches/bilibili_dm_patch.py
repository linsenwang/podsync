"""
Monkey-patch yt-dlp's Bilibili extractor to inject dm_img_* / web_location
risk-control parameters required by Bilibili's x/player/wbi/playurl gateway.

Around 2026-06 Bilibili started rejecting playurl requests that omit the browser
fingerprint params (dm_img_list / dm_img_str / dm_cover_img_str / dm_img_inter +
web_location) with HTTP 412. yt-dlp's upstream extractor does not send these for
the playurl endpoint yet. We inject dummy-but-well-formed values *before* WBI
signing, matching the shapes yt-dlp itself uses for arc/search.
"""
import base64
import random
import string


def build_dm_img_params():
    """Return dummy dm_img_* / web_location params the gateway expects."""
    return {
        'web_location': 1550101,
        'dm_img_list': '[]',
        'dm_img_str': base64.b64encode(
            ''.join(random.choices(string.printable, k=random.randint(16, 64))).encode()
        )[:-2].decode(),
        'dm_cover_img_str': base64.b64encode(
            ''.join(random.choices(string.printable, k=random.randint(32, 128))).encode()
        )[:-2].decode(),
        'dm_img_inter': '{"ds":[],"wh":[6093,6631,31],"of":[430,760,380]}',
    }


def apply_bilibili_dm_img_patch():
    """
    Monkey-patch BilibiliBaseIE._download_playinfo. Idempotent.
    Returns True if the patch is active, False if yt-dlp internals could not be
    patched (logged but never raised).
    """
    try:
        from yt_dlp.extractor.bilibili import BilibiliBaseIE
    except Exception as exc:  # pragma: no cover
        print(f'[bilibili_dm_patch] import failed: {exc}', flush=True)
        return False

    original = BilibiliBaseIE._download_playinfo
    if getattr(original, '_bili_dm_patched', False):
        return True

    def _patched_download_playinfo(self, bvid, cid, headers=None, query=None, **kwargs):
        merged_query = {**build_dm_img_params(), **(query or {})}
        return original(self, bvid, cid, headers=headers, query=merged_query, **kwargs)

    _patched_download_playinfo._bili_dm_patched = True
    BilibiliBaseIE._download_playinfo = _patched_download_playinfo
    return True
