package config

import (
	"github.com/XotoX1337/golfg/app"
	"github.com/gofiber/fiber/v2"
)

func Reload(c *fiber.Ctx) error {
	app := app.GetApp()
	app.ReloadConfig()

	return c.JSON(app.Config)
}
