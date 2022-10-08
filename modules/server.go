package modules

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/CapsLock-Studio/binance-premium-bot/models"
	"github.com/gin-gonic/gin"
	"go.uber.org/ratelimit"
)

type Http struct {
	Store       string
	DB          *DB
	Channel     chan string
	RateLimiter ratelimit.Limiter
}

func NewHttp(db *DB, ratelimiter ratelimit.Limiter) *Http {
	return &Http{
		Channel:     make(chan string, 10),
		DB:          db,
		RateLimiter: ratelimiter,
	}
}

func (h *Http) Serve() {
	route := gin.Default()

	for _, v := range h.DB.GetSates() {
		var setting models.ConfigSetting
		if err := json.Unmarshal([]byte(v.Value), &setting); err != nil {
			log.Fatal(err)
		}

		go h.Bot(setting, v.ID)
	}

	route.Use(func(ctx *gin.Context) {
		userID := ctx.GetHeader("X-USER")

		if userID == "" {
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}

		ID := ctx.Param("id")

		if ID != "" {
			forbidden := true
			states := h.DB.GetUserSates(userID)

			for _, v := range states {
				if v.ID == ID {
					forbidden = false
				}
			}

			if forbidden {
				ctx.AbortWithStatus(http.StatusForbidden)
				return
			}
		}

		ctx.Set("user_id", userID)
	})

	route.GET("/", func(ctx *gin.Context) {
		states := h.DB.GetUserSates(ctx.GetString("user_id"))

		result := make([]string, 0)

		for _, v := range states {
			result = append(result, v.ID)
		}

		ctx.Data(http.StatusOK, "text/plain", []byte(strings.Join(result, "\n")))
	})

	route.POST("/", func(ctx *gin.Context) {
		var r models.ConfigSetting

		r.Difference = DEFAULT_DIFFERENCE
		r.Leverage = DEFAULT_LEVERAGE
		r.Before = DEFAULT_MINUTES

		if ctx.Bind(&r) != nil {
			return
		}

		ID := h.DB.CreateUserState(ctx.GetString("user_id"), r)

		go h.Bot(r, ID)

		// response
		ctx.Data(http.StatusOK, "text/plain", []byte(ID))
	})

	route.DELETE("/:id", func(ctx *gin.Context) {
		ID := ctx.Param("id")

		h.Channel <- ID

		h.DB.DropUserState(ctx.GetString("user_id"), ID)

		ctx.Data(http.StatusOK, "text/plain", []byte("DONE"))
	})

	route.DELETE("/", func(ctx *gin.Context) {
		err := h.DB.DropUserStates(ctx.GetString("user_id"))

		if err != nil {
			return
		}

		ctx.Data(http.StatusOK, "text/plain", []byte("DONE"))
	})

	route.Run()
}

func (h *Http) Bot(setting models.ConfigSetting, ID string) {
	defer func() {
		if err := recover(); err != nil {
			log.Fatal(err)
			return
		}
	}()

	NewCore(&setting, h.Channel, &ID, h.RateLimiter).Run()

	// drop state
	h.DB.DropState(ID)
}
