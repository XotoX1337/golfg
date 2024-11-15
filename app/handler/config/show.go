package config

import (
	"github.com/XotoX1337/golfg/app"
	"github.com/gofiber/fiber/v2"
)

func Show(c *fiber.Ctx) error {
	app := app.GetApp()

	return c.JSON(app.Config)
}
