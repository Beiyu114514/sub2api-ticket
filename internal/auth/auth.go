package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"sub2api-ticket/internal/config"
	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/sub2api"
)

const CookieName = "tsession"
const SessionTTL = 7 * 24 * time.Hour

type Service struct {
	G       *gorm.DB
	Cfg     *config.Config
	Sub2API *sub2api.Client
}

func NewService(g *gorm.DB, cfg *config.Config, c *sub2api.Client) *Service {
	return &Service{G: g, Cfg: cfg, Sub2API: c}
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateSession 签发并写入 cookie
func (s *Service) CreateSession(c *gin.Context, userID uint) error {
	tok := randomToken()
	exp := time.Now().Add(SessionTTL)
	if err := s.G.Create(&model.Session{Token: tok, UserID: userID, ExpiresAt: exp}).Error; err != nil {
		return err
	}
	secure := s.Cfg.SecureCookie()
	if s.Cfg.CookieSameSite == "none" {
		secure = true // SameSite=None 必须 Secure，否则浏览器拒收
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    tok,
		Path:     "/",
		MaxAge:   int(SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite(s.Cfg.CookieSameSite),
	})
	return nil
}

func sameSite(v string) http.SameSite {
	switch v {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}

// CurrentUser 解析 cookie -> user，未登录返回 nil
func (s *Service) CurrentUser(c *gin.Context) *model.User {
	tok, err := c.Cookie(CookieName)
	if err != nil || tok == "" {
		return nil
	}
	var sess model.Session
	if err := s.G.Where("token = ?", tok).First(&sess).Error; err != nil || time.Now().After(sess.ExpiresAt) {
		return nil
	}
	var u model.User
	if err := s.G.First(&u, sess.UserID).Error; err != nil {
		return nil
	}
	return &u
}

// RequireAuth 中间件：登录 + 刷新 last_seen
func (s *Service) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := s.CurrentUser(c)
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		if time.Since(u.LastSeenAt) > time.Hour {
			s.G.Model(u).Update("last_seen_at", time.Now())
		}
		c.Set("user", u)
		c.Next()
	}
}

// RequireAdmin
func (s *Service) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := c.MustGet("user").(*model.User)
		if !u.IsAdmin() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			return
		}
		// 如果是 sub2api 映射用户且较久未向源站核对，进行轻量复验
		if u.Sub2APIID > 0 && time.Since(u.LastSeenAt) > 10*time.Minute {
			if s.Sub2API != nil && s.Cfg.Sub2APIAdminAPIKey != "" {
				info, err := s.Sub2API.GetAdminUser(u.Sub2APIID)
				if err != nil || info.NormalizeRole() != "admin" {
					// 角色降级或账号已不存在，更新本地状态并阻断
					if err == nil {
						newRole := info.NormalizeRole()
						s.G.Model(u).Update("role", newRole)
						u.Role = newRole
					}
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "管理员权限已被源站撤销"})
					return
				}
				s.G.Model(u).Update("last_seen_at", time.Now())
				u.LastSeenAt = time.Now()
			}
		}
		c.Next()
	}
}

// SSO 用 sub2api 透传的 JWT 换自有 session
func (s *Service) SSO(c *gin.Context) {
	var req struct {
		Token  string `json:"token" binding:"required"`
		UserID int64  `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失"})
		return
	}
	info, err := s.Sub2API.VerifyToken(req.Token, req.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	u, err := s.upsertUser(info)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建用户失败"})
		return
	}
	if err := s.CreateSession(c, u.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建会话失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": u})
}

func (s *Service) upsertUser(info *sub2api.UserInfo) (*model.User, error) {
	var u model.User
	err := s.G.Where("sub2api_id = ?", info.Sub2APIID).First(&u).Error
	role := info.NormalizeRole()
	if err != nil {
		u = model.User{
			Sub2APIID:  info.Sub2APIID,
			Username:   info.Username,
			Email:      info.Email,
			Role:       role,
			LastSeenAt: time.Now(),
		}
		if u.Username == "" {
			u.Username = "user-" + strings.TrimSpace(itoa(info.Sub2APIID))
		}
		if err := s.G.Create(&u).Error; err != nil {
			return nil, err
		}
	} else {
		// sub2api 侧字段/角色变化时同步
		updates := map[string]any{"last_seen_at": time.Now()}
		if info.Username != "" && info.Username != u.Username {
			updates["username"] = info.Username
		}
		if info.Email != "" && info.Email != u.Email {
			updates["email"] = info.Email
		}
		if role != u.Role {
			updates["role"] = role
		}
		if len(updates) > 1 {
			s.G.Model(&u).Updates(updates)
		}
	}
	return &u, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// LocalLogin 本地兜底管理员登录（SSO 链路故障时使用）
func (s *Service) LocalLogin(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数缺失"})
		return
	}
	var u model.User
	if err := s.G.Where("email = ? AND password_hash != ''", req.Email).First(&u).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "邮箱或密码错误"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "邮箱或密码错误"})
		return
	}
	if err := s.CreateSession(c, u.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建会话失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": &u})
}

// EnsureLocalAdmin 首次启动按环境变量创建本地管理员
func (s *Service) EnsureLocalAdmin() error {
	if s.Cfg.AdminEmail == "" || s.Cfg.AdminPassword == "" {
		return nil
	}
	var count int64
	s.G.Model(&model.User{}).Where("password_hash != ''").Count(&count)
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(s.Cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u := model.User{
		Sub2APIID:    -1,
		Username:     "admin",
		Email:        s.Cfg.AdminEmail,
		Role:         "admin",
		PasswordHash: string(hash),
		LastSeenAt:   time.Now(),
	}
	if err := s.G.Create(&u).Error; err != nil {
		return err
	}
	return nil
}

func (s *Service) Logout(c *gin.Context) {
	if tok, err := c.Cookie(CookieName); err == nil && tok != "" {
		s.G.Where("token = ?", tok).Delete(&model.Session{})
	}
	secure := s.Cfg.SecureCookie()
	if s.Cfg.CookieSameSite == "none" {
		secure = true
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: sameSite(s.Cfg.CookieSameSite),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
