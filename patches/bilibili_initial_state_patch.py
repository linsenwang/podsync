"""
Monkey-patch yt-dlp's BilibiliIE._real_extract to add a fallback for
window.__INITIAL_STATE__ which Bilibili removed from webpage HTML.

When the initial state is not found in the page and the view/detail API
is blocked by risk control (code -352), falls back to the simpler
/x/web-interface/wbi/view API.
"""

import sys


def apply_patch():
    """Apply the __INITIAL_STATE__ fallback patch to yt-dlp's Bilibili extractor."""
    import glob

    patterns = [
        '/usr/local/lib/python*/dist-packages/yt_dlp/extractor/bilibili.py',
        '/usr/lib/python*/site-packages/yt_dlp/extractor/bilibili.py',
        '/usr/local/lib/python*/site-packages/yt_dlp/extractor/bilibili.py',
    ]
    files = []
    for p in patterns:
        files.extend(glob.glob(p))

    if not files:
        print('[bilibili_isp] Could not find bilibili.py extractor', file=sys.stderr)
        return False

    fp = files[0]
    with open(fp) as fh:
        src = fh.read()

    old = (
        "            new_url = traverse_obj(detail, ('data', 'View', 'redirect_url', {url_or_none}))\n"
        "            if new_url and BiliBiliBangumiIE.suitable(new_url):\n"
        "                return self.url_result(new_url, BiliBiliBangumiIE)\n"
        "            raise ExtractorError('Unable to extract initial state')"
    )

    new = (
        "            new_url = traverse_obj(detail, ('data', 'View', 'redirect_url', {url_or_none}))\n"
        "            if new_url and BiliBiliBangumiIE.suitable(new_url):\n"
        "                return self.url_result(new_url, BiliBiliBangumiIE)\n"
        "            video_info = self._download_json(\n"
        "                'https://api.bilibili.com/x/web-interface/wbi/view', video_id,\n"
        "                note='Downloading video info (fallback)',\n"
        "                errnote='Failed to download video info',\n"
        "                fatal=False,\n"
        "                query=self._sign_wbi(query, video_id), headers=headers)\n"
        "            if video_info and traverse_obj(video_info, ('code', {int})) == 0:\n"
        "                vdata = video_info['data']\n"
        "                initial_state = {\n"
        "                    'videoData': vdata,\n"
        "                    'upData': traverse_obj(vdata, {\n"
        "                        'name': ('owner', 'name'),\n"
        "                        'mid': ('owner', 'mid', {str}),\n"
        "                    }),\n"
        "                }\n"
        "                tags = self._download_json(\n"
        "                    'https://api.bilibili.com/x/web-interface/view/detail/tag', video_id,\n"
        "                    fatal=False, query={'bvid': video_id}, headers=headers)\n"
        "                if traverse_obj(tags, ('code', {int})) == 0:\n"
        "                    initial_state['tags'] = tags['data']\n"
        "            else:\n"
        "                raise ExtractorError('Unable to extract initial state')"
    )

    if old in src:
        src = src.replace(old, new)
        with open(fp, 'w') as fh:
            fh.write(src)
        print(f'[bilibili_isp] Applied initial_state fallback patch to {fp}')
        return True
    else:
        print('[bilibili_isp] Patch pattern not found (may already be applied or version changed)', file=sys.stderr)
        return False


if __name__ == '__main__':
    sys.exit(0 if apply_patch() else 1)
