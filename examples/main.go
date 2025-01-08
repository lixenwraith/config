package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/LixenWraith/config"
)

type SMTPConfig struct {
	Host     string `toml:"host"`
	Port     string `toml:"port"`
	FromAddr string `toml:"from_addr"`
	AuthUser string `toml:"auth_user"`
	AuthPass string `toml:"auth_pass"`
}

type ServerConfig struct {
	Host         string        `toml:"host"`
	Port         int           `toml:"port"`
	ReadTimeout  time.Duration `toml:"read_timeout"`
	WriteTimeout time.Duration `toml:"write_timeout"`
	MaxConns     int           `toml:"max_conns"`
}

type AppConfig struct {
	SMTP   SMTPConfig   `toml:"smtp"`
	Server ServerConfig `toml:"server"`
	Debug  bool         `toml:"debug"`
}

func main() {
	defaultConfig := AppConfig{
		SMTP: SMTPConfig{
			Host:     "smtp.example.com",
			Port:     "587",
			FromAddr: "noreply@example.com",
			AuthUser: "admin",
			AuthPass: "default123",
		},
		Server: ServerConfig{
			Host:         "localhost",
			Port:         8080,
			ReadTimeout:  time.Second * 30,
			WriteTimeout: time.Second * 30,
			MaxConns:     1000,
		},
		Debug: true,
	}

	configPath := filepath.Join(".", "test.toml")
	cfg := defaultConfig

	// CLI argument usage example to override SMTP host and server port of existing and default config:
	// ./main --smtp.host mail.example.com --server.port 9090

	exists, err := config.Load(configPath, &cfg, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if !exists {
		if err := config.Save(configPath, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save default config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Created default config at:", configPath)
	}

	fmt.Printf("Running with config:\n")
	fmt.Printf("Server: %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("SMTP: %s:%s\n", cfg.SMTP.Host, cfg.SMTP.Port)
	fmt.Printf("Debug: %v\n", cfg.Debug)
}