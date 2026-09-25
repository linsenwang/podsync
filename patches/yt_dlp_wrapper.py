#!/usr/bin/env python3
"""yt-dlp 入口包装脚本。

安装为 /usr/local/bin/yt-dlp，优先级高于 pip 生成的 /usr/bin/yt-dlp，
因此在调用真正的 yt-dlp 之前可以先把补丁打上：

  * patches/bilibili_dm_patch.py       B 站 playurl 风控参数
  * patches/youtube_player_retry_patch.py  YouTube player JS 下载重试

两个补丁在 import yt_dlp 之前打上，对 yt-dlp 主入口透明。
"""
import sys

sys.path.insert(0, '/app/patches')

try:
    from bilibili_dm_patch import apply_bilibili_dm_img_patch
    apply_bilibili_dm_img_patch()
except Exception as exc:
    print(f'[yt-dlp-wrapper] failed to apply bilibili patch: {exc}', file=sys.stderr)

try:
    from youtube_player_retry_patch import apply_youtube_player_retry_patch
    apply_youtube_player_retry_patch()
except Exception as exc:
    print(f'[yt-dlp-wrapper] failed to apply youtube player retry patch: {exc}', file=sys.stderr)

import yt_dlp

if __name__ == '__main__':
    yt_dlp.main()
