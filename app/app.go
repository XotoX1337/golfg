package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var app *Application

const default_port int = 9000

type Application struct {
	Name        string
	ServiceName string
	Version     string
	Author      string
	Description string
	Config      struct {
		Log string
		// full path to config file
		Config string
		// full path to log file
		Port int
	}
	ExecutablePath string
	Logger         *zap.Logger
}

func new() *Application {
	app = &Application{}
	app.Name = "go LFG"
	app.ServiceName = "golfg"
	app.Description = "GoLang Looking For Group Tool"
	app.Author = "Frederic Leist"
	app.Version = "v0.1.0"
	app.Config.Port = 0

	executablePath, _ := os.Executable()
	app.ExecutablePath = filepath.Dir(executablePath)
	app.Config.Config = filepath.Join(app.ExecutablePath, app.ServiceName+".toml")
	app.Config.Log = filepath.Join(app.ExecutablePath, app.ServiceName+".log")

	app.Logger = app.getLogger()

	app.loadConfig()
	app.CheckConfig()

	return app
}

func (app *Application) loadConfig() {

	viper.SetConfigName(app.ServiceName)
	viper.SetConfigType("toml")
	viper.AddConfigPath(app.Config.Config)
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		app.writeConfig()
	}
	app.Config.Port = viper.GetInt("app.port")
}

func (app *Application) CheckConfig() {

	if app.Config.Port == 0 {
		app.Logger.Error("invalid value for app.port", zap.Int("port", app.Config.Port))
		app.Config.Port = default_port
	}
}

func (app *Application) writeConfig() {
	viper.Set("app.name", app.Name)
	viper.Set("app.service_name", app.ServiceName)
	viper.Set("app.version", app.Version)
	viper.Set("app.port", app.Config.Port)

	err := viper.WriteConfigAs(app.Config.Config)
	if err != nil {
		log.Fatalln(err)
	}
}

func (app *Application) ReloadConfig() {
	app.loadConfig()
	app.CheckConfig()
}

func (app *Application) getLogger() *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	cfg.OutputPaths = []string{
		app.Config.Log,
	}
	logger, err := cfg.Build()
	if err != nil {
		fmt.Println(err)
	}
	defer logger.Sync() //nolint:errcheck
	return logger
}

func GetApp() *Application {
	if app == nil {
		app = new()
	}
	return app
}
