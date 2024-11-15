package index

import (
	"github.com/gofiber/fiber/v2"
)

func Show(c *fiber.Ctx) error {
	//app := app.GetApp()

	return c.Render("app/views/index/show", fiber.Map{})
}
