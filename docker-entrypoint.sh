#!/bin/sh
# Podsync 容器入口脚本，在启动 podsync 之前准备好带 B 站补丁的 yt-dlp。
set -e

# 每次启动时把 pip 安装的 yt-dlp 升到最新版（失败不阻塞启动）
echo "[$(date)] Checking for yt-dlp updates..."
pip3 install --break-system-packages -U yt-dlp yt-dlp-ejs 2>/dev/null || true

# pip 升级不会动 /usr/local/bin/yt-dlp，但保险起见检查一次 wrapper
if [ ! -x /usr/local/bin/yt-dlp ]; then
    echo "[$(date)] Restoring yt-dlp wrapper..."
    cp /app/patches/yt_dlp_wrapper.py /usr/local/bin/yt-dlp
    chmod +x /usr/local/bin/yt-dlp
fi

# podsync 优先查找 yt-dlp，其次 youtube-dl，两个名字都指向 wrapper
rm -f /usr/local/bin/youtube-dl /usr/bin/youtube-dl
ln -sf /usr/local/bin/yt-dlp /usr/local/bin/youtube-dl
ln -sf /usr/local/bin/yt-dlp /usr/bin/youtube-dl

# B 站已从网页移除 window.__INITIAL_STATE__，给 yt-dlp 打 API 回退补丁
echo "[$(date)] Applying Bilibili initial_state fallback patch..."
python3 /app/patches/bilibili_initial_state_patch.py || true

echo "[$(date)] Current yt-dlp version: $(yt-dlp --version)"

# 启动 podsync（主进程）
exec /app/podsync "$@"
