package main

import (
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"sub2api-ticket/internal/api"
	"sub2api-ticket/internal/auth"
	"sub2api-ticket/internal/config"
	"sub2api-ticket/internal/db"
	"sub2api-ticket/internal/notify"
	"sub2api-ticket/internal/static"
	"sub2api-ticket/internal/store"
	"sub2api-ticket/internal/sub2api"
	"sub2api-ticket/internal/tgbot"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	g, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	settings := store.NewSettings(g)
	tickets := store.NewTicketStore(g, settings)
	s2a := sub2api.New(cfg.Sub2APIBaseURL, cfg.Sub2APIVerifyPath, cfg.Sub2APIUserPath, cfg.Sub2APIAdminAPIKey)
	authSvc := auth.NewService(g, cfg, s2a)
	workerPool := tgbot.NewWorkerPool(16, 256)
	upSvc := &api.UploadService{Deps: api.Deps{G: g, Settings: settings}}
	core := &tgbot.Core{
		G:             g,
		S:             settings,
		T:             tickets,
		PublicBaseURL: cfg.PublicBaseURL,
		TGProxy:       cfg.TGProxy,
		Pool:          workerPool,
		Uploads: &tgbot.SaveImageDeps{
			SaveImage: upSvc.SaveImageAny,
			OpenByKey: upSvc.OpenImageByKey,
			Rebind:    upSvc.RebindTicketToUploads,
		},
	}

	if err := authSvc.EnsureLocalAdmin(); err != nil {
		log.Printf("创建本地管理员失败: %v", err)
	}
	db.CleanExpired(g)

	deps := api.Deps{
		G: g, Cfg: cfg, Auth: authSvc, Settings: settings,
		Tickets: tickets, Sub2API: s2a, BotCore: core,
	}
	r := api.NewRouter(deps)
	registerSPA(r)

	go (&notify.Worker{G: g, S: settings, T: tickets, Core: core, TGProxy: cfg.TGProxy}).Run()
	if cfg.TGPolling {
		go pollingLoop(core)
	}

	log.Printf("sub2api-ticket 已启动，监听 %s（webhook 模式: %v）", cfg.ListenAddr, !cfg.TGPolling && cfg.PublicBaseURL != "")
	if err := r.Run(cfg.ListenAddr); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}

// pollingLoop long polling 模式（TG_POLLING=true 时本地调试用）
func pollingLoop(core *tgbot.Core) {
	offset := int64(0)
	for {
		bot := tgbot.NewWithProxy(core.S.BotToken(), core.TGProxy)
		if !bot.Valid() {
			time.Sleep(5 * time.Second)
			continue
		}
		updates, err := bot.GetUpdates(offset, 30)
		if err != nil {
			log.Printf("[polling] %v", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for i := range updates {
			offset = updates[i].UpdateID + 1
			u := updates[i]
			core.SubmitTask(func() {
				core.HandleUpdate(bot, u)
			})
		}
	}
}

// registerSPA 前端静态资源 + SPA fallback
func registerSPA(r *gin.Engine) {
	fsys := static.FS()
	fileServer := http.FileServer(http.FS(fsys))
	r.NoRoute(func(c *gin.Context) {
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p != "" {
			if st, err := fs.Stat(fsys, p); err == nil && !st.IsDir() {
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		data, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			c.String(200, "前端未构建：请先在 web/ 下执行 npm run build")
			return
		}
		c.Data(200, "text/html; charset=utf-8", data)
	})
}
