package config

import "github.com/caarlos0/env/v11"

type Config struct {
	Host string `env:"HOST"`
	Port string `env:"PORT" envDefault:"3000"`
}

func MustNew() *Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return &cfg
}
