package modules

import (
	"fmt"
	"net/http"

	"github.com/CapsLock-Studio/binance-premium-bot/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/ratelimit"
)

type Http struct {
	RateLimiter ratelimit.Limiter
}

func NewHttp(ratelimiter ratelimit.Limiter) *Http {
	return &Http{
		RateLimiter: ratelimiter,
	}
}

func (h *Http) Serve() {
	ch := make(chan string, 10)
	route := gin.Default()

	route.POST("/", func(ctx *gin.Context) {
		var r models.ConfigSetting

		r.Difference = DEFAULT_DIFFERENCE
		r.Leverage = DEFAULT_LEVERAGE

		if ctx.Bind(&r) != nil {
			return
		}

		ID := uuid.New().String()

		go func() {
			defer func() {
				if err := recover(); err != nil {
					fmt.Println(err)
				}
			}()

			NewCore(&r, ch, &ID, h.RateLimiter).Run()
		}()

		ctx.Data(http.StatusOK, "text/plain", []byte(ID))
	})

	route.DELETE("/:id", func(ctx *gin.Context) {
		ID := ctx.Param("id")

		ch <- ID
		ctx.Data(http.StatusOK, "text/plain", []byte("DONE"))
	})

	route.Run()
}
