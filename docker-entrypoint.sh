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

# 后台监控 feed XML，实时清理非法字符（0x00-0x08, 0x0B, 0x0C, 0x0E-0x1F）
if command -v inotifywait >/dev/null 2>&1; then
    (
        # 等待 podsync 启动并创建 XML 文件
        sleep 5
        while true; do
            if [ -d /app/data ]; then
                inotifywait -e modify,create -r /app/data --include='.*\.xml$' 2>/dev/null | while read -r path file; do
                    xml_file="${path}${file}"
                    if [ -f "$xml_file" ]; then
                        if tr -d '\000-\010\013\014\016-\037' < "$xml_file" > "${xml_file}.tmp" 2>/dev/null; then
                            mv "${xml_file}.tmp" "$xml_file"
                            echo "[$(date)] Cleaned XML: $xml_file"
                        else
                            rm -f "${xml_file}.tmp"
                        fi
                    fi
                done
            fi
            sleep 1
        done
    ) &
fi

# 启动 podsync（主进程）
exec /app/podsync "$@"
