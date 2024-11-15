package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"time"

	goLFG "github.com/XotoX1337/golfg/app"
	"github.com/XotoX1337/golfg/app/handler/config"
	"github.com/XotoX1337/golfg/app/handler/index"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/template/html/v2"
	"github.com/kardianos/service"
	"github.com/spf13/pflag"
)

//go:embed app/views
var views embed.FS

//go:embed all:public
var public embed.FS

var logger service.Logger
var port *int

type program struct{}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}
func (p *program) run() {
	start(*port)
}
func (p *program) Stop(s service.Service) error {
	// Stop should not block. Return with a few seconds.
	return nil
}

func main() {
	port = pflag.IntP("port", "p", goLFG.GetApp().Config.Port, "Port to listen on")
	pflag.Parse()

	svcConfig := &service.Config{
		Name:        goLFG.GetApp().ServiceName,
		DisplayName: goLFG.GetApp().Name,
		Description: goLFG.GetApp().Description,
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	logger, err = s.Logger(nil)
	if err != nil {
		log.Fatal(err)
	}

	err = s.Run()
	if err != nil {
		err := logger.Error(err)
		check(err)
	}

}

func start(port int) {
	app := goLFG.GetApp()
	if port != 0 {
		app.Config.Port = port
	}
	app.CheckConfig()
	engine := html.NewFileSystem(http.FS(views), ".html")
	engine.AddFunc("Name", func() string {
		return app.Name
	})
	engine.AddFunc("Version", func() string {
		return app.Version
	})
	engine.AddFunc("Year", func() int {
		return time.Now().Year()
	})

	fbr := fiber.New(fiber.Config{
		AppName: goLFG.GetApp().Name,
		Views:   engine,
	})

	fbr.Get("/", index.Show)
	fbr.Get("/config", config.Show)
	fbr.Get("/config/reload", config.Reload)

	fbr.Use("/", filesystem.New(filesystem.Config{
		Root:       http.FS(public),
		PathPrefix: "public",
	}))

	log.Fatal(fbr.Listen(fmt.Sprintf("192.168.38.131:%d", app.Config.Port)))
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
