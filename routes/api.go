package routes

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/anomalous69/fchannel/config"
	"github.com/anomalous69/fchannel/util"
	"github.com/gofiber/fiber/v2"
)

func Media(ctx *fiber.Ctx) error {
	if ctx.Query("hash") != "" {
		return RouteImages(ctx, ctx.Query("hash"))
	}

	return ctx.SendStatus(404)
}

func RouteImages(ctx *fiber.Ctx, media string) error {
	req, err := http.NewRequest("GET", config.MediaHashs[media], nil)
	if err != nil {
		return util.MakeError(err, "RouteImages")
	}

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		fileBytes, err := os.ReadFile("./static/notfound.png")
		if err != nil {
			return util.MakeError(err, "RouteImages")
		}

		_, err = ctx.Write(fileBytes)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fileBytes, err := os.ReadFile("./static/notfound.png")
		if err != nil {
			return util.MakeError(err, "RouteImages")
		}

		_, err = ctx.Write(fileBytes)
		return util.MakeError(err, "RouteImages")
	}

	body, _ := io.ReadAll(resp.Body)
	for name, values := range resp.Header {
		for _, value := range values {
			ctx.Append(name, value)
		}
	}

	return ctx.Send(body)
}
