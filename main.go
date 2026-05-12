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
	"os"
	"time"
     	"bytes"
      
 	"io"
	"sync"
)

//go:embed frontend/*
var assets embed.FS

// 全局SSH会话，复用你的连接
var currentSSHClient *ssh.Client
var sshSession *ssh.Session

// 2. 这里粘贴【ConnInfo结构体】
type ConnInfo struct {
    Name     string `json:"name"`    // 连接名
    Host     string `json:"host"`    // IP
    Port     string `json:"port"`    // 端口
    User     string `json:"user"`    // 用户名
    Password string `json:"pwd"`     // 密码
}
var connFile = "connections.json"


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

// 新建空文件
func (a *App) TouchFile(path string) string {
	if term == nil { return "⚠️ 未连接" }
	f, err := term.sftp.Create(path)
	if err != nil { return "❌ 新建文件失败：" + err.Error() }
	defer f.Close()
	return "✅ 新建文件：" + path
}

// 前端调用：执行终端命令，返回输出
func (a *App) RunCommand(cmd string) string {
    if currentSSHClient == nil || sshSession == nil {
        return "错误：未连接服务器！"
    }
    var buf bytes.Buffer
    sshSession.Stdout = &buf
    sshSession.Stderr = &buf
    err := sshSession.Run(cmd)
    if err != nil && err != io.EOF {
        return buf.String() + "\n命令执行异常：" + err.Error()
    }
    return buf.String()
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
		Title:  "NeoTerm",
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