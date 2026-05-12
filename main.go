package main

import (
	"context"
	"embed"
	"fmt"
	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"sync"
	"time"
)

//go:embed frontend/*
var assets embed.FS

type App struct {
	ctx context.Context
}

type ConnConfig struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"pwd"`
}

type SSHTerm struct {
	sshCli *ssh.Client
	sftp   *sftp.Client
	mu     sync.Mutex
}

var term *SSHTerm

func NewApp() *App { return &App{} }
func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// 1. 连接
func (a *App) Connect(cfg ConnConfig) string {
	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{ssh.Password(cfg.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	cli, err := ssh.Dial("tcp", cfg.IP+":"+cfg.Port, sshCfg)
	if err != nil {
		return "❌ 连接失败：" + err.Error()
	}
	sftpCli, err := sftp.NewClient(cli)
	if err != nil {
		return "❌ SFTP初始化失败：" + err.Error()
	}
	term = &SSHTerm{sshCli: cli, sftp: sftpCli}
	return "✅ 连接成功（SFTP就绪）"
}

// 2. 执行命令
func (a *App) SendCmd(cmd string) string {
	if term == nil { return "⚠️ 未连接" }
	sess, err := term.sshCli.NewSession()
	if err != nil { return "❌ 会话错误：" + err.Error() }
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	if err != nil { return string(out) + "\n❌ 失败：" + err.Error() }
	return string(out)
}

// 3. 读取目录
func (a *App) ListDir(path string) []string {
	if term == nil { return []string{} }
	files, err := term.sftp.ReadDir(path)
	if err != nil { return []string{} }
	var list []string
	for _, f := range files {
		list = append(list, f.Name())
	}
	return list
}

// ========== 修复核心：上传接收二进制，不拿本地路径 ==========
func (a *App) UploadFileBin(remotePath string, data []byte) string {
	if term == nil { return "⚠️ 未连接" }
	rf, err := term.sftp.Create(remotePath)
	if err != nil { return "❌ 创建远程文件：" + err.Error() }
	defer rf.Close()
	_, err = rf.Write(data)
	if err != nil { return "❌ 上传失败：" + err.Error() }
	return fmt.Sprintf("✅ 上传成功 → %s", remotePath)
}

// 4. 下载（不变，正常）
func (a *App) DownloadFile(remotePath, localPath string) string {
	if term == nil { return "⚠️ 未连接" }
	rf, err := term.sftp.Open(remotePath)
	if err != nil { return "❌ 打开远程：" + err.Error() }
	defer rf.Close()
	lf, err := os.Create(localPath)
	if err != nil { return "❌ 创建本地：" + err.Error() }
	defer lf.Close()
	_, err = io.Copy(lf, rf)
	if err != nil { return "❌ 下载失败：" + err.Error() }
	return fmt.Sprintf("✅ 下载成功 → %s", localPath)
}

// 5. 删除、重命名、新建（不变）
func (a *App) RemoveFile(path string) string {
	if term == nil { return "⚠️ 未连接" }
	err := term.sftp.Remove(path)
	if err != nil { return "❌ 删除失败：" + err.Error() }
	return "✅ 删除成功：" + path
}
func (a *App) RenameFile(oldPath, newPath string) string {
	if term == nil { return "⚠️ 未连接" }
	err := term.sftp.Rename(oldPath, newPath)
	if err != nil { return "❌ 重命名失败：" + err.Error() }
	return "✅ 重命名成功"
}
func (a *App) Mkdir(path string) string {
	if term == nil { return "⚠️ 未连接" }
	err := term.sftp.Mkdir(path)
	if err != nil { return "❌ 新建文件夹失败：" + err.Error() }
	return "✅ 新建文件夹：" + path
}

// 系统弹窗（下载用）
func (a *App) SaveFileDialog(defaultName string) string {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存文件",
		DefaultFilename: defaultName,
	})
	if err != nil { return "" }
	return path
}

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:  "NeoTerm 仿WindTerm",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind:      []any{app},
	})
	if err != nil { panic(err) }
}