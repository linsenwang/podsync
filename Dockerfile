FROM golang:1.25 AS builder

ENV TAG="nightly"
ENV COMMIT=""

# Optional Go module proxy override, e.g. https://goproxy.cn,direct when building
# from mainland China. Empty by default, so upstream defaults are used.
ARG GOPROXY

WORKDIR /build

COPY . .

RUN make build

# Alpine 3.22 will go EOL on 2027-05-01
FROM alpine:3.22

WORKDIR /app

# deno is the JS runtime yt-dlp prefers (ref: https://github.com/yt-dlp/yt-dlp/issues/14404)
# nodejs 也装上：现有 config.toml 的 YouTube feed 都显式传了 --js-runtimes node
# inotify-tools 供入口脚本后台清理 feed XML 中的非法字符
RUN apk --no-cache add ca-certificates python3 py3-pip ffmpeg tzdata libc6-compat deno nodejs inotify-tools

# 使用 pip 安装 yt-dlp（而不是官方独立可执行文件），因为 B 站补丁需要 patch yt_dlp 模块
RUN pip3 install --no-cache-dir --break-system-packages yt-dlp yt-dlp-ejs

RUN chmod 777 /usr/local/bin
COPY --from=builder /build/bin/podsync /app/podsync
COPY --from=builder /build/html/index.html /app/html/index.html

# B 站补丁：playurl 风控参数注入 + __INITIAL_STATE__ 回退
COPY patches/ /app/patches/
# 全局 yt-dlp 配置：B 站请求头 + 重试退避
COPY patches/yt-dlp.conf /etc/yt-dlp.conf

# yt-dlp wrapper：先打 dm_img 补丁，再调用 yt_dlp 主入口。
# 放在 /usr/local/bin，优先级高于 pip 生成的 /usr/bin/yt-dlp，pip 升级不会覆盖它。
RUN cp /app/patches/yt_dlp_wrapper.py /usr/local/bin/yt-dlp && \
    chmod +x /usr/local/bin/yt-dlp && \
    ln -sf /usr/local/bin/yt-dlp /usr/local/bin/youtube-dl && \
    ln -sf /usr/local/bin/yt-dlp /usr/bin/youtube-dl

# 入口脚本：启动时自更新 yt-dlp、打 initial_state 补丁、清理非法 XML 字符
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["--no-banner"]
