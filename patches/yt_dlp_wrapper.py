#!/usr/bin/env python3
"""yt-dlp 入口包装脚本。

安装为 /usr/local/bin/yt-dlp，优先级高于 pip 生成的 /usr/bin/yt-dlp，
因此在调用真正的 yt-dlp 之前可以先把 B 站风控补丁打上。

补丁说明见 patches/bilibili_dm_patch.py。
"""
import sys

sys.path.insert(0, '/app/patches')

try:
    from bilibili_dm_patch import apply_bilibili_dm_img_patch
    apply_bilibili_dm_img_patch()
except Exception as exc:
    print(f'[yt-dlp-wrapper] failed to apply bilibili patch: {exc}', file=sys.stderr)

import yt_dlp

if __name__ == '__main__':
    yt_dlp.main()
