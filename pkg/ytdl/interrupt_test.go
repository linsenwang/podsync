package ytdl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 进程退出时上下文被取消，yt-dlp 会被 exec 直接杀掉：必须归到 ErrInterrupted，
// 否则容器每次重启都会把在跑的下载记成错误。
func TestExecInterruptedByContextCancel(t *testing.T) {
	dl := &YoutubeDl{path: "/bin/sh", timeout: time.Minute}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	_, err := dl.exec(ctx, "-c", "exec sleep 5")

	assert.Equal(t, ErrInterrupted, err)
}

// 真正的 yt-dlp 失败不能被当成「进程退出」而静默掉。
func TestExecReportsRealFailure(t *testing.T) {
	dl := &YoutubeDl{path: "/bin/sh", timeout: time.Minute}

	output, err := dl.exec(context.Background(), "-c", "echo boom >&2; exit 3")

	require.Error(t, err)
	assert.NotEqual(t, ErrInterrupted, err)
	assert.Contains(t, output, "boom")
}
