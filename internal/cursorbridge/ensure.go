package cursorbridge

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// BridgeRelease 固定的 sdk-bridge 版本（与 GitHub release tag 对齐）。
const BridgeRelease = "v1.0.37"

var ensureMu sync.Mutex

// ProgressFn 下载/准备进度回调（可选）。
type ProgressFn func(stage, detail string)

// EnsureBinary 确保 dataDir/bin 下有 cursor-sdk-bridge；优先环境变量 / PATH，否则下载 release。
func EnsureBinary(dataDir string, onProgress ProgressFn) (string, error) {
	ensureMu.Lock()
	defer ensureMu.Unlock()

	if p := strings.TrimSpace(os.Getenv("CURSOR_SDK_BRIDGE_BIN")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		return "", fmt.Errorf("CURSOR_SDK_BRIDGE_BIN 无效: %s", p)
	}
	name := "cursor-sdk-bridge"
	if runtime.GOOS == "windows" {
		name = "cursor-sdk-bridge.exe"
	}
	if p, err := lookPath(name); err == nil {
		return p, nil
	}
	binDir := filepath.Join(dataDir, "bin", "cursor-sdk-bridge-"+BridgeRelease)
	dest := filepath.Join(binDir, "bin", name)
	if st, err := os.Stat(dest); err == nil && !st.IsDir() && st.Size() > 0 {
		return dest, nil
	}
	if onProgress != nil {
		onProgress("download", "正在下载 Cursor SDK Bridge（约 20MB，首次需等待）…")
	}
	if err := downloadAndExtract(binDir, onProgress); err != nil {
		return "", err
	}
	if st, err := os.Stat(dest); err != nil || st.IsDir() {
		return "", fmt.Errorf("下载后未找到 bridge: %s", dest)
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(dest, 0o755)
	}
	if onProgress != nil {
		onProgress("ready", "Cursor SDK Bridge 已就绪")
	}
	return dest, nil
}

func lookPath(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("not found")
}

func platformArchive() (string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	switch osName {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	case "windows":
		osName = "win32"
	default:
		return "", fmt.Errorf("不支持的系统: %s", runtime.GOOS)
	}
	switch arch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("不支持的架构: %s", runtime.GOARCH)
	}
	if osName == "win32" && arch != "x64" {
		return "", fmt.Errorf("Windows 仅支持 x64")
	}
	return fmt.Sprintf("cursor-sdk-bridge-standalone-%s-%s.tar.gz", osName, arch), nil
}

func downloadAndExtract(destDir string, onProgress ProgressFn) error {
	archive, err := platformArchive()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://github.com/cursor/sdk-bridge/releases/download/%s/%s", BridgeRelease, archive)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}
	// 清掉上次不完整产物
	_ = os.Remove(filepath.Join(destDir, archive+".partial"))
	final := filepath.Join(destDir, archive)
	tmp := final + ".partial"

	client := &http.Client{Timeout: 8 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "WWorkbench-cursorbridge/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载 Cursor SDK Bridge 失败（需能访问 GitHub）: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 Cursor SDK Bridge 失败: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	var written int64
	var lastPct int = -1
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				_ = os.Remove(tmp)
				return werr
			}
			written += int64(n)
			if onProgress != nil && resp.ContentLength > 0 {
				pct := int(written * 100 / resp.ContentLength)
				if pct >= lastPct+10 || pct >= 99 {
					lastPct = pct
					onProgress("download", fmt.Sprintf("下载 Bridge %d%%（%.1f / %.1f MB）",
						pct, float64(written)/1e6, float64(resp.ContentLength)/1e6))
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("下载中断: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if resp.ContentLength > 0 && written != resp.ContentLength {
		_ = os.Remove(tmp)
		return fmt.Errorf("下载不完整: got %d want %d bytes", written, resp.ContentLength)
	}
	if written < 1_000_000 {
		_ = os.Remove(tmp)
		return fmt.Errorf("下载文件过小（%d bytes），可能被拦截或未跟随重定向", written)
	}
	_ = os.Remove(final)
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if onProgress != nil {
		onProgress("extract", "正在解压 Cursor SDK Bridge…")
	}
	if err := extractTarGz(final, destDir); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}
	return nil
}

func extractTarGz(src, destDir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(destDir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode)
			if mode&0o111 != 0 {
				mode = 0o755
			} else {
				mode = 0o600
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}
